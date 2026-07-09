package chat

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	corellm "github.com/boxify/api-go/internal/core/llm"
	coreprompt "github.com/boxify/api-go/internal/core/prompt"
	"github.com/boxify/api-go/internal/domain/types"
	"github.com/boxify/api-go/internal/infrastructure/realtime"
	"github.com/boxify/api-go/internal/infrastructure/security"
	"github.com/boxify/api-go/internal/models"
	appprompts "github.com/boxify/api-go/internal/prompts"
	"github.com/boxify/api-go/internal/prompts/promptsgen"
	"github.com/boxify/api-go/internal/repository"
	"github.com/boxify/api-go/internal/svc"
	"github.com/boxify/api-go/internal/transport/http/request"
	"github.com/boxify/api-go/internal/xerr"
	"github.com/google/uuid"
)

// 验证 ChatStream 会创建会话、落库用户消息、后台生成助手消息并发布 done。
func TestChatStreamCreatesMessagesAndPublishesDone(t *testing.T) {
	userID := uuid.New()
	llmClient := &fakeChatLLMClient{invokeResponses: []fakeInvokeResponse{{
		text: "Thought: done\nFinal Answer: 真实回复",
	}}}
	svcCtx := newChatStreamTestServiceContext(t, userID, llmClient)
	logic := NewChatStreamLogic(context.Background(), svcCtx)

	events, err := logic.ChatStream(userID, &request.ChatStreamRequest{Message: "你好"})
	if err != nil {
		t.Fatalf("ChatStream error = %v, want nil", err)
	}
	got := collectChatEvents(t, events)
	if !hasChatEvent(got, types.EventTypeMeta) || !hasChatEvent(got, types.EventTypeToken) || !hasChatEvent(got, types.EventTypeDone) {
		t.Fatalf("ChatStream events = %#v, want meta/token/done", eventNames(got))
	}

	msgRepo := svcCtx.MessageRepo.(*fakeChatMessageRepo)
	if len(msgRepo.rows) != 2 {
		t.Fatalf("messages len = %d, want 2", len(msgRepo.rows))
	}
	if msgRepo.rows[0].Role != string(corellm.UserRole) || msgRepo.rows[0].Content != "你好" {
		t.Fatalf("user message = %+v, want role user and content", msgRepo.rows[0])
	}
	if msgRepo.rows[1].Role != string(corellm.AssistantRole) || msgRepo.rows[1].Content != "真实回复" {
		t.Fatalf("assistant message = %+v, want generated answer", msgRepo.rows[1])
	}
}

// 验证 ChatStream 会复用已有会话，并把历史消息传给模型。
func TestChatStreamReusesConversationAndBuildsHistory(t *testing.T) {
	userID := uuid.New()
	conversationID := uuid.New()
	llmClient := &fakeChatLLMClient{invokeResponses: []fakeInvokeResponse{{
		text: "Thought: done\nFinal Answer: 带历史回复",
	}}}
	svcCtx := newChatStreamTestServiceContext(t, userID, llmClient)
	svcCtx.ConversationRepo.(*fakeChatConversationRepo).rows = append(svcCtx.ConversationRepo.(*fakeChatConversationRepo).rows, &models.Conversation{
		ID:     conversationID,
		UserID: userID,
		Title:  "旧会话",
	})
	svcCtx.MessageRepo.(*fakeChatMessageRepo).rows = append(svcCtx.MessageRepo.(*fakeChatMessageRepo).rows,
		&models.Message{ID: uuid.New(), ConversationID: conversationID, Role: string(corellm.UserRole), Content: "上一问", CreatedAt: time.Now().Add(-2 * time.Minute)},
		&models.Message{ID: uuid.New(), ConversationID: conversationID, Role: string(corellm.AssistantRole), Content: "上一答", CreatedAt: time.Now().Add(-1 * time.Minute)},
	)
	logic := NewChatStreamLogic(context.Background(), svcCtx)

	events, err := logic.ChatStream(userID, &request.ChatStreamRequest{ConversationID: conversationID.String(), Message: "继续"})
	if err != nil {
		t.Fatalf("ChatStream(existing) error = %v, want nil", err)
	}
	_ = collectChatEvents(t, events)

	msgRepo := svcCtx.MessageRepo.(*fakeChatMessageRepo)
	if len(msgRepo.rows) != 4 {
		t.Fatalf("messages len = %d, want 4", len(msgRepo.rows))
	}
	if svcCtx.ConversationRepo.(*fakeChatConversationRepo).created != 0 {
		t.Fatalf("created conversations = %d, want 0", svcCtx.ConversationRepo.(*fakeChatConversationRepo).created)
	}
	if !llmClient.containsMessage("上一问") || !llmClient.containsMessage("上一答") {
		t.Fatalf("model messages = %#v, want previous history", llmClient.messageContents())
	}
}

