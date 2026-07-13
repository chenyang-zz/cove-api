package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"time"

	corellm "github.com/boxify/api-go/internal/core/llm"
	coretool "github.com/boxify/api-go/internal/core/tool"
	"github.com/boxify/api-go/internal/xerr"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/shared"
)

const defaultOpenAIBaseURL = "https://api.openai.com/v1"

type openaiLLMClient struct {
	client             openai.Client
	apiKey             string
	model              string
	embeddingModel     string
	defaultTemperature *float64
}

type openAIConfig struct {
	httpClient         *http.Client
	apiKey             string
	baseURL            string
	embeddingModel     string
	defaultTemperature *float64
}

type OpenAIOption func(*openAIConfig)

func WithBaseURL(baseURL string) OpenAIOption {
	return func(c *openAIConfig) {
		if strings.TrimSpace(baseURL) != "" {
			c.baseURL = normalizeOpenAIBaseURL(baseURL)
		}
	}
}

func normalizeOpenAIBaseURL(baseURL string) string {
	normalized := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	parsed, err := url.Parse(normalized)
	if err != nil || parsed == nil {
		return normalized
	}
	if strings.EqualFold(parsed.Hostname(), "api.openai.com") && parsed.Path == "" {
		parsed.Path = "/v1"
		return parsed.String()
	}
	return normalized
}

func WithEmbeddingModel(model string) OpenAIOption {
	return func(c *openAIConfig) {
		if strings.TrimSpace(model) != "" {
			c.embeddingModel = strings.TrimSpace(model)
		}
	}
}

func WithTemperature(value float64) OpenAIOption {
	return func(c *openAIConfig) {
		c.defaultTemperature = &value
	}
}

func NewOpenaiLLMClient(apiKey string, model string, opts ...OpenAIOption) corellm.Client {
	cfg := &openAIConfig{
		httpClient: &http.Client{Timeout: 60 * time.Second},
		apiKey:     strings.TrimSpace(apiKey),
		baseURL:    defaultOpenAIBaseURL,
	}
	for _, opt := range opts {
		opt(cfg)
	}

	model = strings.TrimSpace(model)
	if cfg.embeddingModel == "" {
		cfg.embeddingModel = model
	}

	clientOptions := []option.RequestOption{
		option.WithAPIKey(cfg.apiKey),
		option.WithBaseURL(cfg.baseURL),
		option.WithHTTPClient(cfg.httpClient),
	}
	return &openaiLLMClient{
		client:             openai.NewClient(clientOptions...),
		apiKey:             cfg.apiKey,
		model:              model,
		embeddingModel:     cfg.embeddingModel,
		defaultTemperature: cfg.defaultTemperature,
	}
}

func (c *openaiLLMClient) Invoke(ctx context.Context, messages []*corellm.Message, opts ...corellm.ModelCallOption) (string, error) {
	result, err := c.InvokeResult(ctx, messages, opts...)
	if err != nil {
		return "", err
	}
	return result.Text, nil
}

func (c *openaiLLMClient) InvokeResult(ctx context.Context, messages []*corellm.Message, opts ...corellm.ModelCallOption) (*corellm.LLMResult, error) {
	if err := c.validateChatConfig(); err != nil {
		return nil, err
	}
	resp, err := c.client.Chat.Completions.New(ctx, c.chatParams(messages, opts...))
	if err != nil {
		return nil, xerr.Wrapf(err, "请求模型接口失败")
	}
	if resp == nil || len(resp.Choices) == 0 {
		return nil, xerr.Internal("模型返回为空", nil)
	}
	choice := resp.Choices[0]
	return &corellm.LLMResult{
		Text:       choice.Message.Content,
		ToolCalls:  openAIToolCalls(choice.Message.ToolCalls),
		Model:      resp.Model,
		Provider:   "openai",
		ID:         resp.ID,
		StopReason: choice.FinishReason,
		Usage: corellm.TokenUsage{
			InputTokens:  resp.Usage.PromptTokens,
			OutputTokens: resp.Usage.CompletionTokens,
			TotalTokens:  resp.Usage.TotalTokens,
		},
		RawJSON: resp.RawJSON(),
	}, nil
}

func (c *openaiLLMClient) InvokeWithTools(ctx context.Context, messages []*corellm.Message, opts ...corellm.ModelCallOption) (*corellm.LLMResult, error) {
	return c.InvokeResult(ctx, messages, opts...)
}

