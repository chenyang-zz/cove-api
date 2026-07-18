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
	if !hasChatEvent(got, types.EventTypeMeta) || !hasChatEvent(got, types.EventTypeThink) || !hasChatEvent(got, types.EventTypeToken) || !hasChatEvent(got, types.EventTypeDone) {
		t.Fatalf("ChatStream events = %#v, want meta/think/token/done", eventNames(got))
	}
	if thinking, done := countThinkEventStatuses(got); thinking < 1 || done < 1 {
		t.Fatalf("think statuses thinking=%d done=%d, want at least 1 each in %v", thinking, done, eventNames(got))
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
		text: "Thought: interrupted\nFinal Answer: 部分回复",
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
	if !ok || toolResult.Tool != "获取当前时间" || toolResult.Observation == "" || toolResult.Error != "" {
		t.Fatalf("tool result event = %#v, want 获取当前时间 observation without error", toolResult)
	}

	msgRepo := svcCtx.MessageRepo.(*fakeChatMessageRepo)
	if len(msgRepo.rows) != 2 {
		t.Fatalf("messages len = %d, want 2", len(msgRepo.rows))
	}
	assistant := msgRepo.rows[1]
	if assistant.MetaData == nil || len(assistant.MetaData.Parts) < 2 {
		t.Fatalf("assistant metadata = %+v, want tool timeline in parts", assistant.MetaData)
	}
	var sawCall, sawResult bool
	for _, part := range assistant.MetaData.Parts {
		switch part.Type {
		case models.MessagePartTypeToolCall:
			// parts 使用展示名（获取当前时间）
			if part.Tool == "获取当前时间" || part.Tool == "current_time" {
				sawCall = true
			}
		case models.MessagePartTypeToolResult:
			if (part.Tool == "获取当前时间" || part.Tool == "current_time") && part.Observation != "" {
				sawResult = true
			}
		}
	}
	if !sawCall || !sawResult {
		t.Fatalf("assistant parts = %#v, want tool_call and tool_result for time tool", assistant.MetaData.Parts)
	}
	if thinking, done := countThinkEventStatuses(got); thinking < 2 || done < 2 {
		t.Fatalf("think statuses thinking=%d done=%d, want >=2 each for two model rounds in %v", thinking, done, eventNames(got))
	}
}

// 验证 resolveChatRuntimeConfig 会让请求层显式知识库开关覆盖 AgentConfig 默认值。
func TestResolveChatRuntimeConfigUsesRequestKnowledgeOverride(t *testing.T) {
	enabled := true
	disabled := false
	agentConfig := &models.AgentConfig{EnableKnowledge: true, Temperature: 0.3}

	if got := resolveChatRuntimeConfig(&request.ChatStreamRequest{EnableKnowledge: &disabled}, agentConfig, nil); got.EnableKnowledge {
		t.Fatalf("resolveChatRuntimeConfig(explicit false).EnableKnowledge = %v, want false", got.EnableKnowledge)
	}
	if got := resolveChatRuntimeConfig(&request.ChatStreamRequest{EnableKnowledge: &enabled}, &models.AgentConfig{EnableKnowledge: false}, nil); !got.EnableKnowledge {
		t.Fatalf("resolveChatRuntimeConfig(explicit true).EnableKnowledge = %v, want true", got.EnableKnowledge)
	}
	if got := resolveChatRuntimeConfig(&request.ChatStreamRequest{}, agentConfig, nil); !got.EnableKnowledge {
		t.Fatalf("resolveChatRuntimeConfig(agent default).EnableKnowledge = %v, want true", got.EnableKnowledge)
	}
	if got := resolveChatRuntimeConfig(&request.ChatStreamRequest{}, nil, nil); got.EnableKnowledge {
		t.Fatalf("resolveChatRuntimeConfig(no defaults).EnableKnowledge = %v, want false", got.EnableKnowledge)
	}
}

