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
	coremcp "github.com/boxify/api-go/internal/core/mcp"
	coreprompt "github.com/boxify/api-go/internal/core/prompt"
	coretool "github.com/boxify/api-go/internal/core/tool"
	flow "github.com/boxify/api-go/internal/domain/flow"
	domaintoolmcp "github.com/boxify/api-go/internal/domain/tools/mcp"
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
	gotKinds := flowMessageKinds(messages)
	// think(thinking) → think(done) → partial → assistant → done（done 在首 token 前）
	if !hasFlowMessageKindsInOrder(gotKinds, []flow.MessageKind{
		flow.MessageThink, flow.MessageThink, flow.MessagePartial, flow.MessageAssistant, flow.MessageDone,
	}) {
		t.Fatalf("message kinds = %#v, want think/think/partial/assistant/done", gotKinds)
	}
	thinkStart, ok := messages[0].(*flow.ThinkMessage)
	if !ok || thinkStart.Status != flow.ThinkStatusThinking {
		t.Fatalf("first message = %#v, want think thinking", messages[0])
	}
	thinkEnd, ok := messages[1].(*flow.ThinkMessage)
	if !ok || thinkEnd.Status != flow.ThinkStatusDone {
		t.Fatalf("second message = %#v, want think done before first token", messages[1])
	}
	partial, ok := firstFlowMessageOfType[*flow.PartialMessage](messages)
	if !ok || strings.TrimSpace(partial.Text) != "业务回复" {
		t.Fatalf("partial message = %#v, want assistant token", partial)
	}
	assistant, ok := firstFlowMessageOfType[*flow.AssistantMessage](messages)
	if !ok || assistant.Answer != "业务回复" {
		t.Fatalf("assistant message = %#v, want assistant answer", assistant)
	}
	if thinking, done := countThinkStatuses(messages); thinking != 1 || done != 1 {
		t.Fatalf("think counts thinking=%d done=%d, want 1/1", thinking, done)
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

// 验证点：Orchestrator 透传 logic 传入的 SystemPrompt，不再硬编码 Cove intro。
func TestOrchestratorRunPassesThroughSystemPrompt(t *testing.T) {
	userID := uuid.New()
	llmClient := &fakeFlowChatLLMClient{invokeResponses: []fakeFlowInvokeResponse{{
		text: "Thought: done\nFinal Answer: 回复",
	}}}
	svcCtx := newFlowChatTestServiceContext(t, userID, llmClient)
	orchestrator := NewOrchestrator(svcCtx)

	_, err := collectFlowMessages(orchestrator.Run(context.Background(), Input{
		UserID:               userID,
		ConversationID:       uuid.New(),
		CurrentUserMessageID: uuid.New(),
		Message:              "你好",
		Temperature:          0.2,
		SystemPrompt:         "# Soul\n温暖\n\n# Identity\n你是小盒",
	}))
	if err != nil {
		t.Fatalf("Orchestrator.Run(system prompt) error = %v, want nil", err)
	}
	contents := llmClient.messageContents()
	if !containsFlowText(contents, "# Soul") || !containsFlowText(contents, "你是小盒") {
		t.Fatalf("model messages = %#v, want passed-through persona system prompt", contents)
	}
	if containsFlowText(contents, "你是「Cove」的智能助手") {
		t.Fatalf("model messages = %#v, orchestrator must not inject Cove intro itself", contents)
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

// 验证没有用户配置时，聊天 Flow 默认注册当前上下文允许的全部内置工具。
func TestToolRegistryDefaultsBuiltinToolsEnabled(t *testing.T) {
	userID := uuid.New()
	svcCtx := newFlowChatTestServiceContext(t, userID, &fakeFlowChatLLMClient{})
	orchestrator := NewOrchestrator(svcCtx)

	registry, closeTools, err := orchestrator.toolRegistry(context.Background(), userID, []uuid.UUID{uuid.New()})
	if err != nil {
		t.Fatalf("toolRegistry error = %v, want nil", err)
	}
	if closeTools != nil {
		defer closeTools()
	}
	for _, toolKey := range []string{"current_time", "knowledge_search"} {
		if _, ok := registry.Lookup(toolKey); !ok {
			t.Fatalf("toolRegistry Lookup(%q) = false, want default enabled", toolKey)
		}
	}
}

// 验证用户显式关闭的内置工具不会注册到聊天 Agent。
func TestToolRegistryExcludesPersistedDisabledTool(t *testing.T) {
	userID := uuid.New()
	svcCtx := newFlowChatTestServiceContext(t, userID, &fakeFlowChatLLMClient{})
	svcCtx.ToolConfigRepo.(*fakeFlowToolConfigRepo).rows = []*models.ToolConfig{
		{ID: uuid.New(), UserID: userID, ToolKey: "current_time", Enabled: false},
		{ID: uuid.New(), UserID: userID, ToolKey: "current_time", Enabled: true},
	}
	orchestrator := NewOrchestrator(svcCtx)

	registry, closeTools, err := orchestrator.toolRegistry(context.Background(), userID, nil)
	if err != nil {
		t.Fatalf("toolRegistry error = %v, want nil", err)
	}
	if closeTools != nil {
		defer closeTools()
	}
	if _, ok := registry.Lookup("current_time"); ok {
		t.Fatal("toolRegistry Lookup(current_time) = true, want disabled tool excluded")
	}
}

// 验证本轮未开启知识库时，即使用户配置启用也不会注册知识库工具。
func TestToolRegistryKeepsKnowledgeRequestOverride(t *testing.T) {
	userID := uuid.New()
	svcCtx := newFlowChatTestServiceContext(t, userID, &fakeFlowChatLLMClient{})
	svcCtx.ToolConfigRepo.(*fakeFlowToolConfigRepo).rows = []*models.ToolConfig{{
		ID: uuid.New(), UserID: userID, ToolKey: "knowledge_search", Enabled: true,
	}}
	orchestrator := NewOrchestrator(svcCtx)

	registry, closeTools, err := orchestrator.toolRegistry(context.Background(), userID, nil)
	if err != nil {
		t.Fatalf("toolRegistry error = %v, want nil", err)
	}
	if closeTools != nil {
		defer closeTools()
	}
	if _, ok := registry.Lookup("knowledge_search"); ok {
		t.Fatal("toolRegistry Lookup(knowledge_search) = true, want request override excluded")
	}
}

// 验证用户工具配置查询失败时聊天 Flow 会返回错误。
func TestToolRegistryReturnsToolConfigRepositoryError(t *testing.T) {
	userID := uuid.New()
	svcCtx := newFlowChatTestServiceContext(t, userID, &fakeFlowChatLLMClient{})
	want := errors.New("list tool configs")
	svcCtx.ToolConfigRepo.(*fakeFlowToolConfigRepo).listErr = want
	orchestrator := NewOrchestrator(svcCtx)

	_, _, err := orchestrator.toolRegistry(context.Background(), userID, nil)
	if !errors.Is(err, want) {
		t.Fatalf("toolRegistry error = %v, want %v", err, want)
	}
}

// TestToolRegistryRegistersMCPToolAndClosesSession 验证默认启用的 MCP 工具进入 registry、调用原始工具并在本轮结束时关闭 session。
func TestToolRegistryRegistersMCPToolAndClosesSession(t *testing.T) {
	userID := uuid.New()
	server := &models.MCPServer{ID: uuid.New(), UserID: userID, Name: "搜索服务", Enabled: true}
	session := &fakeFlowMCPSession{
		tools:  []coremcp.ToolInfo{{Name: "search", Description: "搜索"}},
		result: &coremcp.CallResult{Content: []coremcp.Content{{Type: "text", Text: "result"}}},
	}
	opener := &fakeFlowMCPOpener{session: session}
	svcCtx := newFlowChatTestServiceContext(t, userID, &fakeFlowChatLLMClient{})
	svcCtx.MCPServerRepo.(*fakeFlowMCPServerRepo).rows = []*models.MCPServer{server}
	svcCtx.MCPToolService = coremcp.NewService(coremcp.WithSessionOpener(opener))
	orchestrator := NewOrchestrator(svcCtx)

	registry, closeTools, err := orchestrator.toolRegistry(context.Background(), userID, nil)
	if err != nil {
		t.Fatalf("toolRegistry error = %v, want nil", err)
	}
	toolKey := domaintoolmcp.ToolKey(server.ID, "search")
	tool, ok := registry.Lookup(toolKey)
	if !ok {
		t.Fatalf("toolRegistry Lookup(%q) = false, want MCP tool", toolKey)
	}
	output, err := tool.Invoke(context.Background(), coretool.Input{"query": "hello"})
	if err != nil || output.Text != "result" || session.lastName != "search" {
		t.Fatalf("MCP Invoke output/error/name = %+v/%v/%q, want result/nil/search", output, err, session.lastName)
	}
	if closeTools == nil {
		t.Fatal("closeTools = nil, want MCP cleanup")
	}
	if err := closeTools(); err != nil {
		t.Fatalf("closeTools error = %v, want nil", err)
	}
	if session.closeCalls != 1 {
		t.Fatalf("session close calls = %d, want 1", session.closeCalls)
	}
}

// TestToolRegistryMCPServerAndToolSwitchesTakePrecedence 验证 server 总开关和工具级关闭都会阻止 MCP 工具进入 registry。
func TestToolRegistryMCPServerAndToolSwitchesTakePrecedence(t *testing.T) {
	userID := uuid.New()
	for _, testCase := range []struct {
		name          string
		serverEnabled bool
		toolDisabled  bool
	}{
		{name: "server disabled", serverEnabled: false},
		{name: "tool disabled", serverEnabled: true, toolDisabled: true},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			server := &models.MCPServer{
				ID: uuid.New(), UserID: userID, Name: testCase.name, Enabled: testCase.serverEnabled,
				ToolsCache: models.MCPMetas{&models.MCPMeta{Name: "search"}},
			}
			session := &fakeFlowMCPSession{tools: []coremcp.ToolInfo{{Name: "search"}}}
			opener := &fakeFlowMCPOpener{session: session}
			svcCtx := newFlowChatTestServiceContext(t, userID, &fakeFlowChatLLMClient{})
			svcCtx.MCPServerRepo.(*fakeFlowMCPServerRepo).rows = []*models.MCPServer{server}
			svcCtx.MCPToolService = coremcp.NewService(coremcp.WithSessionOpener(opener))
			if testCase.toolDisabled {
				svcCtx.ToolConfigRepo.(*fakeFlowToolConfigRepo).rows = []*models.ToolConfig{{
					ID: uuid.New(), UserID: userID, ToolKey: domaintoolmcp.ToolKey(server.ID, "search"), Enabled: false,
				}}
			}

			registry, closeTools, err := NewOrchestrator(svcCtx).toolRegistry(context.Background(), userID, nil)
			if err != nil {
				t.Fatalf("toolRegistry error = %v, want nil", err)
			}
			if closeTools != nil {
				defer closeTools()
			}
			if _, ok := registry.Lookup(domaintoolmcp.ToolKey(server.ID, "search")); ok {
				t.Fatal("MCP tool registered despite disabled gate")
			}
			if !testCase.serverEnabled && opener.calls != 0 {
				t.Fatalf("disabled server opener calls = %d, want 0", opener.calls)
			}
		})
	}
}

// TestToolRegistrySkipsUnavailableMCPServer 验证单个 MCP server 连接失败只会被跳过，不影响内置工具 registry。
func TestToolRegistrySkipsUnavailableMCPServer(t *testing.T) {
	userID := uuid.New()
	server := &models.MCPServer{ID: uuid.New(), UserID: userID, Name: "故障服务", Enabled: true}
	svcCtx := newFlowChatTestServiceContext(t, userID, &fakeFlowChatLLMClient{})
	svcCtx.MCPServerRepo.(*fakeFlowMCPServerRepo).rows = []*models.MCPServer{server}
	svcCtx.MCPToolService = coremcp.NewService(coremcp.WithSessionOpener(&fakeFlowMCPOpener{err: errors.New("offline")}))

	registry, closeTools, err := NewOrchestrator(svcCtx).toolRegistry(context.Background(), userID, nil)
	if err != nil {
		t.Fatalf("toolRegistry error = %v, want nil", err)
	}
	if closeTools != nil {
		t.Fatal("closeTools != nil, want no opened MCP session")
	}
	if _, ok := registry.Lookup("current_time"); !ok {
		t.Fatal("current_time missing after MCP server failure")
	}
}

// TestToolRegistryOpensMCPServersInParallel 验证多个慢 MCP server 并行发现，墙钟接近单次超时而非 N 倍串行。
func TestToolRegistryOpensMCPServersInParallel(t *testing.T) {
	userID := uuid.New()
	servers := []*models.MCPServer{
		{ID: uuid.New(), UserID: userID, Name: "s1", Enabled: true},
		{ID: uuid.New(), UserID: userID, Name: "s2", Enabled: true},
		{ID: uuid.New(), UserID: userID, Name: "s3", Enabled: true},
	}
	opener := &fakeFlowMCPOpener{block: true}
	svcCtx := newFlowChatTestServiceContext(t, userID, &fakeFlowChatLLMClient{})
	svcCtx.Config.MCP.AssembleBudget = "80ms"
	svcCtx.MCPServerRepo.(*fakeFlowMCPServerRepo).rows = servers
	svcCtx.MCPToolService = coremcp.NewService(
		coremcp.WithSessionOpener(opener),
		coremcp.WithDiscoverTimeout(time.Second),
	)

	start := time.Now()
	registry, closeTools, err := NewOrchestrator(svcCtx).toolRegistry(context.Background(), userID, nil)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("toolRegistry error = %v, want nil", err)
	}
	if closeTools != nil {
		t.Fatal("closeTools != nil, want no leases after all timeouts")
	}
	if _, ok := registry.Lookup("current_time"); !ok {
		t.Fatal("current_time missing after parallel MCP timeout")
	}
	// 串行最坏约 3×budget；并行应接近单次 budget。
	if elapsed > 300*time.Millisecond {
		t.Fatalf("toolRegistry elapsed = %v, want parallel wall clock near budget", elapsed)
	}
	if got := opener.callCount(); got < 1 {
		t.Fatalf("opener calls = %d, want at least 1", got)
	}
}