// 验证后台生成失败时会发布 error，并保存可观察到的部分模型输出。
func TestChatStreamSavesPartialAssistantMessageOnGenerationError(t *testing.T) {
	userID := uuid.New()
	llmClient := &fakeChatLLMClient{invokeResponses: []fakeInvokeResponse{{
		text: "部分回复",
		err:  errors.New("model unavailable"),
	}}}
	svcCtx := newChatStreamTestServiceContext(t, userID, llmClient)
	logic := NewChatStreamLogic(context.Background(), svcCtx)

	events, err := logic.ChatStream(userID, &request.ChatStreamRequest{Message: "失败也要保存"})
	if err != nil {
		t.Fatalf("ChatStream(error path) error = %v, want nil", err)
	}
	got := collectChatEvents(t, events)
	if !hasChatEvent(got, types.EventTypeError) {
		t.Fatalf("ChatStream events = %#v, want error", eventNames(got))
	}
	msgRepo := svcCtx.MessageRepo.(*fakeChatMessageRepo)
	if len(msgRepo.rows) != 2 {
		t.Fatalf("messages len = %d, want user plus partial assistant", len(msgRepo.rows))
	}
	partial := msgRepo.rows[1]
	if partial.Role != string(corellm.AssistantRole) || partial.Content != "部分回复" || partial.MetaData == nil || !partial.MetaData.Interrupted {
		t.Fatalf("partial message = %+v, metadata=%+v; want interrupted assistant", partial, partial.MetaData)
	}
}

// 验证 ChatStream 会把工具开始和结束 Flow 消息转换为实时事件，并把工具结果写入消息元数据。
func TestChatStreamPublishesToolEventsAndStoresToolMetadata(t *testing.T) {
	userID := uuid.New()
	llmClient := &fakeChatLLMClient{invokeResponses: []fakeInvokeResponse{
		{text: "Thought: need time\nAction: current_time\nAction Input: {}"},
		{text: "Thought: observed\nFinal Answer: 工具回复"},
	}}
	svcCtx := newChatStreamTestServiceContext(t, userID, llmClient)
	logic := NewChatStreamLogic(context.Background(), svcCtx)

	events, err := logic.ChatStream(userID, &request.ChatStreamRequest{Message: "现在几点"})
	if err != nil {
		t.Fatalf("ChatStream(tool path) error = %v, want nil", err)
	}
	got := collectChatEvents(t, events)
	if !hasChatEvent(got, types.EventTypeToolCall) || !hasChatEvent(got, types.EventTypeToolResult) || !hasChatEvent(got, types.EventTypeDone) {
		t.Fatalf("ChatStream events = %#v, want tool_call/tool_result/done", eventNames(got))
	}
	toolResult, ok := firstToolEvent(got, types.EventTypeToolResult)
	if !ok || toolResult.Tool != "current_time" || toolResult.Observation == "" || toolResult.Error != "" {
		t.Fatalf("tool result event = %#v, want current_time observation without error", toolResult)
	}

	msgRepo := svcCtx.MessageRepo.(*fakeChatMessageRepo)
	if len(msgRepo.rows) != 2 {
		t.Fatalf("messages len = %d, want 2", len(msgRepo.rows))
	}
	assistant := msgRepo.rows[1]
	if assistant.MetaData == nil || len(assistant.MetaData.ToolCalls) != 1 {
		t.Fatalf("assistant metadata = %+v, want one tool call", assistant.MetaData)
	}
	if assistant.MetaData.ToolCalls[0].Tool != "current_time" || assistant.MetaData.ToolCalls[0].Observation == "" {
		t.Fatalf("tool metadata = %+v, want current_time observation", assistant.MetaData.ToolCalls[0])
	}
}