// 验证 resolveChatRuntimeConfig 合并温度，无人格时注入 Cove intro + AgentConfig 提示。
func TestResolveChatRuntimeConfigDefaultsTemperatureAndPrompt(t *testing.T) {
	if got := resolveChatRuntimeConfig(&request.ChatStreamRequest{}, &models.AgentConfig{Temperature: 0.4, SystemPrompt: " prompt "}, nil); got.Temperature != 0.4 || !strings.Contains(got.SystemPrompt, coveAssistantIntro) || !strings.Contains(got.SystemPrompt, "prompt") {
		t.Fatalf("resolveChatRuntimeConfig(agent config) = %+v, want temperature 0.4 and Cove+prompt", got)
	}
	if got := resolveChatRuntimeConfig(&request.ChatStreamRequest{}, &models.AgentConfig{}, nil); got.Temperature != defaultChatTemperature {
		t.Fatalf("resolveChatRuntimeConfig(default temperature).Temperature = %v, want %v", got.Temperature, defaultChatTemperature)
	}
	if got := resolveChatRuntimeConfig(&request.ChatStreamRequest{}, nil, nil); got.Temperature != defaultChatTemperature || got.SystemPrompt != coveAssistantIntro {
		t.Fatalf("resolveChatRuntimeConfig(nil config) = %+v, want default temperature and Cove intro", got)
	}
}

// TestResolveChatRuntimeConfigUsesDatabaseContextPolicy 验证聊天上下文预算只从 AgentConfig 数据库字段构建。
func TestResolveChatRuntimeConfigUsesDatabaseContextPolicy(t *testing.T) {
	agentConfig := &models.AgentConfig{
		ContextEnabled: false, ContextWindowTokens: 65536, ContextOutputReserveTokens: 2048,
		ContextSafetyMarginTokens: 256, ContextTriggerRatio: 0.9, ContextTargetRatio: 0.7,
		ContextKeepRecentTokens: 12000, ContextSummaryMaxTokens: 768,
	}
	got := resolveChatRuntimeConfig(nil, agentConfig, nil)
	if got.ContextPolicy == nil || got.ContextPolicy.Enabled || got.ContextPolicy.WindowTokens != 65536 || got.ContextPolicy.SummaryMaxTokens != 768 {
		t.Fatalf("resolveChatRuntimeConfig().ContextPolicy = %#v, want persisted database policy", got.ContextPolicy)
	}
}

// TestChatAgentConfigUsesDefaultWithoutExplicitID 验证未传配置 ID 时读取当前用户的默认配置。
func TestChatAgentConfigUsesDefaultWithoutExplicitID(t *testing.T) {
	repo := &fakeChatAgentConfigRepo{row: &models.AgentConfig{ID: uuid.New(), UserID: uuid.New(), IsDefault: true}}
	logic := &ChatStreamLogic{svcCtx: &svc.ServiceContext{AgentConfigRepo: repo}}

	config, err := logic.chatAgentConfig(context.Background(), repo.row.UserID, "")
	if err != nil || config != repo.row || repo.findDefaultCalls != 1 || repo.findByIDCalls != 0 {
		t.Fatalf("chatAgentConfig(empty) config=%#v error=%v defaultCalls=%d idCalls=%d, want default row nil 1 0", config, err, repo.findDefaultCalls, repo.findByIDCalls)
	}
}

// TestChatAgentConfigLoadsExplicitOwnedID 验证显式配置 ID 按当前用户加载，并拒绝非法 ID 和其他用户访问。
func TestChatAgentConfigLoadsExplicitOwnedID(t *testing.T) {
	userID := uuid.New()
	row := &models.AgentConfig{ID: uuid.New(), UserID: userID, Temperature: 0.2}
	repo := &fakeChatAgentConfigRepo{row: row}
	logic := &ChatStreamLogic{svcCtx: &svc.ServiceContext{AgentConfigRepo: repo}}

	config, err := logic.chatAgentConfig(context.Background(), userID, row.ID.String())
	if err != nil || config != row || repo.findByIDCalls != 1 {
		t.Fatalf("chatAgentConfig(owner) config=%#v error=%v calls=%d, want row nil 1", config, err, repo.findByIDCalls)
	}
	if _, err := logic.chatAgentConfig(context.Background(), userID, "invalid"); xerr.From(err).Kind != xerr.KindBadRequest {
		t.Fatalf("chatAgentConfig(invalid ID) error=%v, want bad request", err)
	}
	if _, err := logic.chatAgentConfig(context.Background(), uuid.New(), row.ID.String()); xerr.From(err).Kind != xerr.KindNotFound {
		t.Fatalf("chatAgentConfig(other user) error=%v, want not found", err)
	}
}

