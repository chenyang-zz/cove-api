package chat

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"log/slog"

	corereact "github.com/boxify/api-go/internal/core/agent/react"
	"github.com/boxify/api-go/internal/core/llm"
	coremcp "github.com/boxify/api-go/internal/core/mcp"
	coretool "github.com/boxify/api-go/internal/core/tool"
	flow "github.com/boxify/api-go/internal/domain/flow"
	domaintools "github.com/boxify/api-go/internal/domain/tools"
	domaintoolmcp "github.com/boxify/api-go/internal/domain/tools/mcp"
	"github.com/boxify/api-go/internal/domain/types"
	"github.com/boxify/api-go/internal/models"
	"github.com/boxify/api-go/internal/observability/xlog"
	"github.com/boxify/api-go/internal/svc"
	"github.com/boxify/api-go/internal/util"
	"github.com/google/uuid"
)

type Input struct {
	UserID               uuid.UUID
	ConversationID       uuid.UUID
	CurrentUserMessageID uuid.UUID
	Message              string
	Attachments          []*types.MessageAttachment
	EnableKnowledge      bool
	Temperature          float64
	SystemPrompt         string
}

const coveAssistantIntro = "你是「Cove」的智能助手。你可以调用以下工具来帮助回答用户的问题。"

type Orchestrator struct {
	svcCtx *svc.ServiceContext
	log    *slog.Logger
}

func NewOrchestrator(svcCtx *svc.ServiceContext) *Orchestrator {
	return &Orchestrator{
		svcCtx: svcCtx,
		log:    xlog.Component("domain.flow.chat"),
	}
}

func (o *Orchestrator) Run(ctx context.Context, input Input) (<-chan flow.Message, error) {
	if o == nil || o.svcCtx == nil {
		return nil, errors.New("chat flow orchestrator is nil")
	}
	out := make(chan flow.Message, 8)
	go func() {
		defer close(out)
		result, err := o.generate(ctx, input, out)
		if err != nil {
			out <- &flow.ErrorMessage{
				Message: err.Error(),
				Partial: result.Partial,
				Err:     err,
			}
			return
		}
		out <- &flow.AssistantMessage{
			Answer: strings.TrimSpace(result.Answer),
		}
		out <- &flow.DoneMessage{}
	}()
	return out, nil
}

type generationResult struct {
	Answer  string
	Partial string
}

func (o *Orchestrator) generate(ctx context.Context, input Input, events chan<- flow.Message) (generationResult, error) {
	client, err := svc.ChatClient(ctx, o.svcCtx, input.UserID)
	if err != nil {
		return generationResult{}, err
	}

	history, err := o.historyMessages(ctx, input.UserID, input.ConversationID, input.CurrentUserMessageID)
	if err != nil {
		return generationResult{}, err
	}
	runCtx, kbIDs, err := toolContext(ctx, o.svcCtx, input.UserID, input.EnableKnowledge)
	if err != nil {
		return generationResult{}, err
	}
	registry, closeTools, err := o.toolRegistry(runCtx, input.UserID, kbIDs)
	if err != nil {
		return generationResult{}, err
	}
	defer func() {
		if closeTools == nil {
			return
		}
		if closeErr := closeTools(); closeErr != nil {
			o.log.WarnContext(runCtx, "关闭MCP工具会话失败", slog.String("error", closeErr.Error()))
		}
	}()

	hooks := &agentHooks{events: events}
	options := []corereact.Option{
		corereact.WithHooks(hooks),
		corereact.WithSystemPrompt(composeSystemPrompt(coveAssistantIntro, input.SystemPrompt)),
		corereact.WithModelOptions(llm.WithTemperature(input.Temperature)),
	}
	result, err := corereact.New(client, registry, options...).Run(runCtx, corereact.Input{
		Query:    composeQuery(input.Message, input.Attachments),
		Messages: history,
	})
	out := generationResult{
		Partial: hooks.partial(),
	}
	if err != nil {
		if out.Partial == "" && result != nil {
			out.Partial = result.Answer
		}
		return out, err
	}
	if result != nil {
		out.Answer = result.Answer
	}
	return out, nil
}

