package chat

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	corereact "github.com/boxify/api-go/internal/core/agent/react"
	corellm "github.com/boxify/api-go/internal/core/llm"
	coreprompt "github.com/boxify/api-go/internal/core/prompt"
	coretool "github.com/boxify/api-go/internal/core/tool"
	flow "github.com/boxify/api-go/internal/domain/flow"
	"github.com/boxify/api-go/internal/domain/types"
	"github.com/boxify/api-go/internal/infrastructure/security"
	"github.com/boxify/api-go/internal/models"
	appprompts "github.com/boxify/api-go/internal/prompts"
	"github.com/boxify/api-go/internal/prompts/promptsgen"
	"github.com/boxify/api-go/internal/repository"
	"github.com/boxify/api-go/internal/svc"
	"github.com/boxify/api-go/internal/util"
	"github.com/boxify/api-go/internal/xerr"
	"github.com/google/uuid"
)

// 验证聊天 Flow 正常生成时会输出 assistant 和 done 消息。
func TestOrchestratorRunEmitsAssistantAndDone(t *testing.T) {
	userID := uuid.New()
	llmClient := &fakeFlowChatLLMClient{invokeResponses: []fakeFlowInvokeResponse{{
		text: "Thought: done\nFinal Answer: 业务回复",
	}}}
	svcCtx := newFlowChatTestServiceContext(t, userID, llmClient)
	orchestrator := NewOrchestrator(svcCtx)

	messages, err := collectFlowMessages(orchestrator.Run(context.Background(), Input{
		UserID:               userID,
		ConversationID:       uuid.New(),
		CurrentUserMessageID: uuid.New(),
		Message:              "你好",
		Temperature:          0.2,
	}))
	if err != nil {
		t.Fatalf("Orchestrator.Run error = %v, want nil", err)
	}
	if len(messages) != 2 {
		t.Fatalf("messages len = %d, want 2", len(messages))
	}
	assistant, ok := messages[0].(*flow.AssistantMessage)
	if !ok || assistant.Answer != "业务回复" {
		t.Fatalf("first message = %#v, want assistant answer", messages[0])
	}
	if messages[1].Kind() != flow.MessageDone {
		t.Fatalf("second message kind = %q, want %q", messages[1].Kind(), flow.MessageDone)
	}
}

// 验证聊天 Flow 会把历史消息和附件内容传给底层模型。
func TestOrchestratorRunPassesHistoryAndAttachments(t *testing.T) {
	userID := uuid.New()
	conversationID := uuid.New()
	currentUserMessageID := uuid.New()
	llmClient := &fakeFlowChatLLMClient{invokeResponses: []fakeFlowInvokeResponse{{
		text: "Thought: done\nFinal Answer: 带历史回复",
	}}}
	svcCtx := newFlowChatTestServiceContext(t, userID, llmClient)
	svcCtx.MessageRepo.(*fakeFlowMessageRepo).rows = append(svcCtx.MessageRepo.(*fakeFlowMessageRepo).rows,
		&models.Message{ID: uuid.New(), ConversationID: conversationID, Role: string(corellm.UserRole), Content: "上一问", CreatedAt: time.Now().Add(-2 * time.Minute)},
		&models.Message{ID: uuid.New(), ConversationID: conversationID, Role: string(corellm.AssistantRole), Content: "上一答", CreatedAt: time.Now().Add(-1 * time.Minute)},
		&models.Message{ID: currentUserMessageID, ConversationID: conversationID, Role: string(corellm.UserRole), Content: "当前问题", CreatedAt: time.Now()},
	)
	orchestrator := NewOrchestrator(svcCtx)

	_, err := collectFlowMessages(orchestrator.Run(context.Background(), Input{
		UserID:               userID,
		ConversationID:       conversationID,
		CurrentUserMessageID: currentUserMessageID,
		Message:              "继续",
		Temperature:          0.2,
		Attachments: []*types.MessageAttachment{{
			FileName: "note.txt",
			Content:  "附件正文",
		}},
	}))
	if err != nil {
		t.Fatalf("Orchestrator.Run error = %v, want nil", err)
	}
	contents := llmClient.messageContents()
	for _, want := range []string{"上一问", "上一答", "继续", "附件正文"} {
		if !containsFlowText(contents, want) {
			t.Fatalf("model messages = %#v, want contain %q", contents, want)
		}
	}
	if containsFlowText(contents, "当前问题") {
		t.Fatalf("model messages = %#v, should not contain current persisted user message", contents)
	}
}