// 验证有生效人格时 SystemPrompt 含 Soul/Identity（先 Soul），且不注入 Cove intro。
func TestResolveChatRuntimeConfigUsesActivePersona(t *testing.T) {
	persona := &models.AgentPersona{
		Soul:     "# Soul\n温暖",
		Identity: "你是小盒",
	}
	got := resolveChatRuntimeConfig(nil, &models.AgentConfig{SystemPrompt: "extra"}, persona)
	if strings.Contains(got.SystemPrompt, "Cove") {
		t.Fatalf("SystemPrompt = %q, must not inject Cove intro when persona present", got.SystemPrompt)
	}
	if !strings.Contains(got.SystemPrompt, "# Soul\n温暖") || !strings.Contains(got.SystemPrompt, "# Identity\n你是小盒") {
		t.Fatalf("SystemPrompt = %q, want soul then identity sections", got.SystemPrompt)
	}
	if !strings.Contains(got.SystemPrompt, "extra") {
		t.Fatalf("SystemPrompt = %q, want agent config prompt retained", got.SystemPrompt)
	}
	soulIdx := strings.Index(got.SystemPrompt, "# Soul")
	idIdx := strings.Index(got.SystemPrompt, "# Identity")
	if soulIdx < 0 || idIdx < soulIdx {
		t.Fatalf("SystemPrompt order wrong: %q", got.SystemPrompt)
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
		AgentPersonaRepo:  &fakeChatAgentPersonaRepo{},
		ConversationRepo:  &fakeChatConversationRepo{},
		MessageRepo:       &fakeChatMessageRepo{},
		KnowledgeBaseRepo: &fakeChatKnowledgeBaseRepo{},
		ToolConfigRepo:    &fakeChatToolConfigRepo{},
		Realtime:          newFakeChatRealtimeBroker(),
		PromptManager:     promptManager,
		PromptClient:      promptsgen.NewClient(promptManager),
	}
}

type fakeChatAgentPersonaRepo struct {
	active *models.AgentPersona
}

func (r *fakeChatAgentPersonaRepo) Create(_ context.Context, _ uuid.UUID, row *models.AgentPersona) (*models.AgentPersona, error) {
	return row, nil
}
func (r *fakeChatAgentPersonaRepo) List(_ context.Context, _ uuid.UUID) ([]*models.AgentPersona, error) {
	return nil, nil
}
func (r *fakeChatAgentPersonaRepo) FindByID(_ context.Context, _ uuid.UUID, _ uuid.UUID) (*models.AgentPersona, error) {
	return nil, xerr.NotFound("智能体人格不存在")
}
func (r *fakeChatAgentPersonaRepo) FindActive(_ context.Context, _ uuid.UUID) (*models.AgentPersona, error) {
	return r.active, nil
}
func (r *fakeChatAgentPersonaRepo) Update(_ context.Context, _ uuid.UUID, row *models.AgentPersona) (*models.AgentPersona, error) {
	return row, nil
}
func (r *fakeChatAgentPersonaRepo) UpdateFields(_ context.Context, _ uuid.UUID, _ uuid.UUID, row *models.AgentPersona, _ *repository.AgentPersonaUpdateFields) (*models.AgentPersona, error) {
	return row, nil
}
func (r *fakeChatAgentPersonaRepo) Delete(_ context.Context, _ uuid.UUID, _ uuid.UUID) error {
	return nil
}
func (r *fakeChatAgentPersonaRepo) Count(_ context.Context, _ uuid.UUID) (int64, error) {
	return 0, nil
}
func (r *fakeChatAgentPersonaRepo) ActivateByID(_ context.Context, _ uuid.UUID, _ uuid.UUID) error {
	return nil
}

type fakeChatToolConfigRepo struct{}

func (r *fakeChatToolConfigRepo) Create(_ context.Context, _ uuid.UUID, row *models.ToolConfig) (*models.ToolConfig, error) {
	return row, nil
}

func (r *fakeChatToolConfigRepo) List(_ context.Context, _ uuid.UUID) ([]*models.ToolConfig, error) {
	return nil, nil
}

func (r *fakeChatToolConfigRepo) FindByID(_ context.Context, _ uuid.UUID, _ uuid.UUID) (*models.ToolConfig, error) {
	return nil, xerr.NotFound("工具配置不存在")
}

