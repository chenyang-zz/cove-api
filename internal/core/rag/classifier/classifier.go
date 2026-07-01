package classifier

import (
	"context"
	"errors"
	"strings"

	coreprompt "github.com/boxify/api-go/internal/core/prompt"
	ragprompt "github.com/boxify/api-go/internal/core/rag/prompt"
	"github.com/boxify/api-go/internal/core/valuex"
)

type Classifier struct {
	Options
	client TextClient
}

func NewClassifier(client TextClient, opts ...Option) *Classifier {
	classifier := &Classifier{
		Options: Options{
			Prompt:       defaultPrompt,
			Temperature:  defaultTemperature,
			MaxTokens:    defaultMaxTokens,
			SnippetRunes: defaultSnippetRunes,
			Parser:       defaultParser(),
			promptTmpl:   true,
		},
		client: client,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(&classifier.Options)
		}
	}
	return classifier
}

// Classify 分类
func (c *Classifier) Classify(ctx context.Context, input Input) (*Result, error) {
	if c == nil || c.client == nil {
		return nil, errors.New("rag classifier text client is nil")
	}
	if c.Parser == nil {
		return nil, errors.New("rag classifier json parser is nil")
	}

	prompt, err := c.buildPrompt(input)
	if err != nil {
		return nil, err
	}

	// 分类是辅助能力：模型调用失败不能阻断文档主流程。
	answer, err := c.client.Classify(ctx, prompt, c.Temperature, c.MaxTokens)
	if err != nil {
		return &Result{Tags: []string{}}, nil
	}
	return &Result{Tags: c.parseTags(answer)}, nil
}

// buildPrompt 构建提示词
func (c *Classifier) buildPrompt(input Input) (string, error) {
	if !c.promptTmpl {
		return c.Prompt, nil
	}
	existing := "（暂无，可自行创造）"
	if len(input.ExistingTags) > 0 {
		existing = strings.Join(input.ExistingTags, "、")
	}
	content := valuex.TruncateRunes(input.Content, c.SnippetRunes)
	return coreprompt.RenderText(c.Prompt, ragprompt.ContentClassifierData{
		Existing: existing,
		Content:  content,
	})
}

// parseTags 解析标签
func (c *Classifier) parseTags(answer string) []string {
	text := extractJSONArray(answer)
	if text == "" {
		return []string{}
	}
	var raw []any
	if err := c.Parser.Unmarshal(text, &raw); err != nil {
		return []string{}
	}

	// 模型输出不稳定，这里只保留字符串、去空白、截断并限制数量。
	tags := make([]string, 0, 2)
	for _, item := range raw {
		tag := valuex.TruncateRunes(valuex.String(item), 16)
		if tag == "" {
			continue
		}
		tags = append(tags, tag)
		if len(tags) == 2 {
			break
		}
	}
	return tags
}

// extractJSONArray 提取 JSON 数组
func extractJSONArray(answer string) string {
	text := strings.TrimSpace(answer)
	if strings.HasPrefix(text, "```") {
		text = strings.Trim(text, "`")
		text = strings.TrimSpace(strings.TrimPrefix(text, "json"))
	}
	start := strings.Index(text, "[")
	end := strings.LastIndex(text, "]")
	if start == -1 || end < start {
		return ""
	}
	return text[start : end+1]
}
