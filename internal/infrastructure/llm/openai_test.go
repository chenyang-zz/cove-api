package llm_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	corellm "github.com/boxify/api-go/internal/core/llm"
	coretool "github.com/boxify/api-go/internal/core/tool"
	infra "github.com/boxify/api-go/internal/infrastructure/llm"
)

// 验证 OpenAI Invoke 会发送基础聊天请求，并使用 core 层默认温度。
func TestOpenAIClientInvokeSendsChatCompletionRequest(t *testing.T) {
	var authHeader string
	var path string
	var requestBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader = r.Header.Get("Authorization")
		path = r.URL.Path
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"pong"}}]}`))
	}))
	defer server.Close()

	client := infra.NewOpenaiLLMClient("sk-test", "chat-model", infra.WithBaseURL(server.URL+"/v1"))
	got, err := client.Invoke(context.Background(), []*corellm.Message{
		{Role: corellm.UserRole, Content: "ping"},
	})
	if err != nil {
		t.Fatalf("Invoke error = %v", err)
	}
	if got != "pong" {
		t.Fatalf("Invoke = %q, want pong", got)
	}
	if path != "/v1/chat/completions" {
		t.Fatalf("path = %q", path)
	}
	if authHeader != "Bearer sk-test" {
		t.Fatalf("auth = %q", authHeader)
	}
	if requestBody["model"] != "chat-model" {
		t.Fatalf("request body = %#v", requestBody)
	}
	if stream, ok := requestBody["stream"]; ok && stream != false {
		t.Fatalf("stream = %#v, want false or omitted", stream)
	}
	if requestBody["temperature"] != float64(corellm.DefaultTemperature) {
		t.Fatalf("temperature = %#v, want core default %v; body=%#v", requestBody["temperature"], corellm.DefaultTemperature, requestBody)
	}
	if _, ok := requestBody["max_tokens"]; ok {
		t.Fatalf("max_tokens should be omitted when no option is set: %#v", requestBody)
	}
}

// 验证 OpenAI Invoke 会发送可选聊天参数。
func TestOpenAIClientInvokeSendsOptionalChatParams(t *testing.T) {
	var requestBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"pong"}}]}`))
	}))
	defer server.Close()

	client := infra.NewOpenaiLLMClient("sk-test", "chat-model", infra.WithBaseURL(server.URL+"/v1"))
	if _, err := client.Invoke(context.Background(),
		[]*corellm.Message{{Role: corellm.UserRole, Content: "ping"}},
		corellm.WithTemperature(0.7),
		corellm.WithTopP(0.8),
		corellm.WithMaxTokens(128),
	); err != nil {
		t.Fatalf("Invoke error = %v", err)
	}
	if requestBody["temperature"] != float64(0.7) || requestBody["top_p"] != float64(0.8) || requestBody["max_tokens"] != float64(128) {
		t.Fatalf("request body = %#v", requestBody)
	}
}

// 验证 OpenAI client 默认温度会覆盖 core 默认温度。
func TestOpenAIClientInvokeUsesDefaultTemperature(t *testing.T) {
	var requestBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"pong"}}]}`))
	}))
	defer server.Close()

	client := infra.NewOpenaiLLMClient("sk-test", "chat-model",
		infra.WithBaseURL(server.URL+"/v1"),
		infra.WithTemperature(0.6),
	)
	if _, err := client.Invoke(context.Background(), []*corellm.Message{{Role: corellm.UserRole, Content: "ping"}}); err != nil {
		t.Fatalf("Invoke error = %v", err)
	}
	if requestBody["temperature"] != float64(0.6) {
		t.Fatalf("request body = %#v", requestBody)
	}
}

func TestOpenAIClientInvokeTemperatureOptionOverridesDefault(t *testing.T) {
	var requestBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"pong"}}]}`))
	}))
	defer server.Close()

	client := infra.NewOpenaiLLMClient("sk-test", "chat-model",
		infra.WithBaseURL(server.URL+"/v1"),
		infra.WithTemperature(0.6),
	)
	if _, err := client.Invoke(context.Background(),
		[]*corellm.Message{{Role: corellm.UserRole, Content: "ping"}},
		corellm.WithTemperature(0.2),
	); err != nil {
		t.Fatalf("Invoke error = %v", err)
	}
	if requestBody["temperature"] != float64(0.2) {
		t.Fatalf("request body = %#v", requestBody)
	}
}