// 验证 resolveChatRuntimeConfig 会让请求层显式知识库开关覆盖 AgentConfig 默认值。
func TestResolveChatRuntimeConfigUsesRequestKnowledgeOverride(t *testing.T) {
	enabled := true
	disabled := false
	agentConfig := &models.AgentConfig{EnableKnowledge: true, Temperature: 0.3}

	if got := resolveChatRuntimeConfig(&request.ChatStreamRequest{EnableKnowledge: &disabled}, agentConfig); got.EnableKnowledge {
		t.Fatalf("resolveChatRuntimeConfig(explicit false).EnableKnowledge = %v, want false", got.EnableKnowledge)
	}
	if got := resolveChatRuntimeConfig(&request.ChatStreamRequest{EnableKnowledge: &enabled}, &models.AgentConfig{EnableKnowledge: false}); !got.EnableKnowledge {
		t.Fatalf("resolveChatRuntimeConfig(explicit true).EnableKnowledge = %v, want true", got.EnableKnowledge)
	}
	if got := resolveChatRuntimeConfig(&request.ChatStreamRequest{}, agentConfig); !got.EnableKnowledge {
		t.Fatalf("resolveChatRuntimeConfig(agent default).EnableKnowledge = %v, want true", got.EnableKnowledge)
	}
	if got := resolveChatRuntimeConfig(&request.ChatStreamRequest{}, nil); got.EnableKnowledge {
		t.Fatalf("resolveChatRuntimeConfig(no defaults).EnableKnowledge = %v, want false", got.EnableKnowledge)
	}
}

// 验证 resolveChatRuntimeConfig 会在 logic 层合并温度默认值和系统提示词。
func TestResolveChatRuntimeConfigDefaultsTemperatureAndPrompt(t *testing.T) {
	if got := resolveChatRuntimeConfig(&request.ChatStreamRequest{}, &models.AgentConfig{Temperature: 0.4, SystemPrompt: " prompt "}); got.Temperature != 0.4 || got.SystemPrompt != "prompt" {
		t.Fatalf("resolveChatRuntimeConfig(agent config) = %+v, want temperature 0.4 and trimmed prompt", got)
	}
	if got := resolveChatRuntimeConfig(&request.ChatStreamRequest{}, &models.AgentConfig{}); got.Temperature != defaultChatTemperature {
		t.Fatalf("resolveChatRuntimeConfig(default temperature).Temperature = %v, want %v", got.Temperature, defaultChatTemperature)
	}
	if got := resolveChatRuntimeConfig(&request.ChatStreamRequest{}, nil); got.Temperature != defaultChatTemperature {
		t.Fatalf("resolveChatRuntimeConfig(nil config).Temperature = %v, want %v", got.Temperature, defaultChatTemperature)
	}
}

func newChatStreamTestServiceContext(t *testing.T, userID uuid.UUID, llmClient *fakeChatLLMClient) *svc.ServiceContext {
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
	llmManager.Register("fake", fakeChatLLMFactory{client: llmClient})
	promptManager := coreprompt.NewManager()
	if err := appprompts.Register(promptManager); err != nil {
		t.Fatalf("Register prompts error = %v, want nil", err)
	}
	return &svc.ServiceContext{
		SecretCipher:      cipher,
		LLMManager:        llmManager,
		ModelConfigRepo:   &fakeChatModelConfigRepo{rows: []*models.ModelConfig{{ID: uuid.New(), UserID: userID, Type: string(types.ChatModelType), Provider: "fake", ModelName: "fake-chat", APIKeyEncrypted: encrypted, IsDefault: true}}},
		AgentConfigRepo:   &fakeChatAgentConfigRepo{row: &models.AgentConfig{UserID: userID, Temperature: 0.2, EnableKnowledge: true}},
		ConversationRepo:  &fakeChatConversationRepo{},
		MessageRepo:       &fakeChatMessageRepo{},
		KnowledgeBaseRepo: &fakeChatKnowledgeBaseRepo{},
		Realtime:          newFakeChatRealtimeBroker(),
		PromptManager:     promptManager,
		PromptClient:      promptsgen.NewClient(promptManager),
	}
}