func (o *Orchestrator) historyMessages(ctx context.Context, userID uuid.UUID, conversationID uuid.UUID, currentUserMessageID uuid.UUID) ([]*llm.Message, error) {
	rows, err := o.svcCtx.MessageRepo.ListByConversationID(ctx, userID, conversationID)
	if err != nil {
		return nil, err
	}
	messages := make([]*llm.Message, 0, len(rows))
	for _, row := range rows {
		if row == nil || row.ID == currentUserMessageID || strings.TrimSpace(row.Content) == "" {
			continue
		}
		switch row.Role {
		case string(llm.UserRole):
			messages = append(messages, llm.UserMessage(row.Content))
		case string(llm.AssistantRole):
			messages = append(messages, llm.AssistantMessage(row.Content))
		}
	}
	return messages, nil
}

// toolRegistry 构建可用于当前用户的工具注册表。
// 1. 内置工具：默认启用，除非用户显式禁用。
// 2. 知识库工具：仅在用户启用知识库时加载，默认启用，除非用户显式禁用。
// 3. MCP 工具：仅在用户启用 MCP 服务时加载，默认启用，除非用户显式禁用。
func (o *Orchestrator) toolRegistry(ctx context.Context, userID uuid.UUID, kbIDs []uuid.UUID) (*coretool.Registry, func() error, error) {
	if o.svcCtx == nil {
		return coretool.NewRegistry(), nil, nil
	}
	catalog, err := domaintools.NewCatalog(o.svcCtx)
	if err != nil {
		return nil, nil, err
	}
	setNames := []string{domaintools.ToolSetSystem}
	if len(kbIDs) > 0 {
		setNames = append(setNames, domaintools.ToolSetKnowledge)
	}
	registry, err := catalog.BuildRegistry(ctx, coretool.Selection{SetNames: setNames})
	if err != nil {
		return nil, nil, err
	}
	if o.svcCtx.ToolConfigRepo == nil {
		return nil, nil, errors.New("tool config repository is nil")
	}
	rows, err := o.svcCtx.ToolConfigRepo.List(ctx, userID)
	if err != nil {
		return nil, nil, err
	}

	// 未持久化配置的内置工具默认启用，只覆盖用户显式保存过的状态。
	enabled := make(map[string]bool)
	for _, descriptor := range registry.List(nil) {
		enabled[descriptor.Name] = true
	}
	configured := make(map[string]struct{}, len(rows))
	for _, row := range rows {
		if row == nil {
			continue
		}
		if _, exists := configured[row.ToolKey]; exists {
			continue
		}
		if _, exists := enabled[row.ToolKey]; exists {
			enabled[row.ToolKey] = row.Enabled
			configured[row.ToolKey] = struct{}{}
		}
	}
	filtered := coretool.NewRegistry()
	for _, tool := range registry.Tools(enabled) {
		if err := filtered.Register(ctx, tool); err != nil {
			return nil, nil, err
		}
	}

	if o.svcCtx.MCPServerRepo == nil || o.svcCtx.MCPToolService == nil {
		return filtered, nil, nil
	}
	servers, err := o.svcCtx.MCPServerRepo.List(ctx, userID)
	if err != nil {
		return nil, nil, err
	}
	leases := make([]*coremcp.OpenedTools, 0, len(servers))
	closeAll := func() error {
		var errs []error
		for _, lease := range slices.Backward(leases) {
			if closeErr := lease.Close(); closeErr != nil {
				errs = append(errs, closeErr)
			}
		}
		return errors.Join(errs...)
	}
	for _, server := range servers {
		if server == nil || !server.Enabled {
			continue
		}
		serverConfig, configErr := domaintoolmcp.ServerConfig(server, o.svcCtx.SecretCipher)
		if configErr != nil {
			o.warnMCPServer(ctx, userID, server.ID, "解析MCP服务配置失败", configErr)
			continue
		}
		opened, openErr := o.svcCtx.MCPToolService.OpenTools(ctx, serverConfig)
		if openErr != nil {
			o.warnMCPServer(ctx, userID, server.ID, "加载MCP工具失败", openErr)
			continue
		}
		leases = append(leases, opened)
		for _, definition := range domaintoolmcp.Definitions(server, opened.Tools()) {
			if definition == nil {
				continue
			}
			if configuredEnabled, ok := enabledByToolConfig(rows, definition.Key); ok && !configuredEnabled {
				continue
			}
			tool := domaintoolmcp.NewTool(definition, opened)
			if tool == nil {
				continue
			}
			if err := filtered.Register(ctx, tool); err != nil {
				_ = closeAll()
				return nil, nil, fmt.Errorf("register MCP tool %q: %w", definition.Key, err)
			}
		}
	}
	if len(leases) == 0 {
		return filtered, nil, nil
	}
	return filtered, closeAll, nil
}