// 验证 OpenAI InvokeResult 会发送工具参数，并映射文本、工具调用、停止原因和 token 用量。
func TestOpenAIClientInvokeResultMapsRichResponse(t *testing.T) {
	var requestBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"chatcmpl_123",
			"model":"chat-model",
			"choices":[{
				"finish_reason":"tool_calls",
				"message":{
					"content":"need tool",
					"tool_calls":[
						{"id":"call_1","type":"function","function":{"name":"search","arguments":"{\"query\":\"golang\"}"}},
						{"id":"call_bad","type":"function","function":{"name":"bad","arguments":"not-json"}}
					]
				}
			}],
			"usage":{"prompt_tokens":3,"completion_tokens":5,"total_tokens":8}
		}`))
	}))
	defer server.Close()

	client := infra.NewOpenaiLLMClient("sk-test", "chat-model", infra.WithBaseURL(server.URL+"/v1"))
	strict := true
	result, err := client.InvokeResult(context.Background(),
		[]*corellm.Message{{Role: corellm.UserRole, Content: "ping"}},
		corellm.WithTopP(0.8),
		corellm.WithTools(coretool.Descriptor{
			Name:        "search",
			Description: "search docs",
			Schema: coretool.Schema{Strict: &strict, Parameters: coretool.ParametersSchema{
				Type: "object",
				Properties: map[string]coretool.PropertySchema{
					"query": {"type": "string"},
				},
				Required:             []string{"query"},
				AdditionalProperties: false,
			}},
		}),
		corellm.WithRequiredTool("search"),
	)
	if err != nil {
		t.Fatalf("InvokeResult error = %v, want nil", err)
	}
	if requestBody["top_p"] != float64(0.8) {
		t.Fatalf("request top_p = %#v, want 0.8; body=%#v", requestBody["top_p"], requestBody)
	}
	tools, ok := requestBody["tools"].([]any)
	if !ok || len(tools) != 1 {
		t.Fatalf("request tools = %#v, want one tool", requestBody["tools"])
	}
	function := tools[0].(map[string]any)["function"].(map[string]any)
	if function["name"] != "search" || function["strict"] != true {
		t.Fatalf("request function = %#v, want search strict function", function)
	}
	toolChoice := requestBody["tool_choice"].(map[string]any)
	if toolChoice["type"] != "function" || toolChoice["function"].(map[string]any)["name"] != "search" {
		t.Fatalf("request tool_choice = %#v, want required search tool", toolChoice)
	}
	if result.Text != "need tool" || result.ID != "chatcmpl_123" || result.Model != "chat-model" || result.Provider != "openai" || result.StopReason != "tool_calls" {
		t.Fatalf("InvokeResult metadata = %#v, want mapped OpenAI fields", result)
	}
	if result.Usage.InputTokens != 3 || result.Usage.OutputTokens != 5 || result.Usage.TotalTokens != 8 {
		t.Fatalf("InvokeResult usage = %#v, want 3/5/8", result.Usage)
	}
	if len(result.ToolCalls) != 2 || result.ToolCalls[0].Name != "search" || result.ToolCalls[0].Input["query"] != "golang" {
		t.Fatalf("InvokeResult tool calls = %#v, want parsed search call", result.ToolCalls)
	}
	if result.ToolCalls[1].RawInput != "not-json" || result.ToolCalls[1].Input != nil {
		t.Fatalf("InvokeResult invalid tool input = %#v, want raw preserved and nil parsed input", result.ToolCalls[1])
	}
	if !strings.Contains(result.RawJSON, "chatcmpl_123") {
		t.Fatalf("InvokeResult RawJSON = %q, want original response", result.RawJSON)
	}
}

// 验证 OpenAI InvokeWithTools 会把工具定义、工具选择和历史工具结果映射到请求体。
func TestOpenAIClientInvokeWithToolsSendsToolHistory(t *testing.T) {
	var requestBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"chatcmpl_456",
			"model":"chat-model",
			"choices":[{"finish_reason":"stop","message":{"content":"done"}}],
			"usage":{"prompt_tokens":4,"completion_tokens":2,"total_tokens":6}
		}`))
	}))
	defer server.Close()

	client := infra.NewOpenaiLLMClient("sk-test", "chat-model", infra.WithBaseURL(server.URL+"/v1"))
	result, err := client.(corellm.ToolCallingClient).InvokeWithTools(context.Background(),
		[]*corellm.Message{
			corellm.UserMessage("time?"),
			{
				Role: corellm.AssistantRole,
				ToolCalls: []corellm.LLMToolCall{
					{ID: "call_1", Name: "current_time", RawInput: `{"zone":"UTC"}`, Input: coretool.Input{"zone": "UTC"}},
				},
			},
			{Role: corellm.ToolRole, ToolCallID: "call_1", ToolName: "current_time", Content: "12:00"},
		},
		corellm.WithTools(coretool.Descriptor{
			Name:        "current_time",
			Description: "get current time",
			Schema: coretool.Schema{Parameters: coretool.ParametersSchema{
				Type:       "object",
				Properties: map[string]coretool.PropertySchema{"zone": {"type": "string"}},
				Required:   []string{"zone"},
			}},
		}),
		corellm.WithToolChoiceAuto(),
	)
	if err != nil {
		t.Fatalf("InvokeWithTools error = %v, want nil", err)
	}
	if result.Text != "done" || result.ID != "chatcmpl_456" {
		t.Fatalf("InvokeWithTools result = %#v, want done chatcmpl_456", result)
	}
	messages, ok := requestBody["messages"].([]any)
	if !ok || len(messages) != 3 {
		t.Fatalf("request messages = %#v, want user assistant tool", requestBody["messages"])
	}
	assistant := messages[1].(map[string]any)
	calls := assistant["tool_calls"].([]any)
	call := calls[0].(map[string]any)
	if assistant["role"] != "assistant" || call["id"] != "call_1" || call["function"].(map[string]any)["name"] != "current_time" {
		t.Fatalf("assistant message = %#v, want current_time tool call", assistant)
	}
	toolMessage := messages[2].(map[string]any)
	if toolMessage["role"] != "tool" || toolMessage["tool_call_id"] != "call_1" || toolMessage["content"] != "12:00" {
		t.Fatalf("tool message = %#v, want call_1 result 12:00", toolMessage)
	}
	if _, ok := requestBody["tools"].([]any); !ok {
		t.Fatalf("request tools = %#v, want tools from WithTools", requestBody["tools"])
	}
	if requestBody["tool_choice"] != "auto" {
		t.Fatalf("request tool_choice = %#v, want auto", requestBody["tool_choice"])
	}
}

