package llm

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	corellm "github.com/boxify/api-go/internal/core/llm"
	"github.com/boxify/api-go/internal/xerr"
)

const (
	defaultAnthropicBaseURL   = "https://api.anthropic.com"
	defaultAnthropicMaxTokens = int64(1024)
)

type anthropicLLMClient struct {
	client             anthropic.Client
	apiKey             string
	model              string
	defaultMaxTokens   int64
	defaultTemperature *float64
}

type anthropicConfig struct {
	httpClient         *http.Client
	apiKey             string
	baseURL            string
	defaultMaxTokens   int64
	defaultTemperature *float64
}

type AnthropicOption func(*anthropicConfig)

func WithAnthropicBaseURL(baseURL string) AnthropicOption {
	return func(c *anthropicConfig) {
		if strings.TrimSpace(baseURL) != "" {
			c.baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
		}
	}
}

func WithAnthropicMaxTokens(maxTokens int64) AnthropicOption {
	return func(c *anthropicConfig) {
		if maxTokens > 0 {
			c.defaultMaxTokens = maxTokens
		}
	}
}

func WithAnthropicTemperature(value float64) AnthropicOption {
	return func(c *anthropicConfig) {
		c.defaultTemperature = &value
	}
}

func NewAnthropicLLMClient(apiKey string, model string, opts ...AnthropicOption) corellm.Client {
	cfg := &anthropicConfig{
		httpClient:       &http.Client{Timeout: 60 * time.Second},
		apiKey:           strings.TrimSpace(apiKey),
		baseURL:          defaultAnthropicBaseURL,
		defaultMaxTokens: defaultAnthropicMaxTokens,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(cfg)
		}
	}

	clientOptions := []option.RequestOption{
		option.WithAPIKey(cfg.apiKey),
		option.WithBaseURL(cfg.baseURL),
		option.WithHTTPClient(cfg.httpClient),
	}
	return &anthropicLLMClient{
		client:             anthropic.NewClient(clientOptions...),
		apiKey:             cfg.apiKey,
		model:              strings.TrimSpace(model),
		defaultMaxTokens:   cfg.defaultMaxTokens,
		defaultTemperature: cfg.defaultTemperature,
	}
}

func (c *anthropicLLMClient) Invoke(ctx context.Context, messages []*corellm.Message, opts ...corellm.ModelCallOption) (string, error) {
	if err := c.validateChatConfig(); err != nil {
		return "", err
	}
	resp, err := c.client.Messages.New(ctx, c.messageParams(messages, opts...))
	if err != nil {
		return "", xerr.Wrapf(err, "请求模型接口失败")
	}
	if resp == nil || len(resp.Content) == 0 {
		return "", xerr.Internal("模型返回为空", nil)
	}
	var out strings.Builder
	for _, block := range resp.Content {
		if block.Text != "" {
			out.WriteString(block.Text)
		}
	}
	if out.Len() == 0 {
		return "", xerr.Internal("模型返回为空", nil)
	}
	return out.String(), nil
}

func (c *anthropicLLMClient) Stream(ctx context.Context, messages []*corellm.Message, opts ...corellm.ModelCallOption) (<-chan string, error) {
	if err := c.validateChatConfig(); err != nil {
		return nil, err
	}
	stream := c.client.Messages.NewStreaming(ctx, c.messageParams(messages, opts...))
	if err := stream.Err(); err != nil {
		return nil, xerr.Wrapf(err, "请求模型流式接口失败")
	}

	ch := make(chan string)
	go func() {
		defer close(ch)
		for stream.Next() {
			chunk := stream.Current()
			if chunk.Delta.Text == "" {
				continue
			}
			select {
			case <-ctx.Done():
				return
			case ch <- chunk.Delta.Text:
			}
		}
	}()
	return ch, nil
}

func (c *anthropicLLMClient) Embed(context.Context, []string, int, ...corellm.EmbeddingOption) ([][]float64, error) {
	return nil, xerr.BadRequest("Anthropic 当前不支持向量模型调用")
}

func (c *anthropicLLMClient) EmbedOne(context.Context, string, int) ([]float64, error) {
	return nil, xerr.BadRequest("Anthropic 当前不支持向量模型调用")
}

func (c *anthropicLLMClient) messageParams(messages []*corellm.Message, opts ...corellm.ModelCallOption) anthropic.MessageNewParams {
	params := anthropic.MessageNewParams{
		Model:     anthropic.Model(c.model),
		MaxTokens: c.defaultMaxTokens,
	}
	if c.defaultTemperature != nil {
		params.Temperature = anthropic.Float(*c.defaultTemperature)
	}
	chatOpts := corellm.NewChatOptions(opts...)
	if chatOpts.Temperature != nil {
		params.Temperature = anthropic.Float(*chatOpts.Temperature)
	}
	if chatOpts.MaxTokens != nil && *chatOpts.MaxTokens > 0 {
		params.MaxTokens = *chatOpts.MaxTokens
	}
	params.Messages, params.System = toAnthropicMessages(messages)
	return params
}

func (c *anthropicLLMClient) validateChatConfig() error {
	if c.apiKey == "" {
		return xerr.Internal("模型 API Key 未配置", nil)
	}
	if c.model == "" {
		return xerr.Internal("模型名称未配置", nil)
	}
	return nil
}

func toAnthropicMessages(messages []*corellm.Message) ([]anthropic.MessageParam, []anthropic.TextBlockParam) {
	out := make([]anthropic.MessageParam, 0, len(messages))
	system := make([]anthropic.TextBlockParam, 0)
	for _, m := range messages {
		if m == nil || strings.TrimSpace(m.Content) == "" {
			continue
		}
		switch m.Role {
		case corellm.SystemRole:
			system = append(system, anthropic.TextBlockParam{Text: m.Content})
		case corellm.AssistantRole:
			out = append(out, anthropic.NewAssistantMessage(anthropic.NewTextBlock(m.Content)))
		default:
			out = append(out, anthropic.NewUserMessage(anthropic.NewTextBlock(m.Content)))
		}
	}
	return out, system
}
