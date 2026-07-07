package chat

import (
	"context"
	"errors"
	"strings"

	"log/slog"

	corereact "github.com/boxify/api-go/internal/core/agent/react"
	"github.com/boxify/api-go/internal/core/llm"
	coretool "github.com/boxify/api-go/internal/core/tool"
	flow "github.com/boxify/api-go/internal/domain/flow"
	domaintools "github.com/boxify/api-go/internal/domain/tools"
	"github.com/boxify/api-go/internal/domain/types"
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
	registry, err := o.toolRegistry(runCtx, kbIDs)
	if err != nil {
		return generationResult{}, err
	}

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
		Partial: hooks.lastModelOutput,
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

func (o *Orchestrator) toolRegistry(ctx context.Context, kbIDs []uuid.UUID) (*coretool.Registry, error) {
	if o.svcCtx == nil {
		return coretool.NewRegistry(), nil
	}
	catalog, err := domaintools.NewCatalog(o.svcCtx)
	if err != nil {
		return nil, err
	}
	setNames := []string{domaintools.ToolSetSystem}
	if len(kbIDs) > 0 {
		setNames = append(setNames, domaintools.ToolSetKnowledge)
	}
	return catalog.BuildRegistry(ctx, coretool.Selection{SetNames: setNames})
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
	events          chan<- flow.Message
	lastModelOutput string
}

func (h *agentHooks) AfterModel(ctx context.Context, state corereact.State, output string, modelErr error) error {
	if strings.TrimSpace(output) != "" {
		h.lastModelOutput = strings.TrimSpace(output)
	}
	return nil
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