// TestOpenAIClientPreservesCompleteToolJSONSchema 验证 OpenAI 工具参数会保留 MCP schema 的顶层扩展字段。
func TestOpenAIClientPreservesCompleteToolJSONSchema(t *testing.T) {
	var requestBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"chatcmpl_schema","model":"chat-model","choices":[{"finish_reason":"stop","message":{"content":"done"}}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))
	}))
	defer server.Close()

	client := infra.NewOpenaiLLMClient("sk-test", "chat-model", infra.WithBaseURL(server.URL+"/v1"))
	_, err := client.InvokeResult(context.Background(), []*corellm.Message{corellm.UserMessage("ping")}, corellm.WithTools(coretool.Descriptor{
		Name: "mcp_schema",
		Schema: coretool.Schema{Parameters: coretool.NewParametersSchema(map[string]any{
			"type":       "object",
			"properties": map[string]any{"query": map[string]any{"type": "string"}},
			"oneOf":      []any{map[string]any{"required": []any{"query"}}},
			"$defs":      map[string]any{"filter": map[string]any{"type": "object"}},
		})},
	}))
	if err != nil {
		t.Fatalf("InvokeResult error = %v, want nil", err)
	}
	tools := requestBody["tools"].([]any)
	parameters := tools[0].(map[string]any)["function"].(map[string]any)["parameters"].(map[string]any)
	if parameters["oneOf"] == nil || parameters["$defs"] == nil {
		t.Fatalf("OpenAI parameters = %#v, want oneOf and $defs", parameters)
	}
}

func TestOpenAIClientStreamReadsSSEDeltaContent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"he\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"llo\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	client := infra.NewOpenaiLLMClient("sk-test", "chat-model", infra.WithBaseURL(server.URL+"/v1"))
	stream, err := client.Stream(context.Background(), []*corellm.Message{{Role: corellm.UserRole, Content: "say"}})
	if err != nil {
		t.Fatalf("Stream error = %v", err)
	}
	var parts []string
	for token := range stream {
		parts = append(parts, token)
	}
	if got := strings.Join(parts, ""); got != "hello" {
		t.Fatalf("stream = %q, want hello", got)
	}
}