// 验证点：聊天 Flow 应通过 SystemPrompt 向 ReAct core 注入 Cove 应用身份。
func TestOrchestratorRunInjectsCoveIdentityThroughSystemPrompt(t *testing.T) {
	userID := uuid.New()
	llmClient := &fakeFlowChatLLMClient{invokeResponses: []fakeFlowInvokeResponse{{
		text: "Thought: done\nFinal Answer: Cove 回复",
	}}}
	svcCtx := newFlowChatTestServiceContext(t, userID, llmClient)
	orchestrator := NewOrchestrator(svcCtx)

	_, err := collectFlowMessages(orchestrator.Run(context.Background(), Input{
		UserID:               userID,
		ConversationID:       uuid.New(),
		CurrentUserMessageID: uuid.New(),
		Message:              "你好",
		Temperature:          0.2,
	}))
	if err != nil {
		t.Fatalf("Orchestrator.Run(cove intro) error = %v, want nil", err)
	}
	contents := llmClient.messageContents()
	if !containsFlowText(contents, "你是「Cove」的智能助手") {
		t.Fatalf("model messages = %#v, want Cove assistant intro", contents)
	}
	if containsFlowText(contents, "彗记") {
		t.Fatalf("model messages = %#v, should not contain old app name", contents)
	}
}

// 验证点：聊天 Flow 应把 Cove 身份和用户配置的人设统一通过 SystemPrompt 传给 ReAct。
func TestOrchestratorRunCombinesCoveIntroAndUserSystemPrompt(t *testing.T) {
	userID := uuid.New()
	llmClient := &fakeFlowChatLLMClient{invokeResponses: []fakeFlowInvokeResponse{{
		text: "Thought: done\nFinal Answer: Cove 回复",
	}}}
	svcCtx := newFlowChatTestServiceContext(t, userID, llmClient)
	orchestrator := NewOrchestrator(svcCtx)

	_, err := collectFlowMessages(orchestrator.Run(context.Background(), Input{
		UserID:               userID,
		ConversationID:       uuid.New(),
		CurrentUserMessageID: uuid.New(),
		Message:              "你好",
		Temperature:          0.2,
		SystemPrompt:         "回答要简洁。",
	}))
	if err != nil {
		t.Fatalf("Orchestrator.Run(combined system prompt) error = %v, want nil", err)
	}
	contents := llmClient.messageContents()
	if !containsFlowText(contents, "你是「Cove」的智能助手") || !containsFlowText(contents, "回答要简洁。") {
		t.Fatalf("model messages = %#v, want Cove intro and user system prompt", contents)
	}
}

// 验证聊天 Flow 开启知识库时只把启用聊天的知识库写入工具上下文。
func TestOrchestratorRunUsesEnabledKnowledgeBases(t *testing.T) {
	userID := uuid.New()
	enabledID := uuid.New()
	disabledID := uuid.New()
	llmClient := &fakeFlowChatLLMClient{invokeResponses: []fakeFlowInvokeResponse{{
		text: "Thought: done\nFinal Answer: 知识库回复",
	}}}
	svcCtx := newFlowChatTestServiceContext(t, userID, llmClient)
	svcCtx.KnowledgeBaseRepo.(*fakeFlowKnowledgeBaseRepo).rows = []*models.KnowledgeBase{
		{ID: enabledID, UserID: userID, ChatEnabled: true},
		{ID: disabledID, UserID: userID, ChatEnabled: false},
	}
	orchestrator := NewOrchestrator(svcCtx)

	_, err := collectFlowMessages(orchestrator.Run(context.Background(), Input{
		UserID:               userID,
		ConversationID:       uuid.New(),
		CurrentUserMessageID: uuid.New(),
		Message:              "查资料",
		EnableKnowledge:      true,
		Temperature:          0.2,
	}))
	if err != nil {
		t.Fatalf("Orchestrator.Run error = %v, want nil", err)
	}
	if !slices.Equal(llmClient.knowledgeBaseIDs(), []uuid.UUID{enabledID}) {
		t.Fatalf("knowledgeBaseIDs = %#v, want enabled only", llmClient.knowledgeBaseIDs())
	}
}