func collectChatEvents(t *testing.T, events <-chan types.Event) []types.Event {
	t.Helper()
	timer := time.NewTimer(2 * time.Second)
	defer timer.Stop()
	out := []types.Event{}
	for {
		select {
		case event, ok := <-events:
			if !ok {
				return out
			}
			out = append(out, event)
		case <-timer.C:
			t.Fatalf("timed out waiting for chat events; got %#v", eventNames(out))
		}
	}
}

func hasChatEvent(events []types.Event, eventType string) bool {
	for _, event := range events {
		if event != nil && event.EventName() == eventType {
			return true
		}
	}
	return false
}

func eventNames(events []types.Event) []string {
	out := make([]string, 0, len(events))
	for _, event := range events {
		if event == nil {
			out = append(out, "<nil>")
			continue
		}
		out = append(out, event.EventName())
	}
	return out
}

func firstToolEvent(events []types.Event, eventType string) (*types.ToolEvent, bool) {
	for _, event := range events {
		toolEvent, ok := event.(*types.ToolEvent)
		if ok && toolEvent.EventName() == eventType {
			return toolEvent, true
		}
	}
	return nil, false
}

type fakeInvokeResponse struct {
	text string
	err  error
}

type fakeChatLLMClient struct {
	mu              sync.Mutex
	invokeResponses []fakeInvokeResponse
	messages        [][]*corellm.Message
}

func (c *fakeChatLLMClient) Invoke(ctx context.Context, messages []*corellm.Message, opts ...corellm.ModelCallOption) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.messages = append(c.messages, append([]*corellm.Message(nil), messages...))
	if len(c.invokeResponses) == 0 {
		return "Thought: done\nFinal Answer: fallback", nil
	}
	resp := c.invokeResponses[0]
	c.invokeResponses = c.invokeResponses[1:]
	return resp.text, resp.err
}

func (c *fakeChatLLMClient) InvokeResult(ctx context.Context, messages []*corellm.Message, opts ...corellm.ModelCallOption) (*corellm.LLMResult, error) {
	text, err := c.Invoke(ctx, messages, opts...)
	if err != nil {
		return nil, err
	}
	return &corellm.LLMResult{Text: text}, nil
}

func (c *fakeChatLLMClient) Stream(ctx context.Context, messages []*corellm.Message, opts ...corellm.ModelCallOption) (<-chan string, error) {
	ch := make(chan string)
	close(ch)
	return ch, nil
}

func (c *fakeChatLLMClient) Embed(ctx context.Context, texts []string, dimensions int, opts ...corellm.EmbeddingOption) ([][]float64, error) {
	return nil, nil
}

func (c *fakeChatLLMClient) EmbedOne(ctx context.Context, text string, dimensions int) ([]float64, error) {
	return nil, nil
}

func (c *fakeChatLLMClient) containsMessage(text string) bool {
	for _, content := range c.messageContents() {
		if strings.Contains(content, text) {
			return true
		}
	}
	return false
}

func (c *fakeChatLLMClient) messageContents() []string {
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

type fakeChatLLMFactory struct {
	client corellm.Client
}

func (f fakeChatLLMFactory) NewClient(cfg corellm.ModelConfig) (corellm.Client, error) {
	return f.client, nil
}

type fakeChatRealtimeBroker struct {
	mu   sync.Mutex
	subs map[string][]chan types.Event
}

func newFakeChatRealtimeBroker() *fakeChatRealtimeBroker {
	return &fakeChatRealtimeBroker{subs: map[string][]chan types.Event{}}
}

func (b *fakeChatRealtimeBroker) Publish(ctx context.Context, topic string, event types.Event) error {
	b.mu.Lock()
	subs := append([]chan types.Event(nil), b.subs[topic]...)
	b.mu.Unlock()
	for _, ch := range subs {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case ch <- event:
		}
	}
	return nil
}

func (b *fakeChatRealtimeBroker) Subscribe(ctx context.Context, topic string) (realtime.Subscription, error) {
	ch := make(chan types.Event, 16)
	b.mu.Lock()
	b.subs[topic] = append(b.subs[topic], ch)
	b.mu.Unlock()
	return &fakeChatRealtimeSubscription{events: ch}, nil
}