// 验证 OpenAI 原生工具流会保留文本增量并聚合完整的工具参数。
func TestOpenAIClientStreamWithToolsAggregatesToolCall(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"查询中\",\"tool_calls\":[{\"index\":0,\"id\":\"call_1\",\"type\":\"function\",\"function\":{\"name\":\"search\",\"arguments\":\"{\\\"query\\\"\"}}]}}]}\n\n"))
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"function\":{\"arguments\":\":\\\"golang\\\"}\"}}]}}]}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	client := infra.NewOpenaiLLMClient("sk-test", "chat-model", infra.WithBaseURL(server.URL+"/v1"))
	stream, err := client.(corellm.ToolStreamEventClient).StreamWithTools(context.Background(), []*corellm.Message{corellm.UserMessage("search")})
	if err != nil {
		t.Fatalf("StreamWithTools error = %v, want nil", err)
	}
	events := collectStreamEvents(stream)
	if len(events) != 3 || events[0].Kind != corellm.StreamEventTextDelta || events[0].Text != "查询中" {
		t.Fatalf("StreamWithTools events = %#v, want text/tool/done", events)
	}
	call := events[1].ToolCall
	if events[1].Kind != corellm.StreamEventToolCall || call == nil || call.ID != "call_1" || call.Name != "search" || call.Input["query"] != "golang" {
		t.Fatalf("StreamWithTools tool call = %#v, want aggregated search call", events[1])
	}
	if events[2].Kind != corellm.StreamEventDone {
		t.Fatalf("StreamWithTools final event = %#v, want done", events[2])
	}
}

func collectStreamEvents(stream <-chan corellm.StreamEvent) []corellm.StreamEvent {
	var events []corellm.StreamEvent
	for event := range stream {
		events = append(events, event)
	}
	return events
}

// 验证 OpenAI Stream 会发送可选聊天参数。
func TestOpenAIClientStreamSendsOptionalChatParams(t *testing.T) {
	var requestBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	client := infra.NewOpenaiLLMClient("sk-test", "chat-model",
		infra.WithBaseURL(server.URL+"/v1"),
		infra.WithTemperature(0.6),
	)
	stream, err := client.Stream(context.Background(),
		[]*corellm.Message{{Role: corellm.UserRole, Content: "say"}},
		corellm.WithTemperature(0.2),
		corellm.WithTopP(0.9),
		corellm.WithMaxTokens(64),
	)
	if err != nil {
		t.Fatalf("Stream error = %v", err)
	}
	for range stream {
	}
	if requestBody["temperature"] != float64(0.2) || requestBody["top_p"] != float64(0.9) || requestBody["max_tokens"] != float64(64) {
		t.Fatalf("request body = %#v", requestBody)
	}
	if requestBody["stream"] != true {
		t.Fatalf("stream = %#v, want true", requestBody["stream"])
	}
}

func TestOpenAIClientEmbedUsesEmbeddingEndpointAndDimensions(t *testing.T) {
	var requestBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/embeddings" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"embedding":[0.1,0.2]},{"embedding":[0.3,0.4]}]}`))
	}))
	defer server.Close()

	client := infra.NewOpenaiLLMClient("sk-test", "chat-model",
		infra.WithBaseURL(server.URL+"/v1"),
		infra.WithEmbeddingModel("embed-model"),
	)
	vecs, err := client.Embed(context.Background(), []string{"a", "b"}, 1024)
	if err != nil {
		t.Fatalf("Embed error = %v", err)
	}
	if len(vecs) != 2 || len(vecs[0]) != 2 || vecs[0][0] != float64(0.1) {
		t.Fatalf("vectors = %#v", vecs)
	}
	if requestBody["model"] != "embed-model" || requestBody["dimensions"] != float64(1024) {
		t.Fatalf("request body = %#v", requestBody)
	}

	one, err := client.EmbedOne(context.Background(), "a", 1024)
	if err != nil {
		t.Fatalf("EmbedOne error = %v", err)
	}
	if len(one) != 2 {
		t.Fatalf("one = %#v", one)
	}
}

