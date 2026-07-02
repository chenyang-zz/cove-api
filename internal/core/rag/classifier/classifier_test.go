package classifier

import (
	"context"
	"errors"
	"strings"
	"testing"

	corellm "github.com/boxify/api-go/internal/core/llm"
)

type fakeTextClient struct {
	answer     string
	err        error
	lastPrompt string
	lastTemp   float64
	lastTokens int64
}

func (f *fakeTextClient) Classify(ctx context.Context, prompt string, temperature float64, maxTokens int64) (string, error) {
	f.lastPrompt = prompt
	f.lastTemp = temperature
	f.lastTokens = maxTokens
	return f.answer, f.err
}

type fakeParser struct {
	err error
}

func (f fakeParser) Repair(input string) (string, error) {
	return input, nil
}

func (f fakeParser) Unmarshal(input string, out any) error {
	if f.err != nil {
		return f.err
	}
	return defaultParser().Unmarshal(input, out)
}

func TestClassifierBuildsDefaultPromptAndAppliesOptions(t *testing.T) {
	// 验证默认分类提示词会通过包内模板渲染已有标签和裁剪后的正文，并应用温度和 maxTokens 配置。
	client := &fakeTextClient{answer: `["技术"]`}
	classifier := NewClassifier(
		WithTextClient(client),
		WithSnippetRunes(5),
		WithTemperature(0.5),
		WithMaxTokens(99),
	)

	out, err := classifier.Classify(context.Background(), Input{
		Content:      "1234567890",
		ExistingTags: []string{"技术", "生活"},
	})
	if err != nil {
		t.Fatalf("Classify error = %v", err)
	}
	if len(out.Tags) != 1 || out.Tags[0] != "技术" {
		t.Fatalf("tags = %#v, want 技术", out.Tags)
	}
	if !strings.Contains(client.lastPrompt, "技术、生活") || strings.Contains(client.lastPrompt, "67890") {
		t.Fatalf("prompt = %q, want existing tags and clipped content", client.lastPrompt)
	}
	if client.lastTemp != 0.5 || client.lastTokens != 99 {
		t.Fatalf("options temp/tokens = %v/%d, want 0.5/99", client.lastTemp, client.lastTokens)
	}
}

func TestClassifierUsesExternalPromptAsFinalText(t *testing.T) {
	// 验证外部传入的 prompt 已经是最终文本，classifier 不再执行模板参数替换。
	client := &fakeTextClient{answer: `["技术"]`}
	prompt := "已有：{{ .Existing }}\n文本：{{ .Content }}"
	out, err := NewClassifier(WithTextClient(client), WithPrompt(prompt)).Classify(context.Background(), Input{
		Content:      "1234567890",
		ExistingTags: []string{"技术", "生活"},
	})
	if err != nil {
		t.Fatalf("Classify error = %v", err)
	}
	if len(out.Tags) != 1 || out.Tags[0] != "技术" {
		t.Fatalf("tags = %#v, want 技术", out.Tags)
	}
	if client.lastPrompt != prompt {
		t.Fatalf("prompt = %q, want external final prompt unchanged", client.lastPrompt)
	}
}

func TestClassifierParsesJSONVariantsAndNormalizesTags(t *testing.T) {
	// 验证分类器能解析纯 JSON、markdown fence 和夹杂文本的数组，并规整标签数量与长度。
	cases := []struct {
		name   string
		answer string
		want   []string
	}{
		{name: "json", answer: `[" 技术 ","学习","第三个"]`, want: []string{"技术", "学习"}},
		{name: "fence", answer: "```json\n[\"财经\"]\n```", want: []string{"财经"}},
		{name: "mixed", answer: "结果如下：[\"12345678901234567890\", 123, \"生活\"]", want: []string{"1234567890123456", "生活"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := &fakeTextClient{answer: tc.answer}
			out, err := NewClassifier(WithTextClient(client)).Classify(context.Background(), Input{Content: "content"})
			if err != nil {
				t.Fatalf("Classify error = %v", err)
			}
			if strings.Join(out.Tags, ",") != strings.Join(tc.want, ",") {
				t.Fatalf("tags = %#v, want %#v", out.Tags, tc.want)
			}
		})
	}
}