type fakeChatRealtimeSubscription struct {
	events chan types.Event
	once   sync.Once
}

func (s *fakeChatRealtimeSubscription) Events() <-chan types.Event {
	return s.events
}

func (s *fakeChatRealtimeSubscription) Close(ctx context.Context) error {
	s.once.Do(func() {
		close(s.events)
	})
	return nil
}

type fakeChatModelConfigRepo struct {
	rows []*models.ModelConfig
}

func (r *fakeChatModelConfigRepo) Create(ctx context.Context, row *models.ModelConfig) (*models.ModelConfig, error) {
	return row, nil
}

func (r *fakeChatModelConfigRepo) Update(ctx context.Context, row *models.ModelConfig) (*models.ModelConfig, error) {
	return row, nil
}

func (r *fakeChatModelConfigRepo) Delete(ctx context.Context, id uuid.UUID) error {
	return nil
}

func (r *fakeChatModelConfigRepo) List(ctx context.Context, userID uuid.UUID, modelType *types.ModelType) ([]*models.ModelConfig, error) {
	out := []*models.ModelConfig{}
	for _, row := range r.rows {
		if row.UserID == userID && (modelType == nil || row.Type == string(*modelType)) {
			out = append(out, row)
		}
	}
	return out, nil
}

func (r *fakeChatModelConfigRepo) FindByID(ctx context.Context, userID uuid.UUID, configID uuid.UUID) (*models.ModelConfig, error) {
	return nil, xerr.NotFound("模型配置不存在")
}

type fakeChatAgentConfigRepo struct {
	row *models.AgentConfig
}

func (r *fakeChatAgentConfigRepo) Create(ctx context.Context, userID uuid.UUID, row *models.AgentConfig) (*models.AgentConfig, error) {
	return row, nil
}

func (r *fakeChatAgentConfigRepo) List(ctx context.Context, userID uuid.UUID) ([]*models.AgentConfig, error) {
	return nil, nil
}

func (r *fakeChatAgentConfigRepo) FindByID(ctx context.Context, userID uuid.UUID, agentConfigID uuid.UUID) (*models.AgentConfig, error) {
	return nil, xerr.NotFound("智能体配置不存在")
}

func (r *fakeChatAgentConfigRepo) FindByUserID(ctx context.Context, userID uuid.UUID) (*models.AgentConfig, error) {
	if r.row == nil {
		return nil, xerr.NotFound("智能体配置不存在")
	}
	return r.row, nil
}

func (r *fakeChatAgentConfigRepo) Update(ctx context.Context, userID uuid.UUID, row *models.AgentConfig) (*models.AgentConfig, error) {
	return row, nil
}

func (r *fakeChatAgentConfigRepo) UpdateFields(ctx context.Context, userID uuid.UUID, agentConfigID uuid.UUID, row *models.AgentConfig, fields *repository.AgentConfigUpdateFields) (*models.AgentConfig, error) {
	return row, nil
}

func (r *fakeChatAgentConfigRepo) Delete(ctx context.Context, userID uuid.UUID, agentConfigID uuid.UUID) error {
	return nil
}

type fakeChatConversationRepo struct {
	rows    []*models.Conversation
	created int
}

func (r *fakeChatConversationRepo) Create(ctx context.Context, userID uuid.UUID, row *models.Conversation) (*models.Conversation, error) {
	if row.ID == uuid.Nil {
		row.ID = uuid.New()
	}
	row.UserID = userID
	r.created++
	r.rows = append(r.rows, row)
	return row, nil
}

func (r *fakeChatConversationRepo) List(ctx context.Context, userID uuid.UUID) ([]*models.Conversation, error) {
	return nil, nil
}

func (r *fakeChatConversationRepo) FindByID(ctx context.Context, userID uuid.UUID, conversationID uuid.UUID) (*models.Conversation, error) {
	for _, row := range r.rows {
		if row.UserID == userID && row.ID == conversationID {
			return row, nil
		}
	}
	return nil, xerr.NotFound("会话不存在")
}