func TestOpenAIClientEmbedSplitsRequestsByBatchSize(t *testing.T) {
	// 验证 OpenAI-compatible embedding 会按传入批次大小拆分请求，并保持向量顺序与输入文本一致。
	var batchSizes []int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/embeddings" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		var requestBody map[string]any
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		inputs, ok := requestBody["input"].([]any)
		if !ok {
			t.Fatalf("request input = %#v, want array", requestBody["input"])
		}
		batchSizes = append(batchSizes, len(inputs))
		resp := struct {
			Data []struct {
				Embedding []float64 `json:"embedding"`
			} `json:"data"`
		}{Data: make([]struct {
			Embedding []float64 `json:"embedding"`
		}, 0, len(inputs))}
		for _, input := range inputs {
			text, ok := input.(string)
			if !ok {
				t.Fatalf("input item = %#v, want string", input)
			}
			resp.Data = append(resp.Data, struct {
				Embedding []float64 `json:"embedding"`
			}{Embedding: []float64{float64(len(text))}})
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer server.Close()

	texts := make([]string, 0, 23)
	for i := 0; i < 23; i++ {
		texts = append(texts, strings.Repeat("x", i+1))
	}
	client := infra.NewOpenaiLLMClient("sk-test", "chat-model",
		infra.WithBaseURL(server.URL+"/v1"),
		infra.WithEmbeddingModel("embed-model"),
	)
	vecs, err := client.Embed(context.Background(), texts, 1024, corellm.WithEmbeddingBatchSize(10))
	if err != nil {
		t.Fatalf("Embed error = %v", err)
	}
	if got := strings.Trim(strings.Join(strings.Fields(fmt.Sprint(batchSizes)), ","), "[]"); got != "10,10,3" {
		t.Fatalf("batch sizes = %v, want [10 10 3]", batchSizes)
	}
	if len(vecs) != len(texts) {
		t.Fatalf("vectors len = %d, want %d", len(vecs), len(texts))
	}
	for i, vec := range vecs {
		if len(vec) != 1 || vec[0] != float64(i+1) {
			t.Fatalf("vector[%d] = %#v, want [%d]", i, vec, i+1)
		}
	}
}

// 验证 OpenAI Vision 会发送多模态 chat/completions 请求（text + image_url data URL）。
func TestOpenAIClientVisionSendsImageURL(t *testing.T) {
	var requestBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"a cat"}}]}`))
	}))
	defer server.Close()

	client := infra.NewOpenaiLLMClient("sk-test", "vision-model", infra.WithBaseURL(server.URL+"/v1"))
	vision, ok := client.(corellm.VisionClient)
	if !ok {
		t.Fatal("openai client does not implement VisionClient")
	}
	got, err := vision.Vision(
		context.Background(),
		"describe",
		"YWJj",
		"image/png",
		corellm.WithTemperature(0.2),
		corellm.WithTopP(0.9),
		corellm.WithMaxTokens(256),
	)
	if err != nil {
		t.Fatalf("Vision error = %v", err)
	}
	if got == nil || got.Text != "a cat" || got.Description.Description != "a cat" {
		t.Fatalf("Vision = %#v, want structured fallback description a cat", got)
	}
	if requestBody["model"] != "vision-model" ||
		requestBody["max_tokens"] != float64(256) ||
		requestBody["temperature"] != float64(0.2) ||
		requestBody["top_p"] != float64(0.9) {
		t.Fatalf("request body = %#v", requestBody)
	}
	messages, ok := requestBody["messages"].([]any)
	if !ok || len(messages) != 1 {
		t.Fatalf("messages = %#v", requestBody["messages"])
	}
	msg, ok := messages[0].(map[string]any)
	if !ok {
		t.Fatalf("message = %#v", messages[0])
	}
	content, ok := msg["content"].([]any)
	if !ok || len(content) != 2 {
		t.Fatalf("content = %#v, want text and image parts", msg["content"])
	}
	encoded, _ := json.Marshal(content)
	body := string(encoded)
	if !strings.Contains(body, `"type":"text"`) || !strings.Contains(body, "describe") {
		t.Fatalf("content = %s, want text prompt", body)
	}
	if !strings.Contains(body, `"type":"image_url"`) || !strings.Contains(body, "data:image/png;base64,YWJj") {
		t.Fatalf("content = %s, want image_url data URL", body)
	}
}

// 验证 OpenAI Vision 未传 option 时使用聊天默认温度与看图默认 max_tokens。
func TestOpenAIClientVisionUsesDefaultChatAndVisionOptions(t *testing.T) {
	var requestBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
	}))
	defer server.Close()

	client := infra.NewOpenaiLLMClient("sk-test", "vision-model", infra.WithBaseURL(server.URL+"/v1"))
	vision := client.(corellm.VisionClient)
	if _, err := vision.Vision(context.Background(), "describe", "YWJj", "image/jpeg"); err != nil {
		t.Fatalf("Vision error = %v", err)
	}
	if requestBody["temperature"] != float64(corellm.DefaultTemperature) {
		t.Fatalf("temperature = %#v, want default %v", requestBody["temperature"], corellm.DefaultTemperature)
	}
	if requestBody["max_tokens"] != float64(corellm.DefaultVisionMaxTokens) {
		t.Fatalf("max_tokens = %#v, want default %d", requestBody["max_tokens"], corellm.DefaultVisionMaxTokens)
	}
}

