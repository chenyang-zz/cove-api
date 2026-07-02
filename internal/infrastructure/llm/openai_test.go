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
	infra "github.com/boxify/api-go/internal/infrastructure/llm"
)

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
	if _, ok := requestBody["temperature"]; ok {
		t.Fatalf("temperature should be omitted when no option is set: %#v", requestBody)
	}
	if _, ok := requestBody["max_tokens"]; ok {
		t.Fatalf("max_tokens should be omitted when no option is set: %#v", requestBody)
	}
}

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
		corellm.WithMaxTokens(128),
	); err != nil {
		t.Fatalf("Invoke error = %v", err)
	}
	if requestBody["temperature"] != float64(0.7) || requestBody["max_tokens"] != float64(128) {
		t.Fatalf("request body = %#v", requestBody)
	}
}

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
		corellm.WithMaxTokens(64),
	)
	if err != nil {
		t.Fatalf("Stream error = %v", err)
	}
	for range stream {
	}
	if requestBody["temperature"] != float64(0.2) || requestBody["max_tokens"] != float64(64) {
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