func (c *openaiLLMClient) Stream(ctx context.Context, messages []*corellm.Message, opts ...corellm.ModelCallOption) (<-chan string, error) {
	events, err := c.StreamEvents(ctx, messages, opts...)
	if err != nil {
		return nil, err
	}
	ch := make(chan string)
	go func() {
		defer close(ch)
		for event := range events {
			if event.Kind != corellm.StreamEventTextDelta || event.Text == "" {
				continue
			}
			select {
			case <-ctx.Done():
				return
			case ch <- event.Text:
			}
		}
	}()
	return ch, nil
}

// StreamEvents 执行结构化流式文本生成。
func (c *openaiLLMClient) StreamEvents(ctx context.Context, messages []*corellm.Message, opts ...corellm.ModelCallOption) (<-chan corellm.StreamEvent, error) {
	return c.streamEvents(ctx, messages, opts...)
}

// StreamWithTools 执行支持原生工具调用的结构化流式生成。
func (c *openaiLLMClient) StreamWithTools(ctx context.Context, messages []*corellm.Message, opts ...corellm.ModelCallOption) (<-chan corellm.StreamEvent, error) {
	return c.streamEvents(ctx, messages, opts...)
}

func (c *openaiLLMClient) streamEvents(ctx context.Context, messages []*corellm.Message, opts ...corellm.ModelCallOption) (<-chan corellm.StreamEvent, error) {
	if err := c.validateChatConfig(); err != nil {
		return nil, err
	}
	stream := c.client.Chat.Completions.NewStreaming(ctx, c.chatParams(messages, opts...))
	if err := stream.Err(); err != nil {
		return nil, xerr.Wrapf(err, "请求模型流式接口失败")
	}

	ch := make(chan corellm.StreamEvent)
	go func() {
		defer close(ch)
		toolCalls := make(map[int64]*openAIStreamToolCall)
		for stream.Next() {
			chunk := stream.Current()
			for _, choice := range chunk.Choices {
				if choice.Delta.Content != "" {
					if !sendStreamEvent(ctx, ch, corellm.StreamEvent{Kind: corellm.StreamEventTextDelta, Text: choice.Delta.Content}) {
						return
					}
				}
				for _, delta := range choice.Delta.ToolCalls {
					call := toolCalls[delta.Index]
					if call == nil {
						call = &openAIStreamToolCall{}
						toolCalls[delta.Index] = call
					}
					if delta.ID != "" {
						call.ID = delta.ID
					}
					if delta.Function.Name != "" {
						call.Name = delta.Function.Name
					}
					call.Arguments += delta.Function.Arguments
				}
			}
		}
		if err := stream.Err(); err != nil {
			sendStreamEvent(ctx, ch, corellm.StreamEvent{Kind: corellm.StreamEventError, Err: xerr.Wrapf(err, "请求模型流式接口失败")})
			return
		}
		for index := int64(0); ; index++ {
			call, ok := toolCalls[index]
			if !ok {
				break
			}
			if !sendStreamEvent(ctx, ch, corellm.StreamEvent{Kind: corellm.StreamEventToolCall, ToolCall: &corellm.LLMToolCall{
				ID:       call.ID,
				Name:     call.Name,
				Input:    parseToolInput(call.Arguments),
				RawInput: call.Arguments,
			}}) {
				return
			}
		}
		sendStreamEvent(ctx, ch, corellm.StreamEvent{Kind: corellm.StreamEventDone})
	}()
	return ch, nil
}

type openAIStreamToolCall struct {
	ID        string
	Name      string
	Arguments string
}

func sendStreamEvent(ctx context.Context, ch chan<- corellm.StreamEvent, event corellm.StreamEvent) bool {
	select {
	case <-ctx.Done():
		return false
	case ch <- event:
		return true
	}
}