// TestToolRegistryMCPAssembleConcurrencyCap 验证并行 OpenTools 受并发上限约束。
func TestToolRegistryMCPAssembleConcurrencyCap(t *testing.T) {
	userID := uuid.New()
	servers := make([]*models.MCPServer, 0, 5)
	for i := range 5 {
		servers = append(servers, &models.MCPServer{
			ID: uuid.New(), UserID: userID, Name: "s" + string(rune('a'+i)), Enabled: true,
		})
	}
	opener := &fakeFlowMCPOpener{block: true}
	svcCtx := newFlowChatTestServiceContext(t, userID, &fakeFlowChatLLMClient{})
	svcCtx.Config.MCP.AssembleBudget = "200ms"
	svcCtx.Config.MCP.AssembleConcurrency = 2
	svcCtx.MCPServerRepo.(*fakeFlowMCPServerRepo).rows = servers
	svcCtx.MCPToolService = coremcp.NewService(
		coremcp.WithSessionOpener(opener),
		coremcp.WithDiscoverTimeout(time.Second),
	)

	if _, _, err := NewOrchestrator(svcCtx).toolRegistry(context.Background(), userID, nil); err != nil {
		t.Fatalf("toolRegistry error = %v, want nil", err)
	}
	if peak := opener.maxInFlight(); peak > 2 {
		t.Fatalf("max in-flight OpenSession = %d, want <= 2", peak)
	}
}