func (r *fakeChatConversationRepo) Update(ctx context.Context, userID uuid.UUID, row *models.Conversation) (*models.Conversation, error) {
	return row, nil
}

func (r *fakeChatConversationRepo) UpdateFields(ctx context.Context, userID uuid.UUID, conversationID uuid.UUID, row *models.Conversation, fields *repository.ConversationUpdateFields) (*models.Conversation, error) {
	return row, nil
}

func (r *fakeChatConversationRepo) Delete(ctx context.Context, userID uuid.UUID, conversationID uuid.UUID) error {
	return nil
}

type fakeChatMessageRepo struct {
	rows []*models.Message
}

func (r *fakeChatMessageRepo) Create(ctx context.Context, userID uuid.UUID, row *models.Message) (*models.Message, error) {
	if row.ID == uuid.Nil {
		row.ID = uuid.New()
	}
	if row.CreatedAt.IsZero() {
		row.CreatedAt = time.Now()
	}
	r.rows = append(r.rows, row)
	return row, nil
}

func (r *fakeChatMessageRepo) List(ctx context.Context, userID uuid.UUID) ([]*models.Message, error) {
	return append([]*models.Message(nil), r.rows...), nil
}

func (r *fakeChatMessageRepo) ListByConversationID(ctx context.Context, userID uuid.UUID, conversationID uuid.UUID) ([]*models.Message, error) {
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

func (r *fakeChatMessageRepo) FindByID(ctx context.Context, userID uuid.UUID, messageID uuid.UUID) (*models.Message, error) {
	return nil, xerr.NotFound("消息不存在")
}

func (r *fakeChatMessageRepo) Update(ctx context.Context, userID uuid.UUID, row *models.Message) (*models.Message, error) {
	return row, nil
}

func (r *fakeChatMessageRepo) UpdateFields(ctx context.Context, userID uuid.UUID, messageID uuid.UUID, row *models.Message, fields *repository.MessageUpdateFields) (*models.Message, error) {
	return row, nil
}

func (r *fakeChatMessageRepo) Delete(ctx context.Context, userID uuid.UUID, messageID uuid.UUID) error {
	return nil
}

func (r *fakeChatMessageRepo) Count(ctx context.Context, conversationID uuid.UUID) (int64, error) {
	var count int64
	for _, row := range r.rows {
		if row.ConversationID == conversationID {
			count++
		}
	}
	return count, nil
}

type fakeChatKnowledgeBaseRepo struct {
	rows []*models.KnowledgeBase
}

func (r *fakeChatKnowledgeBaseRepo) Create(ctx context.Context, userID uuid.UUID, row *models.KnowledgeBase) (*models.KnowledgeBase, error) {
	return row, nil
}

func (r *fakeChatKnowledgeBaseRepo) List(ctx context.Context, userID uuid.UUID) ([]*models.KnowledgeBase, error) {
	out := []*models.KnowledgeBase{}
	for _, row := range r.rows {
		if row.UserID == userID {
			out = append(out, row)
		}
	}
	return out, nil
}

func (r *fakeChatKnowledgeBaseRepo) FindDefault(ctx context.Context, userID uuid.UUID) (*models.KnowledgeBase, error) {
	return nil, xerr.NotFound("默认知识库不存在")
}

func (r *fakeChatKnowledgeBaseRepo) FindByID(ctx context.Context, userID uuid.UUID, knowledgeBaseID uuid.UUID) (*models.KnowledgeBase, error) {
	return nil, xerr.NotFound("知识库不存在")
}

func (r *fakeChatKnowledgeBaseRepo) Update(ctx context.Context, userID uuid.UUID, row *models.KnowledgeBase) (*models.KnowledgeBase, error) {
	return row, nil
}

func (r *fakeChatKnowledgeBaseRepo) UpdateFields(ctx context.Context, userID uuid.UUID, knowledgeBaseID uuid.UUID, row *models.KnowledgeBase, fields *repository.KnowledgeBaseUpdateFields) (*models.KnowledgeBase, error) {
	return row, nil
}

func (r *fakeChatKnowledgeBaseRepo) Delete(ctx context.Context, userID uuid.UUID, knowledgeBaseID uuid.UUID) error {
	return nil
}
