package llm

import (
	"context"
	"strings"
	"testing"
)

type recordingFactory struct {
	name string
	got  ModelConfig
}

func (f *recordingFactory) NewClient(cfg ModelConfig) (Client, error) {
	f.got = cfg
	return stubClient{}, nil
}

type stubClient struct{}

func (stubClient) Invoke(context.Context, []*Message, ...ModelCallOption) (string, error) {
	return "", nil
}

func (stubClient) Stream(context.Context, []*Message, ...ModelCallOption) (<-chan string, error) {
	ch := make(chan string)
	close(ch)
	return ch, nil
}

func (stubClient) Embed(context.Context, []string, int, ...EmbeddingOption) ([][]float64, error) {
	return nil, nil
}

func (stubClient) EmbedOne(context.Context, string, int) ([]float64, error) {
	return nil, nil
}

func TestManagerRoutesRegisteredProviders(t *testing.T) {
	openAI := &recordingFactory{name: "openai"}
	anthropic := &recordingFactory{name: "anthropic"}
	manager := NewManager()
	manager.Register("openai", openAI)
	manager.Register("deepseek", openAI)
	manager.Register("anthropic", anthropic)

	if _, err := manager.NewClient(ModelConfig{
		Provider: " deepseek ",
		Model:    "deepseek-chat",
		APIKey:   "sk-deepseek",
		BaseURL:  "https://api.deepseek.com",
	}); err != nil {
		t.Fatalf("NewClient deepseek error = %v", err)
	}
	if openAI.got.Provider != "deepseek" || openAI.got.Model != "deepseek-chat" || openAI.got.APIKey != "sk-deepseek" {
		t.Fatalf("openai-compatible factory config = %#v", openAI.got)
	}

	if _, err := manager.NewClient(ModelConfig{
		Provider: "anthropic",
		Model:    "claude-sonnet-4-5",
		APIKey:   "sk-ant",
	}); err != nil {
		t.Fatalf("NewClient anthropic error = %v", err)
	}
	if anthropic.got.Provider != "anthropic" || anthropic.got.Model != "claude-sonnet-4-5" || anthropic.got.APIKey != "sk-ant" {
		t.Fatalf("anthropic factory config = %#v", anthropic.got)
	}
}

func TestManagerRejectsInvalidConfig(t *testing.T) {
	manager := NewManager()
	manager.Register("openai", &recordingFactory{})

	tests := []struct {
		name string
		cfg  ModelConfig
		want string
	}{
		{
			name: "missing provider",
			cfg:  ModelConfig{Model: "gpt-4o-mini", APIKey: "sk-test"},
			want: "模型 provider 未配置",
		},
		{
			name: "missing model",
			cfg:  ModelConfig{Provider: "openai", APIKey: "sk-test"},
			want: "模型名称未配置",
		},
		{
			name: "missing api key",
			cfg:  ModelConfig{Provider: "openai", Model: "gpt-4o-mini"},
			want: "模型 API Key 未配置",
		},
		{
			name: "unsupported provider",
			cfg:  ModelConfig{Provider: "unknown", Model: "model", APIKey: "sk-test"},
			want: "不支持的模型 provider",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := manager.NewClient(tc.cfg)
			if err == nil {
				t.Fatal("NewClient error = nil")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("NewClient error = %v, want contains %q", err, tc.want)
			}
		})
	}
}