func (r *fakeChatToolConfigRepo) Update(_ context.Context, _ uuid.UUID, row *models.ToolConfig) (*models.ToolConfig, error) {
	return row, nil
}

func (r *fakeChatToolConfigRepo) UpdateFields(_ context.Context, _ uuid.UUID, _ uuid.UUID, row *models.ToolConfig, _ *repository.ToolConfigUpdateFields) (*models.ToolConfig, error) {
	return row, nil
}

func (r *fakeChatToolConfigRepo) Delete(_ context.Context, _ uuid.UUID, _ uuid.UUID) error {
	return nil
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

func countThinkEventStatuses(events []types.Event) (thinking int, done int) {
	for _, event := range events {
		think, ok := event.(*types.ThinkEvent)
		if !ok {
			continue
		}
		switch think.Status {
		case types.ThinkStatusThinking:
			thinking++
		case types.ThinkStatusDone:
			done++
		}
	}
	return thinking, done
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

func (c *fakeChatLLMClient) StreamEvents(ctx context.Context, messages []*corellm.Message, opts ...corellm.ModelCallOption) (<-chan corellm.StreamEvent, error) {
	text, err := c.Invoke(ctx, messages, opts...)
	ch := make(chan corellm.StreamEvent, 2)
	if text != "" {
		ch <- corellm.StreamEvent{Kind: corellm.StreamEventTextDelta, Text: text}
	}
	if err != nil {
		ch <- corellm.StreamEvent{Kind: corellm.StreamEventError, Err: err}
	} else {
		ch <- corellm.StreamEvent{Kind: corellm.StreamEventDone}
	}
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
	row              *models.AgentConfig
	findDefaultCalls int
	findByIDCalls    int
}

func (r *fakeChatAgentConfigRepo) Create(ctx context.Context, userID uuid.UUID, row *models.AgentConfig) (*models.AgentConfig, error) {
	return row, nil
}

func (r *fakeChatAgentConfigRepo) List(ctx context.Context, userID uuid.UUID) ([]*models.AgentConfig, error) {
	return nil, nil
}

func (r *fakeChatAgentConfigRepo) FindByID(ctx context.Context, userID uuid.UUID, agentConfigID uuid.UUID) (*models.AgentConfig, error) {
	r.findByIDCalls++
	if r.row == nil || r.row.ID != agentConfigID || r.row.UserID != userID {
		return nil, xerr.NotFound("智能体配置不存在")
	}
	return r.row, nil
}

func (r *fakeChatAgentConfigRepo) FindDefault(ctx context.Context, userID uuid.UUID) (*models.AgentConfig, error) {
	r.findDefaultCalls++
	if r.row == nil || r.row.UserID != userID || !r.row.IsDefault {
		return nil, xerr.NotFound("默认智能体配置不存在")
	}
	return r.row, nil
}

func (r *fakeChatAgentConfigRepo) SetDefault(ctx context.Context, userID uuid.UUID, agentConfigID uuid.UUID) (*models.AgentConfig, error) {
	if r.row == nil || r.row.UserID != userID || r.row.ID != agentConfigID {
		return nil, xerr.NotFound("智能体配置不存在")
	}
	r.row.IsDefault = true
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

func (r *fakeChatConversationRepo) PageList(ctx context.Context, userID uuid.UUID, query repository.ConversationListQuery) ([]*models.Conversation, int64, error) {
	return nil, 0, nil
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

func (r *fakeChatMessageRepo) ListPage(ctx context.Context, userID uuid.UUID, query repository.MessageListQuery) ([]*models.Message, bool, error) {
	rows, err := r.ListByConversationID(ctx, userID, query.ConversationID)
	if err != nil {
		return nil, false, err
	}
	limit := query.Limit
	if limit < 1 {
		limit = 30
	}
	if len(rows) <= limit {
		return rows, false, nil
	}
	return rows[len(rows)-limit:], true, nil
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

func (r *fakeChatKnowledgeBaseRepo) SetDefault(ctx context.Context, userID uuid.UUID, knowledgeBaseID uuid.UUID) (*models.KnowledgeBase, error) {
	row, err := r.FindByID(ctx, userID, knowledgeBaseID)
	if err != nil {
		return nil, err
	}
	row.IsDefault = true
	return row, nil
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