func TestClassifierErrorsReturnEmptyTags(t *testing.T) {
	// 验证模型调用失败或 JSON 解析失败不会阻断主流程，而是返回空标签。
	clientErr := &fakeTextClient{err: errors.New("llm failed")}
	out, err := NewClassifier(WithTextClient(clientErr)).Classify(context.Background(), Input{Content: "content"})
	if err != nil {
		t.Fatalf("Classify client error = %v, want nil", err)
	}
	if len(out.Tags) != 0 {
		t.Fatalf("client error tags = %#v, want empty", out.Tags)
	}

	parseErr := &fakeTextClient{answer: `["技术"]`}
	out, err = NewClassifier(WithTextClient(parseErr), WithParser(fakeParser{err: errors.New("parse failed")})).Classify(context.Background(), Input{Content: "content"})
	if err != nil {
		t.Fatalf("Classify parser error = %v, want nil", err)
	}
	if len(out.Tags) != 0 {
		t.Fatalf("parser error tags = %#v, want empty", out.Tags)
	}
}

func TestClassifierErrorsWhenClientMissing(t *testing.T) {
	// 验证 NewClassifier 默认不携带模型客户端，调用时返回明确错误。
	_, err := NewClassifier().Classify(context.Background(), Input{Content: "content"})
	if err == nil || !strings.Contains(err.Error(), "rag classifier text client is nil") {
		t.Fatalf("Classify missing client error = %v, want rag classifier text client is nil", err)
	}
}

func TestClassifierInputClientOverridesDefaultClient(t *testing.T) {
	// 验证请求级 client 会覆盖构造级 client，便于按用户模型配置执行分类。
	defaultClient := &fakeTextClient{answer: `["默认"]`}
	inputClient := &fakeTextClient{answer: `["请求"]`}

	out, err := NewClassifier(WithTextClient(defaultClient)).Classify(context.Background(), Input{Content: "content"}, WithInputTextClient(inputClient))
	if err != nil {
		t.Fatalf("Classify error = %v", err)
	}
	if len(out.Tags) != 1 || out.Tags[0] != "请求" {
		t.Fatalf("tags = %#v, want 请求 from input client", out.Tags)
	}
	if defaultClient.lastPrompt != "" {
		t.Fatalf("default client prompt = %q, want unused", defaultClient.lastPrompt)
	}
	if inputClient.lastPrompt == "" {
		t.Fatal("input client prompt is empty, want request-level client used")
	}
}

type fakeLLMClient struct {
	answer       string
	err          error
	lastMessages []*corellm.Message
	lastOptions  corellm.ModelCallOptions
}

func (f *fakeLLMClient) Invoke(ctx context.Context, messages []*corellm.Message, opts ...corellm.ModelCallOption) (string, error) {
	f.lastMessages = messages
	f.lastOptions = corellm.NewChatOptions(opts...)
	return f.answer, f.err
}

func (f *fakeLLMClient) Stream(ctx context.Context, messages []*corellm.Message, opts ...corellm.ModelCallOption) (<-chan string, error) {
	ch := make(chan string)
	close(ch)
	return ch, nil
}

func (f *fakeLLMClient) Embed(ctx context.Context, texts []string, dimensions int, opts ...corellm.EmbeddingOption) ([][]float64, error) {
	return nil, nil
}

func (f *fakeLLMClient) EmbedOne(ctx context.Context, text string, dimensions int) ([]float64, error) {
	return nil, nil
}

func TestClassifierWithLLMClientInvokesModel(t *testing.T) {
	// 验证 WithClient 会把 core llm client 适配成分类器文本客户端，并透传温度与 maxTokens。
	client := &fakeLLMClient{answer: `["技术"]`}
	out, err := NewClassifier(WithClient(client), WithTemperature(0.6), WithMaxTokens(88)).Classify(context.Background(), Input{Content: "content"})
	if err != nil {
		t.Fatalf("Classify error = %v", err)
	}
	if len(out.Tags) != 1 || out.Tags[0] != "技术" {
		t.Fatalf("tags = %#v, want 技术", out.Tags)
	}
	if len(client.lastMessages) != 1 || client.lastMessages[0].Role != corellm.UserRole || !strings.Contains(client.lastMessages[0].Content, "content") {
		t.Fatalf("messages = %#v, want one user prompt containing content", client.lastMessages)
	}
	if client.lastOptions.Temperature == nil || *client.lastOptions.Temperature != 0.6 || client.lastOptions.MaxTokens == nil || *client.lastOptions.MaxTokens != 88 {
		t.Fatalf("options = %#v, want temperature=0.6 maxTokens=88", client.lastOptions)
	}
}

