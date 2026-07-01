package classifier

import (
	"context"
	"errors"
	"strings"
	"testing"
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
	classifier := NewClassifier(client,
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
	out, err := NewClassifier(client, WithPrompt(prompt)).Classify(context.Background(), Input{
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
			out, err := NewClassifier(client).Classify(context.Background(), Input{Content: "content"})
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
	out, err := NewClassifier(clientErr).Classify(context.Background(), Input{Content: "content"})
	if err != nil {
		t.Fatalf("Classify client error = %v, want nil", err)
	}
	if len(out.Tags) != 0 {
		t.Fatalf("client error tags = %#v, want empty", out.Tags)
	}

	parseErr := &fakeTextClient{answer: `["技术"]`}
	out, err = NewClassifier(parseErr, WithParser(fakeParser{err: errors.New("parse failed")})).Classify(context.Background(), Input{Content: "content"})
	if err != nil {
		t.Fatalf("Classify parser error = %v, want nil", err)
	}
	if len(out.Tags) != 0 {
		t.Fatalf("parser error tags = %#v, want empty", out.Tags)
	}
}