func (c *openaiLLMClient) chatParams(messages []*corellm.Message, opts ...corellm.ModelCallOption) openai.ChatCompletionNewParams {
	params := openai.ChatCompletionNewParams{
		Model:    openai.ChatModel(c.model),
		Messages: toOpenAIMessages(messages),
	}
	chatCallOpts := make([]corellm.ModelCallOption, 0, len(opts)+1)
	if c.defaultTemperature != nil {
		chatCallOpts = append(chatCallOpts, corellm.WithTemperature(*c.defaultTemperature))
	}
	chatCallOpts = append(chatCallOpts, opts...)
	chatOpts := corellm.NewChatOptions(chatCallOpts...)
	if chatOpts.Temperature != nil {
		params.Temperature = openai.Float(*chatOpts.Temperature)
	}
	if chatOpts.TopP != nil {
		params.TopP = openai.Float(*chatOpts.TopP)
	}
	if chatOpts.MaxTokens != nil {
		params.MaxTokens = openai.Int(*chatOpts.MaxTokens)
	}
	params.Tools = toOpenAITools(chatOpts.Tools)
	params.ToolChoice = toOpenAIToolChoice(chatOpts.ToolChoice)
	return params
}

func toOpenAITools(tools []coretool.Descriptor) []openai.ChatCompletionToolUnionParam {
	if len(tools) == 0 {
		return nil
	}
	out := make([]openai.ChatCompletionToolUnionParam, 0, len(tools))
	for _, item := range tools {
		if strings.TrimSpace(item.Name) == "" {
			continue
		}
		function := shared.FunctionDefinitionParam{
			Name:        strings.TrimSpace(item.Name),
			Description: openai.String(item.Description),
			Parameters:  shared.FunctionParameters(toolParameters(item.Schema.Parameters)),
		}
		if item.Schema.Strict != nil {
			function.Strict = openai.Bool(*item.Schema.Strict)
		}
		out = append(out, openai.ChatCompletionToolUnionParam{
			OfFunction: &openai.ChatCompletionFunctionToolParam{Function: function},
		})
	}
	return out
}

func toOpenAIToolChoice(choice *corellm.ToolChoice) openai.ChatCompletionToolChoiceOptionUnionParam {
	if choice == nil {
		return openai.ChatCompletionToolChoiceOptionUnionParam{}
	}
	switch choice.Mode {
	case corellm.ToolChoiceAuto:
		return openai.ChatCompletionToolChoiceOptionUnionParam{OfAuto: openai.String(string(openai.ChatCompletionToolChoiceOptionAutoAuto))}
	case corellm.ToolChoiceNone:
		return openai.ChatCompletionToolChoiceOptionUnionParam{OfAuto: openai.String(string(openai.ChatCompletionToolChoiceOptionAutoNone))}
	case corellm.ToolChoiceRequired:
		return openai.ChatCompletionToolChoiceOptionUnionParam{OfAuto: openai.String(string(openai.ChatCompletionToolChoiceOptionAutoRequired))}
	case corellm.ToolChoiceTool:
		if strings.TrimSpace(choice.Name) == "" {
			return openai.ChatCompletionToolChoiceOptionUnionParam{}
		}
		return openai.ToolChoiceOptionFunctionToolChoice(openai.ChatCompletionNamedToolChoiceFunctionParam{Name: strings.TrimSpace(choice.Name)})
	default:
		return openai.ChatCompletionToolChoiceOptionUnionParam{}
	}
}

func openAIToolCalls(calls []openai.ChatCompletionMessageToolCallUnion) []corellm.LLMToolCall {
	if len(calls) == 0 {
		return nil
	}
	out := make([]corellm.LLMToolCall, 0, len(calls))
	for _, call := range calls {
		if call.Type != "function" {
			continue
		}
		function := call.AsFunction()
		rawInput := function.Function.Arguments
		out = append(out, corellm.LLMToolCall{
			ID:       function.ID,
			Name:     function.Function.Name,
			Input:    parseToolInput(rawInput),
			RawInput: rawInput,
		})
	}
	return out
}

func toolParameters(schema coretool.ParametersSchema) map[string]any {
	return schema.Map()
}

func parseToolInput(raw string) coretool.Input {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var input map[string]any
	if err := json.Unmarshal([]byte(raw), &input); err != nil {
		return nil
	}
	return coretool.Input(input)
}