// 验证聊天 Flow 只使用调用方传入的归一化知识库开关。
func TestOrchestratorRunUsesNormalizedKnowledgeFlag(t *testing.T) {
	userID := uuid.New()
	kbID := uuid.New()
	llmClient := &fakeFlowChatLLMClient{invokeResponses: []fakeFlowInvokeResponse{{
		text: "Thought: done\nFinal Answer: 不查知识库",
	}}}
	svcCtx := newFlowChatTestServiceContext(t, userID, llmClient)
	svcCtx.KnowledgeBaseRepo.(*fakeFlowKnowledgeBaseRepo).rows = []*models.KnowledgeBase{
		{ID: kbID, UserID: userID, ChatEnabled: true},
	}
	orchestrator := NewOrchestrator(svcCtx)

	_, err := collectFlowMessages(orchestrator.Run(context.Background(), Input{
		UserID:               userID,
		ConversationID:       uuid.New(),
		CurrentUserMessageID: uuid.New(),
		Message:              "不要查资料",
		EnableKnowledge:      false,
		Temperature:          0.2,
	}))
	if err != nil {
		t.Fatalf("Orchestrator.Run error = %v, want nil", err)
	}
	if got := llmClient.knowledgeBaseIDs(); len(got) != 0 {
		t.Fatalf("knowledgeBaseIDs = %#v, want empty when request disables knowledge", got)
	}
}

// 验证模型失败时聊天 Flow 会输出携带部分回复的 error 消息。
func TestOrchestratorRunEmitsErrorWithPartial(t *testing.T) {
	userID := uuid.New()
	llmClient := &fakeFlowChatLLMClient{invokeResponses: []fakeFlowInvokeResponse{{
		text: "部分回复",
		err:  errors.New("model unavailable"),
	}}}
	svcCtx := newFlowChatTestServiceContext(t, userID, llmClient)
	orchestrator := NewOrchestrator(svcCtx)

	messages, err := collectFlowMessages(orchestrator.Run(context.Background(), Input{
		UserID:               userID,
		ConversationID:       uuid.New(),
		CurrentUserMessageID: uuid.New(),
		Message:              "失败",
		Temperature:          0.2,
	}))
	if err != nil {
		t.Fatalf("Orchestrator.Run error = %v, want nil", err)
	}
	if len(messages) != 1 {
		t.Fatalf("messages len = %d, want 1", len(messages))
	}
	errMsg, ok := messages[0].(*flow.ErrorMessage)
	if !ok {
		t.Fatalf("message = %#v, want ErrorMessage", messages[0])
	}
	if errMsg.Partial != "部分回复" || !strings.Contains(errMsg.Message, "model unavailable") {
		t.Fatalf("error message = %#v, want partial and model error", errMsg)
	}
}

// 验证聊天 Flow 会在工具成功调用时依次输出工具开始、工具结束、助手和完成消息。
func TestOrchestratorRunEmitsToolCallAndResultMessages(t *testing.T) {
	userID := uuid.New()
	llmClient := &fakeFlowChatLLMClient{invokeResponses: []fakeFlowInvokeResponse{
		{text: "Thought: need time\nAction: current_time\nAction Input: {}"},
		{text: "Thought: observed\nFinal Answer: 时间回复"},
	}}
	svcCtx := newFlowChatTestServiceContext(t, userID, llmClient)
	orchestrator := NewOrchestrator(svcCtx)

	messages, err := collectFlowMessages(orchestrator.Run(context.Background(), Input{
		UserID:               userID,
		ConversationID:       uuid.New(),
		CurrentUserMessageID: uuid.New(),
		Message:              "现在几点",
		Temperature:          0.2,
	}))
	if err != nil {
		t.Fatalf("Orchestrator.Run(tool success) error = %v, want nil", err)
	}
	if gotKinds := flowMessageKinds(messages); !slices.Equal(gotKinds, []flow.MessageKind{flow.MessageToolCall, flow.MessageToolResult, flow.MessageAssistant, flow.MessageDone}) {
		t.Fatalf("message kinds = %#v, want tool_call/tool_result/assistant/done", gotKinds)
	}
	call, ok := messages[0].(*flow.ToolCallMessage)
	if !ok || call.Tool != "current_time" || call.Iteration != 1 {
		t.Fatalf("tool call message = %#v, want current_time iteration 1", messages[0])
	}
	result, ok := messages[1].(*flow.ToolResultMessage)
	if !ok || result.Tool != "current_time" || result.Observation == "" || result.Error != "" || result.Iteration != 1 {
		t.Fatalf("tool result message = %#v, want current_time observation without error", messages[1])
	}
}