// TestToolRegistryRegistersMultipleMCPServers 验证多个 MCP server 并行发现后均可注册并关闭。
func TestToolRegistryRegistersMultipleMCPServers(t *testing.T) {
	userID := uuid.New()
	serverA := &models.MCPServer{ID: uuid.New(), UserID: userID, Name: "A", Enabled: true}
	serverB := &models.MCPServer{ID: uuid.New(), UserID: userID, Name: "B", Enabled: true}
	sessionA := &fakeFlowMCPSession{
		tools:  []coremcp.ToolInfo{{Name: "alpha"}},
		result: &coremcp.CallResult{Content: []coremcp.Content{{Type: "text", Text: "a"}}},
	}
	sessionB := &fakeFlowMCPSession{
		tools:  []coremcp.ToolInfo{{Name: "beta"}},
		result: &coremcp.CallResult{Content: []coremcp.Content{{Type: "text", Text: "b"}}},
	}
	opener := &fakeFlowMCPOpener{
		byServer: map[uuid.UUID]coremcp.ToolSession{
			serverA.ID: sessionA,
			serverB.ID: sessionB,
		},
	}
	svcCtx := newFlowChatTestServiceContext(t, userID, &fakeFlowChatLLMClient{})
	svcCtx.MCPServerRepo.(*fakeFlowMCPServerRepo).rows = []*models.MCPServer{serverA, serverB}
	svcCtx.MCPToolService = coremcp.NewService(coremcp.WithSessionOpener(opener))

	registry, closeTools, err := NewOrchestrator(svcCtx).toolRegistry(context.Background(), userID, nil)
	if err != nil {
		t.Fatalf("toolRegistry error = %v, want nil", err)
	}
	keyA := domaintoolmcp.ToolKey(serverA.ID, "alpha")
	keyB := domaintoolmcp.ToolKey(serverB.ID, "beta")
	if _, ok := registry.Lookup(keyA); !ok {
		t.Fatalf("Lookup(%q) = false, want tool from server A", keyA)
	}
	if _, ok := registry.Lookup(keyB); !ok {
		t.Fatalf("Lookup(%q) = false, want tool from server B", keyB)
	}
	if closeTools == nil {
		t.Fatal("closeTools = nil, want MCP cleanup for both servers")
	}
	if err := closeTools(); err != nil {
		t.Fatalf("closeTools error = %v, want nil", err)
	}
	if sessionA.closeCalls != 1 || sessionB.closeCalls != 1 {
		t.Fatalf("session close calls = %d/%d, want 1/1", sessionA.closeCalls, sessionB.closeCalls)
	}
}

