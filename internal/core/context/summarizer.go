package context

import (
	stdcontext "context"
	"encoding/json"
	"errors"
	"strings"

	contextprompt "github.com/boxify/api-go/internal/core/context/prompt"
	"github.com/boxify/api-go/internal/core/llm"
	coreprompt "github.com/boxify/api-go/internal/core/prompt"
)

// defaultSummarizer 使用非 nil 模型客户端返回默认的 LLM 摘要器。
// NewManager 负责保证仅在已经注入 client 时调用该函数。
func defaultSummarizer(client llm.Client) Summarizer {
	return NewLLMSummarizer(client)
}

// LLMSummarizer 使用通用 llm.Client 生成滚动摘要。
type LLMSummarizer struct {
	client llm.Client
}

// NewLLMSummarizer 创建模型摘要器；client 为 nil 时 Summarize 返回错误。
func NewLLMSummarizer(client llm.Client) *LLMSummarizer {
	return &LLMSummarizer{client: client}
}

// Summarize 使用温度 0 合并已有摘要和新增消息。
func (s *LLMSummarizer) Summarize(ctx stdcontext.Context, previousSummary string, messages []*llm.Message, maxTokens int64) (string, error) {
	if s == nil || s.client == nil {
		return "", errors.New("context summarizer client is nil")
	}
	transcript, err := json.Marshal(messages)
	if err != nil {
		return "", err
	}
	promptText, err := coreprompt.Render(contextprompt.Templates, contextprompt.SummaryTemplate, contextprompt.SummaryData{
		PreviousSummary: strings.TrimSpace(previousSummary),
		Transcript:      string(transcript),
	})
	if err != nil {
		return "", err
	}
	text, err := s.client.Invoke(ctx, []*llm.Message{llm.UserMessage(promptText)}, llm.WithTemperature(0), llm.WithMaxTokens(maxTokens))
	if err != nil {
		return "", err
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return "", errors.New("context summarizer returned empty text")
	}
	return text, nil
}