// 验证聊天 Flow 会透传 Runner 的错误 observation，不在 Flow 层转换 error 字段。
func TestOrchestratorRunKeepsToolRunnerErrorAsObservation(t *testing.T) {
	userID := uuid.New()
	llmClient := &fakeFlowChatLLMClient{invokeResponses: []fakeFlowInvokeResponse{{
		text: "Thought: call bad tool\nAction: missing_tool\nAction Input: {\"q\":\"x\"}",
	}}}
	svcCtx := newFlowChatTestServiceContext(t, userID, llmClient)
	orchestrator := NewOrchestrator(svcCtx)

	messages, err := collectFlowMessages(orchestrator.Run(context.Background(), Input{
		UserID:               userID,
		ConversationID:       uuid.New(),
		CurrentUserMessageID: uuid.New(),
		Message:              "调用失败工具",
		Temperature:          0.2,
	}))
	if err != nil {
		t.Fatalf("Orchestrator.Run(tool failure) error = %v, want nil", err)
	}
	if gotKinds := flowMessageKinds(messages); !slices.Equal(gotKinds, []flow.MessageKind{flow.MessageToolCall, flow.MessageToolResult, flow.MessageAssistant, flow.MessageDone}) {
		t.Fatalf("message kinds = %#v, want tool_call/tool_result/assistant/done", gotKinds)
	}
	result, ok := messages[1].(*flow.ToolResultMessage)
	if !ok || result.Tool != "missing_tool" || result.Error != "" || !strings.Contains(result.Observation, "missing_tool") {
		t.Fatalf("tool result message = %#v, want missing_tool observation without flow error conversion", messages[1])
	}
}

// 验证 AfterTool 收到真实 toolErr 时直接返回错误，不额外发送工具结束消息。
func TestAgentHooksAfterToolReturnsToolErrorWithoutEmittingResult(t *testing.T) {
	ch := make(chan flow.Message, 1)
	hooks := &agentHooks{events: ch}
	toolErr := errors.New("tool transport failed")

	err := hooks.AfterTool(context.Background(), corereact.State{Iteration: 3}, corereact.ToolCall{
		Name:  "remote_tool",
		Input: map[string]any{"q": "x"},
	}, coretool.Output{Text: "ignored"}, toolErr)

	if !errors.Is(err, toolErr) {
		t.Fatalf("AfterTool error = %v, want toolErr", err)
	}
	if len(ch) != 0 {
		t.Fatalf("emitted messages = %d, want 0 when toolErr is returned directly", len(ch))
	}
}

// 验证 AfterTool 会完整透传工具输出，不在 Flow 层裁剪 observation。
func TestAgentHooksAfterToolKeepsFullObservation(t *testing.T) {
	ch := make(chan flow.Message, 1)
	hooks := &agentHooks{events: ch}
	fullObservation := strings.Repeat("长", 4100)

	err := hooks.AfterTool(context.Background(), corereact.State{Iteration: 2}, corereact.ToolCall{
		Name: "long_tool",
	}, coretool.Output{Text: fullObservation}, nil)

	if err != nil {
		t.Fatalf("AfterTool error = %v, want nil", err)
	}
	message := <-ch
	result, ok := message.(*flow.ToolResultMessage)
	if !ok {
		t.Fatalf("emitted message = %#v, want ToolResultMessage", message)
	}
	if result.Observation != fullObservation {
		t.Fatalf("observation len = %d, want full len %d", len([]rune(result.Observation)), len([]rune(fullObservation)))
	}
}

func collectFlowMessages(ch <-chan flow.Message, err error) ([]flow.Message, error) {
	if err != nil {
		return nil, err
	}
	out := []flow.Message{}
	for msg := range ch {
		out = append(out, msg)
	}
	return out, nil
}

func flowMessageKinds(messages []flow.Message) []flow.MessageKind {
	out := make([]flow.MessageKind, 0, len(messages))
	for _, message := range messages {
		if message == nil {
			out = append(out, "")
			continue
		}
		out = append(out, message.Kind())
	}
	return out
}

func containsFlowText(values []string, want string) bool {
	for _, value := range values {
		if strings.Contains(value, want) {
			return true
		}
	}
	return false
}