func (c *openaiLLMClient) Embed(ctx context.Context, texts []string, dimensions int, opts ...corellm.EmbeddingOption) ([][]float64, error) {
	if err := c.validateEmbeddingConfig(); err != nil {
		return nil, err
	}
	if len(texts) == 0 {
		return nil, nil
	}
	embeddingOpts := corellm.NewEmbeddingOptions(opts...)
	batchSize := len(texts)
	if embeddingOpts.BatchSize > 0 {
		batchSize = embeddingOpts.BatchSize
	}
	vecs := make([][]float64, 0, len(texts))
	for start := 0; start < len(texts); start += batchSize {
		end := min(start+batchSize, len(texts))
		batchVecs, err := c.embedBatch(ctx, texts[start:end], dimensions)
		if err != nil {
			return nil, err
		}
		vecs = append(vecs, batchVecs...)
	}
	return vecs, nil
}

func (c *openaiLLMClient) embedBatch(ctx context.Context, texts []string, dimensions int) ([][]float64, error) {
	params := openai.EmbeddingNewParams{
		Model: openai.EmbeddingModel(c.embeddingModel),
		Input: openai.EmbeddingNewParamsInputUnion{OfArrayOfStrings: texts},
	}
	if dimensions > 0 {
		params.Dimensions = openai.Int(int64(dimensions))
	}

	resp, err := c.client.Embeddings.New(ctx, params)
	if err != nil {
		return nil, xerr.Wrapf(err, "请求模型向量接口失败")
	}
	if resp == nil {
		return nil, xerr.Internal("模型返回的向量为空", nil)
	}

	vecs := make([][]float64, 0, len(resp.Data))
	for _, item := range resp.Data {
		vec := make([]float64, 0, len(item.Embedding))
		for _, v := range item.Embedding {
			vec = append(vec, float64(v))
		}
		vecs = append(vecs, vec)
	}
	return vecs, nil
}

func (c *openaiLLMClient) EmbedOne(ctx context.Context, text string, dimensions int) ([]float64, error) {
	vecs, err := c.Embed(ctx, []string{text}, dimensions)
	if err != nil {
		return nil, err
	}
	if len(vecs) == 0 {
		return nil, xerr.Internal("模型返回的向量为空", nil)
	}
	return vecs[0], nil
}

// Vision 使用 OpenAI 兼容的 chat/completions 多模态接口描述图片。
//
// 可选参数复用通用聊天 ModelCallOption（温度、TopP、MaxTokens、工具等）。
// 返回结构化 VisionResult，Description 由 corellm.ParseVisionDescription 规整。
func (c *openaiLLMClient) Vision(ctx context.Context, prompt string, imageBase64 string, mime string, opts ...corellm.ModelCallOption) (*corellm.VisionResult, error) {
	if err := c.validateChatConfig(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(imageBase64) == "" {
		return nil, xerr.BadRequest("图片内容不能为空")
	}
	mime = strings.TrimSpace(mime)
	if mime == "" {
		mime = "image/jpeg"
	}
	dataURL := "data:" + mime + ";base64," + imageBase64
	// 复用聊天参数合并逻辑，并补上 client 级默认温度。
	chatCallOpts := make([]corellm.ModelCallOption, 0, len(opts)+1)
	if c.defaultTemperature != nil {
		chatCallOpts = append(chatCallOpts, corellm.WithTemperature(*c.defaultTemperature))
	}
	chatCallOpts = append(chatCallOpts, opts...)
	chatOpts := corellm.NewVisionOptions(chatCallOpts...)

	params := openai.ChatCompletionNewParams{
		Model: openai.ChatModel(c.model),
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.UserMessage([]openai.ChatCompletionContentPartUnionParam{
				openai.TextContentPart(prompt),
				openai.ImageContentPart(openai.ChatCompletionContentPartImageImageURLParam{
					URL: dataURL,
				}),
			}),
		},
	}
	if chatOpts.Temperature != nil {
		params.Temperature = openai.Float(*chatOpts.Temperature)
	}
	if chatOpts.TopP != nil {
		params.TopP = openai.Float(*chatOpts.TopP)
	}
	if chatOpts.MaxTokens != nil {
		params.MaxTokens = openai.Int(*chatOpts.MaxTokens)
	}
	params.Tools = toOpenAITools(chatOpts.Tools)
	params.ToolChoice = toOpenAIToolChoice(chatOpts.ToolChoice)

	resp, err := c.client.Chat.Completions.New(ctx, params)
	if err != nil {
		return nil, xerr.Wrapf(err, "请求多模态模型接口失败")
	}
	if resp == nil || len(resp.Choices) == 0 {
		return nil, xerr.Internal("多模态模型返回为空", nil)
	}
	text := resp.Choices[0].Message.Content
	return &corellm.VisionResult{
		Description: corellm.ParseVisionDescription(text),
		Text:        text,
		Model:       resp.Model,
		Provider:    "openai",
		ID:          resp.ID,
		StopReason:  resp.Choices[0].FinishReason,
		Usage: corellm.TokenUsage{
			InputTokens:  resp.Usage.PromptTokens,
			OutputTokens: resp.Usage.CompletionTokens,
			TotalTokens:  resp.Usage.TotalTokens,
		},
		RawJSON: resp.RawJSON(),
	}, nil
}