func enabledByToolConfig(rows []*models.ToolConfig, toolKey string) (bool, bool) {
	for _, row := range rows {
		if row != nil && row.ToolKey == toolKey {
			return row.Enabled, true
		}
	}
	return false, false
}

// warnMCPServer 记录 MCP 服务相关的警告日志，包含用户 ID、服务 ID 和错误信息。
func (o *Orchestrator) warnMCPServer(ctx context.Context, userID uuid.UUID, serverID uuid.UUID, message string, err error) {
	if o == nil || o.log == nil {
		return
	}
	o.log.WarnContext(ctx, message,
		slog.String("user_id", userID.String()),
		slog.String("server_id", serverID.String()),
		slog.String("error", err.Error()),
	)
}

func toolContext(ctx context.Context, svcCtx *svc.ServiceContext, userID uuid.UUID, enableKnowledge bool) (context.Context, []uuid.UUID, error) {
	ctx = util.WithUserID(ctx, userID)
	if !enableKnowledge || svcCtx == nil || svcCtx.KnowledgeBaseRepo == nil {
		return ctx, nil, nil
	}
	rows, err := svcCtx.KnowledgeBaseRepo.List(ctx, userID)
	if err != nil {
		return ctx, nil, err
	}
	kbIDs := make([]uuid.UUID, 0, len(rows))
	for _, row := range rows {
		if row != nil && row.ChatEnabled && row.ID != uuid.Nil {
			kbIDs = append(kbIDs, row.ID)
		}
	}
	if len(kbIDs) == 0 {
		return ctx, nil, nil
	}
	return util.WithKnowledgeBaseIDs(ctx, kbIDs), kbIDs, nil
}

func composeSystemPrompt(parts ...string) string {
	out := []string{}
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return strings.Join(out, "\n\n")
}

func composeQuery(message string, attachments []*types.MessageAttachment) string {
	query := strings.TrimSpace(message)
	if len(attachments) == 0 {
		return query
	}
	parts := []string{query, "\n\n附件内容："}
	for _, attachment := range attachments {
		if attachment == nil || strings.TrimSpace(attachment.Content) == "" {
			continue
		}
		name := strings.TrimSpace(attachment.FileName)
		if name == "" {
			name = "未命名附件"
		}
		parts = append(parts, "\n["+name+"]\n"+strings.TrimSpace(attachment.Content))
	}
	return strings.TrimSpace(strings.Join(parts, ""))
}

type agentHooks struct {
	corereact.NoopHooks
	events   chan<- flow.Message
	streamed strings.Builder
}

// OnToken 将 Agent 已确认可展示的模型文本增量交给 flow。
func (h *agentHooks) OnToken(ctx context.Context, state corereact.State, text string) error {
	if text == "" {
		return nil
	}
	h.streamed.WriteString(text)
	return h.emit(ctx, &flow.PartialMessage{Text: text})
}

func (h *agentHooks) partial() string {
	if h == nil {
		return ""
	}
	return strings.TrimSpace(h.streamed.String())
}

func (h *agentHooks) BeforeTool(ctx context.Context, state corereact.State, call corereact.ToolCall) error {
	return h.emit(ctx, &flow.ToolCallMessage{
		Tool:       call.Name,
		Input:      cloneToolInput(call.Input),
		Iteration:  state.Iteration,
		ToolCallID: state.LastDecision.ToolCallID,
	})
}

func (h *agentHooks) AfterTool(ctx context.Context, state corereact.State, call corereact.ToolCall, output coretool.Output, toolErr error) error {
	if toolErr != nil {
		return toolErr
	}
	return h.emit(ctx, &flow.ToolResultMessage{
		Tool:        call.Name,
		Input:       cloneToolInput(call.Input),
		Observation: output.Text,
		Iteration:   state.Iteration,
		ToolCallID:  state.LastDecision.ToolCallID,
	})
}

func (h *agentHooks) emit(ctx context.Context, message flow.Message) error {
	if h == nil || h.events == nil || message == nil {
		return nil
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case h.events <- message:
		return nil
	}
}

func cloneToolInput(input coretool.Input) map[string]any {
	if input == nil {
		return nil
	}
	out := make(map[string]any, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}