func newFlowChatTestServiceContext(t *testing.T, userID uuid.UUID, llmClient *fakeFlowChatLLMClient) *svc.ServiceContext {
	t.Helper()
	cipher, err := security.NewSecretCipher("0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatalf("NewSecretCipher error = %v", err)
	}
	encrypted, err := cipher.Encrypt("test-key")
	if err != nil {
		t.Fatalf("Encrypt error = %v", err)
	}
	llmManager := corellm.NewManager()
	llmManager.Register("fake", fakeFlowLLMFactory{client: llmClient})
	promptManager := coreprompt.NewManager()
	if err := appprompts.Register(promptManager); err != nil {
		t.Fatalf("Register prompts error = %v, want nil", err)
	}
	return &svc.ServiceContext{
		SecretCipher:      cipher,
		LLMManager:        llmManager,
		ModelConfigRepo:   &fakeFlowModelConfigRepo{rows: []*models.ModelConfig{{ID: uuid.New(), UserID: userID, Type: string(types.ChatModelType), Provider: "fake", ModelName: "fake-chat", APIKeyEncrypted: encrypted, IsDefault: true}}},
		MessageRepo:       &fakeFlowMessageRepo{},
		KnowledgeBaseRepo: &fakeFlowKnowledgeBaseRepo{},
		PromptManager:     promptManager,
		PromptClient:      promptsgen.NewClient(promptManager),
	}
}

type fakeFlowInvokeResponse struct {
	text string
	err  error
}

type fakeFlowChatLLMClient struct {
	mu              sync.Mutex
	invokeResponses []fakeFlowInvokeResponse
	messages        [][]*corellm.Message
	kbIDs           [][]uuid.UUID
}

func (c *fakeFlowChatLLMClient) Invoke(ctx context.Context, messages []*corellm.Message, opts ...corellm.ModelCallOption) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.messages = append(c.messages, append([]*corellm.Message(nil), messages...))
	kbIDs, _ := util.KnowledgeBaseIDsFromContext(ctx)
	c.kbIDs = append(c.kbIDs, append([]uuid.UUID(nil), kbIDs...))
	if len(c.invokeResponses) == 0 {
		return "Thought: done\nFinal Answer: fallback", nil
	}
	resp := c.invokeResponses[0]
	c.invokeResponses = c.invokeResponses[1:]
	return resp.text, resp.err
}

func (c *fakeFlowChatLLMClient) InvokeResult(ctx context.Context, messages []*corellm.Message, opts ...corellm.ModelCallOption) (*corellm.LLMResult, error) {
	text, err := c.Invoke(ctx, messages, opts...)
	if err != nil {
		return nil, err
	}
	return &corellm.LLMResult{Text: text}, nil
}

func (c *fakeFlowChatLLMClient) Stream(ctx context.Context, messages []*corellm.Message, opts ...corellm.ModelCallOption) (<-chan string, error) {
	ch := make(chan string)
	close(ch)
	return ch, nil
}

func (c *fakeFlowChatLLMClient) Embed(ctx context.Context, texts []string, dimensions int, opts ...corellm.EmbeddingOption) ([][]float64, error) {
	return nil, nil
}

func (c *fakeFlowChatLLMClient) EmbedOne(ctx context.Context, text string, dimensions int) ([]float64, error) {
	return nil, nil
}

func (c *fakeFlowChatLLMClient) messageContents() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := []string{}
	for _, call := range c.messages {
		for _, message := range call {
			out = append(out, message.Content)
		}
	}
	return out
}

func (c *fakeFlowChatLLMClient) knowledgeBaseIDs() []uuid.UUID {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.kbIDs) == 0 {
		return nil
	}
	return append([]uuid.UUID(nil), c.kbIDs[len(c.kbIDs)-1]...)
}

type fakeFlowLLMFactory struct {
	client corellm.Client
}

func (f fakeFlowLLMFactory) NewClient(cfg corellm.ModelConfig) (corellm.Client, error) {
	return f.client, nil
}

type fakeFlowModelConfigRepo struct {
	rows []*models.ModelConfig
}

func (r *fakeFlowModelConfigRepo) Create(ctx context.Context, row *models.ModelConfig) (*models.ModelConfig, error) {
	return row, nil
}

func (r *fakeFlowModelConfigRepo) Update(ctx context.Context, row *models.ModelConfig) (*models.ModelConfig, error) {
	return row, nil
}