func TestClassifierInputLLMClientOverridesDefaultClient(t *testing.T) {
	// 验证 WithInputClient 传入的 core llm client 会覆盖构造级默认 client。
	defaultClient := &fakeLLMClient{answer: `["默认"]`}
	inputClient := &fakeLLMClient{answer: `["请求"]`}

	out, err := NewClassifier(WithClient(defaultClient)).Classify(context.Background(), Input{Content: "content"}, WithInputClient(inputClient))
	if err != nil {
		t.Fatalf("Classify error = %v", err)
	}
	if len(out.Tags) != 1 || out.Tags[0] != "请求" {
		t.Fatalf("tags = %#v, want 请求 from input llm client", out.Tags)
	}
	if len(defaultClient.lastMessages) != 0 {
		t.Fatalf("default client messages = %#v, want unused", defaultClient.lastMessages)
	}
	if len(inputClient.lastMessages) == 0 {
		t.Fatal("input client messages is empty, want request-level llm client used")
	}
}

func TestClassifierClientOptionsUseLastConstructionClient(t *testing.T) {
	// 验证构造级 TextClient 和 LLM client 同时传入时，后传的 client option 生效。
	textClient := &fakeTextClient{answer: `["文本"]`}
	llmClient := &fakeLLMClient{answer: `["模型"]`}

	out, err := NewClassifier(WithClient(llmClient), WithTextClient(textClient)).Classify(context.Background(), Input{Content: "content"})
	if err != nil {
		t.Fatalf("Classify with text client last error = %v", err)
	}
	if len(out.Tags) != 1 || out.Tags[0] != "文本" {
		t.Fatalf("tags with text client last = %#v, want 文本", out.Tags)
	}
	if len(llmClient.lastMessages) != 0 {
		t.Fatalf("llm messages = %#v, want unused when text client is last", llmClient.lastMessages)
	}

	llmClient = &fakeLLMClient{answer: `["模型"]`}
	textClient = &fakeTextClient{answer: `["文本"]`}
	out, err = NewClassifier(WithTextClient(textClient), WithClient(llmClient)).Classify(context.Background(), Input{Content: "content"})
	if err != nil {
		t.Fatalf("Classify with llm client last error = %v", err)
	}
	if len(out.Tags) != 1 || out.Tags[0] != "模型" {
		t.Fatalf("tags with llm client last = %#v, want 模型", out.Tags)
	}
	if textClient.lastPrompt != "" {
		t.Fatalf("text client prompt = %q, want unused when llm client is last", textClient.lastPrompt)
	}
}

func TestClassifierInputClientOptionsUseLastRequestClient(t *testing.T) {
	// 验证请求级 TextClient 和 LLM client 同时传入时，后传的 input client option 生效。
	textClient := &fakeTextClient{answer: `["文本"]`}
	llmClient := &fakeLLMClient{answer: `["模型"]`}

	out, err := NewClassifier().Classify(context.Background(), Input{Content: "content"}, WithInputClient(llmClient), WithInputTextClient(textClient))
	if err != nil {
		t.Fatalf("Classify with input text client last error = %v", err)
	}
	if len(out.Tags) != 1 || out.Tags[0] != "文本" {
		t.Fatalf("tags with input text client last = %#v, want 文本", out.Tags)
	}
	if len(llmClient.lastMessages) != 0 {
		t.Fatalf("llm messages = %#v, want unused when input text client is last", llmClient.lastMessages)
	}

	llmClient = &fakeLLMClient{answer: `["模型"]`}
	textClient = &fakeTextClient{answer: `["文本"]`}
	out, err = NewClassifier().Classify(context.Background(), Input{Content: "content"}, WithInputTextClient(textClient), WithInputClient(llmClient))
	if err != nil {
		t.Fatalf("Classify with input llm client last error = %v", err)
	}
	if len(out.Tags) != 1 || out.Tags[0] != "模型" {
		t.Fatalf("tags with input llm client last = %#v, want 模型", out.Tags)
	}
	if textClient.lastPrompt != "" {
		t.Fatalf("text client prompt = %q, want unused when input llm client is last", textClient.lastPrompt)
	}
}