func (c *openaiLLMClient) validateChatConfig() error {
	if c.apiKey == "" {
		return xerr.Internal("模型 API Key 未配置", nil)
	}
	if c.model == "" {
		return xerr.Internal("模型名称未配置", nil)
	}
	return nil
}

func (c *openaiLLMClient) validateEmbeddingConfig() error {
	if c.apiKey == "" {
		return xerr.Internal("模型 API Key 未配置", nil)
	}
	if c.embeddingModel == "" {
		return xerr.Internal("向量模型名称未配置", nil)
	}
	return nil
}

func toOpenAIMessages(messages []*corellm.Message) []openai.ChatCompletionMessageParamUnion {
	out := make([]openai.ChatCompletionMessageParamUnion, 0, len(messages))
	for _, m := range messages {
		if m == nil {
			continue
		}
		switch m.Role {
		case corellm.SystemRole:
			out = append(out, openai.ChatCompletionMessageParamUnion{
				OfSystem: &openai.ChatCompletionSystemMessageParam{
					Content: openai.ChatCompletionSystemMessageParamContentUnion{
						OfString: openai.String(m.Content),
					},
				},
			})
		case corellm.AssistantRole:
			assistant := &openai.ChatCompletionAssistantMessageParam{}
			if m.Content != "" {
				assistant.Content = openai.ChatCompletionAssistantMessageParamContentUnion{
					OfString: openai.String(m.Content),
				}
			}
			assistant.ToolCalls = toOpenAIToolCallParams(m.ToolCalls)
			out = append(out, openai.ChatCompletionMessageParamUnion{OfAssistant: assistant})
		case corellm.ToolRole:
			if strings.TrimSpace(m.ToolCallID) == "" {
				continue
			}
			out = append(out, openai.ChatCompletionMessageParamUnion{
				OfTool: &openai.ChatCompletionToolMessageParam{
					Content: openai.ChatCompletionToolMessageParamContentUnion{
						OfString: openai.String(m.Content),
					},
					ToolCallID: strings.TrimSpace(m.ToolCallID),
				},
			})
		default:
			out = append(out, openai.ChatCompletionMessageParamUnion{
				OfUser: &openai.ChatCompletionUserMessageParam{
					Content: openai.ChatCompletionUserMessageParamContentUnion{
						OfString: openai.String(m.Content),
					},
				},
			})
		}
	}
	return out
}

func toOpenAIToolCallParams(calls []corellm.LLMToolCall) []openai.ChatCompletionMessageToolCallUnionParam {
	if len(calls) == 0 {
		return nil
	}
	out := make([]openai.ChatCompletionMessageToolCallUnionParam, 0, len(calls))
	for _, call := range calls {
		name := strings.TrimSpace(call.Name)
		id := strings.TrimSpace(call.ID)
		if name == "" || id == "" {
			continue
		}
		out = append(out, openai.ChatCompletionMessageToolCallUnionParam{
			OfFunction: &openai.ChatCompletionMessageFunctionToolCallParam{
				ID: id,
				Function: openai.ChatCompletionMessageFunctionToolCallFunctionParam{
					Name:      name,
					Arguments: toolCallRawInput(call),
				},
			},
		})
	}
	return out
}

func toolCallRawInput(call corellm.LLMToolCall) string {
	if strings.TrimSpace(call.RawInput) != "" {
		return call.RawInput
	}
	if len(call.Input) == 0 {
		return "{}"
	}
	data, err := json.Marshal(call.Input)
	if err != nil {
		return "{}"
	}
	return string(data)
}

var _ corellm.ToolCallingClient = (*openaiLLMClient)(nil)