// 验证模型失败时聊天 Flow 会输出携带部分回复的 error 消息。
func TestOrchestratorRunEmitsErrorWithPartial(t *testing.T) {
	userID := uuid.New()
	llmClient := &fakeFlowChatLLMClient{invokeResponses: []fakeFlowInvokeResponse{{
		text: "Thought: interrupted\nFinal Answer: 部分回复",
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
	if gotKinds := flowMessageKinds(messages); !slices.Equal(gotKinds, []flow.MessageKind{
		flow.MessageThink, flow.MessageThink, flow.MessagePartial, flow.MessageError,
	}) {
		t.Fatalf("message kinds = %#v, want think/think/partial/error", gotKinds)
	}
	thinkStart, ok := messages[0].(*flow.ThinkMessage)
	if !ok || thinkStart.Status != flow.ThinkStatusThinking {
		t.Fatalf("first message = %#v, want think thinking", messages[0])
	}
	thinkEnd, ok := messages[1].(*flow.ThinkMessage)
	if !ok || thinkEnd.Status != flow.ThinkStatusDone {
		t.Fatalf("second message = %#v, want think done before first token", messages[1])
	}
	partial, ok := messages[2].(*flow.PartialMessage)
	if !ok || strings.TrimSpace(partial.Text) != "部分回复" {
		t.Fatalf("third message = %#v, want partial token", messages[2])
	}
	errMsg, ok := messages[3].(*flow.ErrorMessage)
	if !ok {
		t.Fatalf("message = %#v, want ErrorMessage", messages[3])
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
	gotKinds := flowMessageKinds(messages)
	// 每轮：thinking → done（首 token 前或无 token 时 AfterModel）
	if !hasFlowMessageKindsInOrder(gotKinds, []flow.MessageKind{
		flow.MessageThink, flow.MessageThink, flow.MessageToolCall, flow.MessageToolResult,
		flow.MessageThink, flow.MessageThink, flow.MessagePartial, flow.MessageAssistant, flow.MessageDone,
	}) {
		t.Fatalf("message kinds = %#v, want think pairs before tool and before final tokens", gotKinds)
	}
	call, ok := firstFlowMessageOfType[*flow.ToolCallMessage](messages)
	if !ok || call.Tool != "获取当前时间" || call.Iteration != 1 {
		t.Fatalf("tool call message = %#v, want display name 获取当前时间 at iteration 1", call)
	}
	result, ok := firstFlowMessageOfType[*flow.ToolResultMessage](messages)
	if !ok || result.Tool != "获取当前时间" || result.Observation == "" || result.Error != "" || result.Iteration != 1 {
		t.Fatalf("tool result message = %#v, want display name 获取当前时间 observation without error", result)
	}
	// 两轮模型调用各有 thinking → done
	if thinking, done := countThinkStatuses(messages); thinking != 2 || done != 2 {
		t.Fatalf("think counts thinking=%d done=%d, want 2/2", thinking, done)
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
	if gotKinds := flowMessageKinds(messages); !hasFlowMessageKindsInOrder(gotKinds, []flow.MessageKind{
		flow.MessageThink, flow.MessageThink, flow.MessageToolCall, flow.MessageToolResult,
	}) {
		t.Fatalf("message kinds = %#v, want think/tool_call/tool_result prefix", gotKinds)
	}
	result, ok := firstFlowMessageOfType[*flow.ToolResultMessage](messages)
	if !ok || result.Tool != "missing_tool" || result.Error != "" || !strings.Contains(result.Observation, "missing_tool") {
		t.Fatalf("tool result message = %#v, want missing_tool observation without flow error conversion", result)
	}
}

// TestOrchestratorRunKeepsMCPIsErrorAsObservation 验证 MCP IsError 经默认 Runner 恢复为完整 observation，不会中断聊天 Flow。
func TestOrchestratorRunKeepsMCPIsErrorAsObservation(t *testing.T) {
	userID := uuid.New()
	server := &models.MCPServer{ID: uuid.New(), UserID: userID, Name: "故障搜索", Enabled: true}
	toolKey := domaintoolmcp.ToolKey(server.ID, "search")
	llmClient := &fakeFlowChatLLMClient{invokeResponses: []fakeFlowInvokeResponse{
		{text: "Thought: search\nAction: " + toolKey + "\nAction Input: {\"query\":\"hello\"}"},
		{text: "Thought: recovered\nFinal Answer: 已处理"},
	}}
	session := &fakeFlowMCPSession{
		tools: []coremcp.ToolInfo{{Name: "search", Title: "网页搜索", Description: "搜索"}},
		result: &coremcp.CallResult{
			Content:           []coremcp.Content{{Type: "text", Text: "远端拒绝请求"}},
			StructuredContent: map[string]any{"code": "bad_request"},
			IsError:           true,
		},
	}
	svcCtx := newFlowChatTestServiceContext(t, userID, llmClient)
	svcCtx.MCPServerRepo.(*fakeFlowMCPServerRepo).rows = []*models.MCPServer{server}
	svcCtx.MCPToolService = coremcp.NewService(coremcp.WithSessionOpener(&fakeFlowMCPOpener{session: session}))

	messages, err := collectFlowMessages(NewOrchestrator(svcCtx).Run(context.Background(), Input{
		UserID: userID, ConversationID: uuid.New(), CurrentUserMessageID: uuid.New(), Message: "搜索", Temperature: 0.2,
	}))
	if err != nil {
		t.Fatalf("Orchestrator.Run error = %v, want nil", err)
	}
	if gotKinds := flowMessageKinds(messages); !hasFlowMessageKindsInOrder(gotKinds, []flow.MessageKind{
		flow.MessageThink, flow.MessageThink, flow.MessageToolCall, flow.MessageToolResult,
	}) {
		t.Fatalf("message kinds = %#v, want think/tool_call/tool_result prefix", gotKinds)
	}
	result, ok := firstFlowMessageOfType[*flow.ToolResultMessage](messages)
	if !ok || result.Tool != "网页搜索" || result.Error != "" || !strings.HasPrefix(result.Observation, "tool invocation failed:\n") || !strings.Contains(result.Observation, "远端拒绝请求") || !strings.Contains(result.Observation, `"code":"bad_request"`) {
		t.Fatalf("tool result message = %#v, want recovered MCP error observation", result)
	}
	if session.closeCalls != 1 {
		t.Fatalf("MCP session close calls = %d, want 1 after flow completion", session.closeCalls)
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

// 验证工具事件优先使用 descriptor 的 display_name，缺失时回退内部工具名称。
func TestAgentHooksUseDisplayNameWithInternalNameFallback(t *testing.T) {
	ctx := context.Background()
	registry := coretool.NewRegistry()
	if err := registry.Register(ctx, coretool.NewFuncTool(coretool.Descriptor{
		Name: "internal_tool",
		Annotations: map[string]any{
			"display_name": "纯展示名",
		},
	}, func(context.Context, coretool.Input) (coretool.Output, error) {
		return coretool.Output{}, nil
	})); err != nil {
		t.Fatalf("Registry.Register(display tool) error = %v, want nil", err)
	}
	hooks := &agentHooks{registry: registry}
	if got := hooks.displayName(ctx, "internal_tool"); got != "纯展示名" {
		t.Fatalf("displayName(annotation) = %q, want 纯展示名", got)
	}
	if got := hooks.displayName(ctx, "missing_tool"); got != "missing_tool" {
		t.Fatalf("displayName(fallback) = %q, want missing_tool", got)
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

func firstFlowMessageOfType[T flow.Message](messages []flow.Message) (T, bool) {
	var zero T
	for _, message := range messages {
		if typed, ok := message.(T); ok {
			return typed, true
		}
	}
	return zero, false
}

func countThinkStatuses(messages []flow.Message) (thinking int, done int) {
	for _, message := range messages {
		think, ok := message.(*flow.ThinkMessage)
		if !ok {
			continue
		}
		switch think.Status {
		case flow.ThinkStatusThinking:
			thinking++
		case flow.ThinkStatusDone:
			done++
		}
	}
	return thinking, done
}

// hasFlowMessageKindsInOrder reports whether want kinds appear as a subsequence of got.
func hasFlowMessageKindsInOrder(got []flow.MessageKind, want []flow.MessageKind) bool {
	if len(want) == 0 {
		return true
	}
	i := 0
	for _, kind := range got {
		if kind == want[i] {
			i++
			if i == len(want) {
				return true
			}
		}
	}
	return false
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
		MCPServerRepo:     &fakeFlowMCPServerRepo{},
		ToolConfigRepo:    &fakeFlowToolConfigRepo{},
		MCPToolService:    coremcp.NewService(coremcp.WithClient(&fakeFlowMCPClient{})),
		PromptManager:     promptManager,
		PromptClient:      promptsgen.NewClient(promptManager),
	}
}

type fakeFlowMCPClient struct {
	tools []coremcp.ToolInfo
	err   error
	calls int
}

func (c *fakeFlowMCPClient) ListTools(context.Context, coremcp.ServerConfig) ([]coremcp.ToolInfo, error) {
	c.calls++
	if c.err != nil {
		return nil, c.err
	}
	return append([]coremcp.ToolInfo(nil), c.tools...), nil
}

type fakeFlowMCPOpener struct {
	mu        sync.Mutex
	session   coremcp.ToolSession
	err       error
	calls     int
	block     bool
	inFlight  int
	maxFlight int
	byServer  map[uuid.UUID]coremcp.ToolSession
	errByID   map[uuid.UUID]error
}

func (o *fakeFlowMCPOpener) OpenSession(ctx context.Context, server coremcp.ServerConfig) (coremcp.ToolSession, error) {
	o.mu.Lock()
	o.calls++
	o.inFlight++
	if o.inFlight > o.maxFlight {
		o.maxFlight = o.inFlight
	}
	block := o.block
	session := o.session
	err := o.err
	if o.byServer != nil {
		if s, ok := o.byServer[server.ID]; ok {
			session = s
		}
	}
	if o.errByID != nil {
		if e, ok := o.errByID[server.ID]; ok {
			err = e
		}
	}
	o.mu.Unlock()

	defer func() {
		o.mu.Lock()
		o.inFlight--
		o.mu.Unlock()
	}()

	if block {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	if err != nil {
		return nil, err
	}
	return session, nil
}

func (o *fakeFlowMCPOpener) callCount() int {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.calls
}

func (o *fakeFlowMCPOpener) maxInFlight() int {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.maxFlight
}

type fakeFlowMCPSession struct {
	tools      []coremcp.ToolInfo
	result     *coremcp.CallResult
	listErr    error
	callErr    error
	closeErr   error
	lastName   string
	lastInput  map[string]any
	closeCalls int
}

func (s *fakeFlowMCPSession) ListTools(context.Context) ([]coremcp.ToolInfo, error) {
	if s.listErr != nil {
		return nil, s.listErr
	}
	return append([]coremcp.ToolInfo(nil), s.tools...), nil
}

func (s *fakeFlowMCPSession) CallTool(_ context.Context, name string, input map[string]any) (*coremcp.CallResult, error) {
	s.lastName = name
	s.lastInput = input
	if s.callErr != nil {
		return nil, s.callErr
	}
	return s.result, nil
}

func (s *fakeFlowMCPSession) Close() error {
	s.closeCalls++
	return s.closeErr
}

type fakeFlowMCPServerRepo struct {
	rows []*models.MCPServer
	err  error
}

func (r *fakeFlowMCPServerRepo) Create(_ context.Context, userID uuid.UUID, row *models.MCPServer) (*models.MCPServer, error) {
	row.UserID = userID
	r.rows = append(r.rows, row)
	return row, nil
}

func (r *fakeFlowMCPServerRepo) List(_ context.Context, userID uuid.UUID) ([]*models.MCPServer, error) {
	if r.err != nil {
		return nil, r.err
	}
	out := make([]*models.MCPServer, 0, len(r.rows))
	for _, row := range r.rows {
		if row != nil && row.UserID == userID {
			out = append(out, row)
		}
	}
	return out, nil
}

func (r *fakeFlowMCPServerRepo) FindByID(_ context.Context, userID uuid.UUID, id uuid.UUID) (*models.MCPServer, error) {
	for _, row := range r.rows {
		if row != nil && row.UserID == userID && row.ID == id {
			return row, nil
		}
	}
	return nil, xerr.NotFound("MCP服务不存在")
}

func (r *fakeFlowMCPServerRepo) Update(_ context.Context, _ uuid.UUID, row *models.MCPServer) (*models.MCPServer, error) {
	return row, nil
}

func (r *fakeFlowMCPServerRepo) UpdateFields(_ context.Context, _ uuid.UUID, _ uuid.UUID, row *models.MCPServer, _ *repository.MCPServerUpdateFields) (*models.MCPServer, error) {
	return row, nil
}

func (r *fakeFlowMCPServerRepo) Delete(_ context.Context, _ uuid.UUID, _ uuid.UUID) error {
	return nil
}

func (r *fakeFlowMCPServerRepo) FindByName(_ context.Context, userID uuid.UUID, name string) (*models.MCPServer, error) {
	for _, row := range r.rows {
		if row != nil && row.UserID == userID && row.Name == name {
			return row, nil
		}
	}
	return nil, xerr.NotFound("MCP服务不存在")
}

type fakeFlowToolConfigRepo struct {
	rows    []*models.ToolConfig
	listErr error
}

func (r *fakeFlowToolConfigRepo) Create(_ context.Context, userID uuid.UUID, row *models.ToolConfig) (*models.ToolConfig, error) {
	row.UserID = userID
	r.rows = append(r.rows, row)
	return row, nil
}

func (r *fakeFlowToolConfigRepo) List(_ context.Context, userID uuid.UUID) ([]*models.ToolConfig, error) {
	if r.listErr != nil {
		return nil, r.listErr
	}
	out := make([]*models.ToolConfig, 0, len(r.rows))
	for _, row := range r.rows {
		if row != nil && row.UserID == userID {
			out = append(out, row)
		}
	}
	return out, nil
}

func (r *fakeFlowToolConfigRepo) FindByID(_ context.Context, userID uuid.UUID, id uuid.UUID) (*models.ToolConfig, error) {
	for _, row := range r.rows {
		if row != nil && row.UserID == userID && row.ID == id {
			return row, nil
		}
	}
	return nil, xerr.NotFound("工具配置不存在")
}

func (r *fakeFlowToolConfigRepo) Update(_ context.Context, _ uuid.UUID, row *models.ToolConfig) (*models.ToolConfig, error) {
	return row, nil
}

func (r *fakeFlowToolConfigRepo) UpdateFields(_ context.Context, _ uuid.UUID, _ uuid.UUID, row *models.ToolConfig, _ *repository.ToolConfigUpdateFields) (*models.ToolConfig, error) {
	return row, nil
}

func (r *fakeFlowToolConfigRepo) Delete(_ context.Context, _ uuid.UUID, _ uuid.UUID) error {
	return nil
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

func (c *fakeFlowChatLLMClient) StreamEvents(ctx context.Context, messages []*corellm.Message, opts ...corellm.ModelCallOption) (<-chan corellm.StreamEvent, error) {
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

func (r *fakeFlowMessageRepo) ListPage(ctx context.Context, userID uuid.UUID, query repository.MessageListQuery) ([]*models.Message, bool, error) {
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