func (r *fakeFlowModelConfigRepo) Delete(ctx context.Context, id uuid.UUID) error {
	return nil
}

func (r *fakeFlowModelConfigRepo) List(ctx context.Context, userID uuid.UUID, modelType *types.ModelType) ([]*models.ModelConfig, error) {
	out := []*models.ModelConfig{}
	for _, row := range r.rows {
		if row.UserID == userID && (modelType == nil || row.Type == string(*modelType)) {
			out = append(out, row)
		}
	}
	return out, nil
}

func (r *fakeFlowModelConfigRepo) FindByID(ctx context.Context, userID uuid.UUID, configID uuid.UUID) (*models.ModelConfig, error) {
	return nil, xerr.NotFound("模型配置不存在")
}

type fakeFlowMessageRepo struct {
	rows []*models.Message
}

func (r *fakeFlowMessageRepo) Create(ctx context.Context, userID uuid.UUID, row *models.Message) (*models.Message, error) {
	return row, nil
}

func (r *fakeFlowMessageRepo) List(ctx context.Context, userID uuid.UUID) ([]*models.Message, error) {
	return append([]*models.Message(nil), r.rows...), nil
}

func (r *fakeFlowMessageRepo) ListByConversationID(ctx context.Context, userID uuid.UUID, conversationID uuid.UUID) ([]*models.Message, error) {
	out := []*models.Message{}
	for _, row := range r.rows {
		if row.ConversationID == conversationID {
			out = append(out, row)
		}
	}
	slices.SortFunc(out, func(a, b *models.Message) int {
		return a.CreatedAt.Compare(b.CreatedAt)
	})
	return out, nil
}

func (r *fakeFlowMessageRepo) FindByID(ctx context.Context, userID uuid.UUID, messageID uuid.UUID) (*models.Message, error) {
	return nil, xerr.NotFound("消息不存在")
}

func (r *fakeFlowMessageRepo) Update(ctx context.Context, userID uuid.UUID, row *models.Message) (*models.Message, error) {
	return row, nil
}

func (r *fakeFlowMessageRepo) UpdateFields(ctx context.Context, userID uuid.UUID, messageID uuid.UUID, row *models.Message, fields *repository.MessageUpdateFields) (*models.Message, error) {
	return row, nil
}

func (r *fakeFlowMessageRepo) Delete(ctx context.Context, userID uuid.UUID, messageID uuid.UUID) error {
	return nil
}

func (r *fakeFlowMessageRepo) Count(ctx context.Context, conversationID uuid.UUID) (int64, error) {
	return 0, nil
}

type fakeFlowKnowledgeBaseRepo struct {
	rows []*models.KnowledgeBase
}

func (r *fakeFlowKnowledgeBaseRepo) Create(ctx context.Context, userID uuid.UUID, row *models.KnowledgeBase) (*models.KnowledgeBase, error) {
	return row, nil
}

func (r *fakeFlowKnowledgeBaseRepo) List(ctx context.Context, userID uuid.UUID) ([]*models.KnowledgeBase, error) {
	out := []*models.KnowledgeBase{}
	for _, row := range r.rows {
		if row.UserID == userID {
			out = append(out, row)
		}
	}
	return out, nil
}

func (r *fakeFlowKnowledgeBaseRepo) FindDefault(ctx context.Context, userID uuid.UUID) (*models.KnowledgeBase, error) {
	return nil, xerr.NotFound("默认知识库不存在")
}

func (r *fakeFlowKnowledgeBaseRepo) FindByID(ctx context.Context, userID uuid.UUID, knowledgeBaseID uuid.UUID) (*models.KnowledgeBase, error) {
	return nil, xerr.NotFound("知识库不存在")
}

func (r *fakeFlowKnowledgeBaseRepo) Update(ctx context.Context, userID uuid.UUID, row *models.KnowledgeBase) (*models.KnowledgeBase, error) {
	return row, nil
}

func (r *fakeFlowKnowledgeBaseRepo) UpdateFields(ctx context.Context, userID uuid.UUID, knowledgeBaseID uuid.UUID, row *models.KnowledgeBase, fields *repository.KnowledgeBaseUpdateFields) (*models.KnowledgeBase, error) {
	return row, nil
}

func (r *fakeFlowKnowledgeBaseRepo) Delete(ctx context.Context, userID uuid.UUID, knowledgeBaseID uuid.UUID) error {
	return nil
}
