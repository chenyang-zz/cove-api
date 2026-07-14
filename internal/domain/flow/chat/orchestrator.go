package chat

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"sync"
	"time"

	"log/slog"

	"github.com/boxify/api-go/internal/config"
	corereact "github.com/boxify/api-go/internal/core/agent/react"
	corecontext "github.com/boxify/api-go/internal/core/context"
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
	ContextPolicy        *corecontext.Policy
}

const (
	// defaultMCPAssembleBudget 是单轮对话中 MCP 工具发现的墙钟上限。
	defaultMCPAssembleBudget = 8 * time.Second
	// defaultMCPAssembleConcurrency 限制同时 OpenTools 的 MCP server 数，对齐 toolconfig。
	defaultMCPAssembleConcurrency = 4
)

// mcpServerWork 表示一个已解密、待并行发现的 MCP server。
type mcpServerWork struct {
	server *models.MCPServer
	cfg    coremcp.ServerConfig
}

// mcpOpenResult 保存并行 OpenTools 的单 server 结果。
type mcpOpenResult struct {
	server *models.MCPServer
	opened *coremcp.OpenedTools
	err    error
}

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

	history, err := o.historyEntries(ctx, input.UserID, input.ConversationID, input.CurrentUserMessageID)
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

	hooks := &agentHooks{events: events, registry: registry}

	// 如果启用上下文管理，则使用 corecontext.Manager 规整历史消息；否则直接使用原始历史。
	historyMessages := contextEntryMessages(history)
	var contextManager *corecontext.Manager
	if input.ContextPolicy != nil && input.ContextPolicy.Enabled {
		var store corecontext.Store
		if o.svcCtx.ConversationContextStateRepo != nil {
			store = newConversationContextStore(o.svcCtx.ConversationContextStateRepo, input.UserID, input.ConversationID)
		}
		contextManager, err = corecontext.NewManager(
			corecontext.WithLLMClient(client),
			corecontext.WithPolicy(input.ContextPolicy),
			corecontext.WithStore(store),
		)
		if err != nil {
			return generationResult{}, err
		}
		query := composeQuery(input.Message, input.Attachments)
		prepared, prepareErr := contextManager.Prepare(runCtx, &corecontext.Input{
			Key:     input.ConversationID.String(),
			Entries: history,
			Pinned: []*llm.Message{
				llm.SystemMessage(strings.TrimSpace(input.SystemPrompt)),
				llm.UserMessage(query),
			},
			Tools: registry.List(nil),
		})
		if prepareErr != nil {
			return generationResult{}, prepareErr
		}
		historyMessages = prepared.Messages
		o.log.DebugContext(runCtx, "完成模型上下文规整",
			slog.Int("before_tokens", prepared.BeforeTokens),
			slog.Int("after_tokens", prepared.AfterTokens),
			slog.Bool("compacted", prepared.Compacted),
			slog.Bool("fallback", prepared.UsedFallback),
		)
	}

	// SystemPrompt 由 logic 层完整组装（人格 / Cove intro / AgentConfig），此处只透传。
	options := []corereact.Option{
		corereact.WithHooks(hooks),
		corereact.WithSystemPrompt(strings.TrimSpace(input.SystemPrompt)),
		corereact.WithModelOptions(llm.WithTemperature(input.Temperature)),
	}
	// 如果启用了上下文管理，则在 corereact 中注入 MessagePreparer，确保每轮生成前都使用规整后的历史。
	if contextManager != nil {
		options = append(options, corereact.WithMessagePreparer(contextManager))
	}
	result, err := corereact.New(client, registry, options...).Run(runCtx, corereact.Input{
		Query:    composeQuery(input.Message, input.Attachments),
		Messages: historyMessages,
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

func (o *Orchestrator) historyEntries(ctx context.Context, userID uuid.UUID, conversationID uuid.UUID, currentUserMessageID uuid.UUID) ([]*corecontext.Entry, error) {
	rows, err := o.svcCtx.MessageRepo.ListByConversationID(ctx, userID, conversationID)
	if err != nil {
		return nil, err
	}
	entries := make([]*corecontext.Entry, 0, len(rows))
	for _, row := range rows {
		if row == nil || row.ID == currentUserMessageID || strings.TrimSpace(row.Content) == "" {
			continue
		}
		switch row.Role {
		case string(llm.UserRole):
			entries = append(entries, &corecontext.Entry{ID: row.ID.String(), Message: llm.UserMessage(row.Content)})
		case string(llm.AssistantRole):
			entries = append(entries, &corecontext.Entry{ID: row.ID.String(), Message: llm.AssistantMessage(row.Content)})
		}
	}
	return entries, nil
}

func contextEntryMessages(entries []*corecontext.Entry) []*llm.Message {
	messages := make([]*llm.Message, 0, len(entries))
	for _, entry := range entries {
		if entry != nil && entry.Message != nil {
			messages = append(messages, entry.Message)
		}
	}
	return llm.CloneMessages(messages)
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

	// 先收集可发现的 server；解密失败不进入并行阶段。
	works := make([]mcpServerWork, 0, len(servers))
	for _, server := range servers {
		if server == nil || !server.Enabled {
			continue
		}
		serverConfig, configErr := domaintoolmcp.ServerConfig(server, o.svcCtx.SecretCipher)
		if configErr != nil {
			o.warnMCPServer(ctx, userID, server.ID, "解析MCP服务配置失败", configErr)
			continue
		}
		works = append(works, mcpServerWork{server: server, cfg: serverConfig})
	}
	if len(works) == 0 {
		return filtered, nil, nil
	}

	// Phase 1：有限并行发现，墙钟与并发度来自 config.MCP（非法/零值回落默认）。
	budget, concurrency := mcpAssembleSettings(o.svcCtx.Config)
	mcpCtx, cancel := context.WithTimeout(ctx, budget)
	defer cancel()

	results := make([]mcpOpenResult, len(works))
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup
	for i, work := range works {
		i, work := i, work
		wg.Go(func() {
			select {
			case <-mcpCtx.Done():
				results[i] = mcpOpenResult{server: work.server, err: mcpCtx.Err()}
				return
			case sem <- struct{}{}:
			}
			defer func() { <-sem }()
			opened, openErr := o.svcCtx.MCPToolService.OpenTools(mcpCtx, work.cfg)
			results[i] = mcpOpenResult{server: work.server, opened: opened, err: openErr}
		})
	}
	wg.Wait()

	// 统一收集成功 lease，确保后续 Register 失败时也能全部关闭。
	leases := make([]*coremcp.OpenedTools, 0, len(results))
	for _, result := range results {
		if result.opened != nil {
			leases = append(leases, result.opened)
		}
	}
	closeAll := func() error {
		var errs []error
		for _, lease := range slices.Backward(leases) {
			if closeErr := lease.Close(); closeErr != nil {
				errs = append(errs, closeErr)
			}
		}
		return errors.Join(errs...)
	}

	openedOK, failed := 0, 0
	// Phase 2：按原顺序串行注册，避免并发写 registry。
	for _, result := range results {
		if result.err != nil {
			failed++
			o.warnMCPServer(ctx, userID, result.server.ID, "加载MCP工具失败", result.err)
			continue
		}
		if result.opened == nil || result.server == nil {
			failed++
			continue
		}
		openedOK++
		for _, definition := range domaintoolmcp.Definitions(result.server, result.opened.Tools()) {
			if definition == nil {
				continue
			}
			if configuredEnabled, ok := enabledByToolConfig(rows, definition.Key); ok && !configuredEnabled {
				continue
			}
			tool := domaintoolmcp.NewTool(definition, result.opened)
			if tool == nil {
				continue
			}
			if err := filtered.Register(ctx, tool); err != nil {
				_ = closeAll()
				return nil, nil, fmt.Errorf("register MCP tool %q: %w", definition.Key, err)
			}
		}
	}
	if mcpCtx.Err() != nil && failed > 0 {
		o.warnMCPAssembleBudget(ctx, userID, budget, openedOK, failed)
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

// warnMCPAssembleBudget 在总预算耗尽或取消时输出一条汇总日志。
func (o *Orchestrator) warnMCPAssembleBudget(ctx context.Context, userID uuid.UUID, budget time.Duration, openedOK, failed int) {
	if o == nil || o.log == nil {
		return
	}
	o.log.WarnContext(ctx, "MCP组装预算耗尽或取消",
		slog.String("user_id", userID.String()),
		slog.Duration("budget", budget),
		slog.Int("opened_ok", openedOK),
		slog.Int("failed_or_skipped", failed),
	)
}

// mcpAssembleSettings 从配置解析对话组装预算与并发度；解析失败或非正值时回落默认。
func mcpAssembleSettings(cfg config.Config) (time.Duration, int) {
	budget := defaultMCPAssembleBudget
	if cfg.MCP.AssembleBudget != "" {
		if d, err := time.ParseDuration(cfg.MCP.AssembleBudget); err == nil && d > 0 {
			budget = d
		}
	}
	concurrency := defaultMCPAssembleConcurrency
	if cfg.MCP.AssembleConcurrency > 0 {
		concurrency = cfg.MCP.AssembleConcurrency
	}
	return budget, concurrency
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

// composeQuery 将用户消息和附件内容组合为单条查询，附件内容按顺序附加在消息后。
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
	registry *coretool.Registry
	streamed strings.Builder
	// thinking 为 true 表示本轮模型调用尚未结束思考态；首 token / AfterModel 兜底时结束。
	thinking bool
}

// BeforeModel 在发起大模型请求前发出 thinking 状态。
func (h *agentHooks) BeforeModel(ctx context.Context, state corereact.State, messages []*llm.Message) error {
	h.thinking = true
	return h.emit(ctx, &flow.ThinkMessage{
		Status:    flow.ThinkStatusThinking,
		Iteration: state.Iteration,
	})
}

// AfterModel 在整段流结束后触发；仅作兜底结束 thinking（本轮无任何 OnToken 时）。
// 有可见 token 时 think 已在首 token 前结束，此处为幂等 no-op。
func (h *agentHooks) AfterModel(ctx context.Context, state corereact.State, output string, modelErr error) error {
	return h.endThink(ctx, state.Iteration)
}

// OnToken 在首个可展示 token 前结束 thinking，再转发增量。
func (h *agentHooks) OnToken(ctx context.Context, state corereact.State, text string) error {
	if text == "" {
		return nil
	}
	if err := h.endThink(ctx, state.Iteration); err != nil {
		return err
	}
	h.streamed.WriteString(text)
	return h.emit(ctx, &flow.PartialMessage{Text: text})
}

// endThink 结束本轮 thinking（幂等）。
func (h *agentHooks) endThink(ctx context.Context, iteration int) error {
	if h == nil || !h.thinking {
		return nil
	}
	h.thinking = false
	return h.emit(ctx, &flow.ThinkMessage{
		Status:    flow.ThinkStatusDone,
		Iteration: iteration,
	})
}

func (h *agentHooks) partial() string {
	if h == nil {
		return ""
	}
	return strings.TrimSpace(h.streamed.String())
}

func (h *agentHooks) BeforeTool(ctx context.Context, state corereact.State, call corereact.ToolCall) error {
	return h.emit(ctx, &flow.ToolCallMessage{
		Tool:       h.displayName(ctx, call.Name),
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
		Tool:        h.displayName(ctx, call.Name),
		Input:       cloneToolInput(call.Input),
		Observation: output.Text,
		Iteration:   state.Iteration,
		ToolCallID:  state.LastDecision.ToolCallID,
	})
}

// displayName 返回供前端展示的工具名称，无法解析时保留内部名称作为兜底。
func (h *agentHooks) displayName(ctx context.Context, name string) string {
	name = strings.TrimSpace(name)
	if h == nil || h.registry == nil || name == "" {
		return name
	}
	tool, ok := h.registry.Lookup(name)
	if !ok || tool == nil {
		return name
	}
	descriptor, err := tool.Describe(ctx)
	if err != nil || descriptor.Annotations == nil {
		return name
	}
	displayName, _ := descriptor.Annotations["display_name"].(string)
	if displayName = strings.TrimSpace(displayName); displayName == "" {
		return name
	}
	return displayName
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
	maps.Copy(out, input)
	return out
}
