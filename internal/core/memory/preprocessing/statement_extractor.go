/**
 * @Time   : 2026/6/23 01:01
 * @Author : chenyangzhao542@gmail.com
 * @File   : statement_extractor.go
 **/

package preprocessing

import (
	"context"

	"github.com/boxify/api-go/internal/core/jsonx"
	"github.com/boxify/api-go/internal/core/llm"
	"github.com/boxify/api-go/internal/core/memory"
	"github.com/boxify/api-go/internal/xerr"
)

// 原子陈述抽取: 把一段文本切成带类型/时间属性的原子陈述句
//
// 调用对话模型， 按受控的陈述类型 (FACT/OPINION/PREDICTION/SUGGRESSION) 和
// 时间类型 (STATIC/DYNAMIC/ATEMPORAL) 标注. 并标记指代是否为解析.
// 失败返回空列表, 不中断流水线，

type StatementExtractor struct {
	llm        llm.Client
	prompt     memory.Prompter
	jsonParser jsonx.Parser
}

func NewStatementExtractor(llm llm.Client, prompt memory.Prompter, jsonParser jsonx.Parser) *StatementExtractor {
	return &StatementExtractor{
		llm:        llm,
		prompt:     prompt,
		jsonParser: jsonParser,
	}
}

// Extract 从一段文本抽取原子陈述句
func (e *StatementExtractor) Extract(ctx context.Context, content string, context string) ([]*memory.ExtractedStatement, error) {
	promptText, err := e.prompt.StatementExtract(&memory.StatementPromptInput{
		Content: content,
		Context: context,
	})
	if err != nil {
		return nil, xerr.Wrapf(err, "parse statement extract prompt failed: %v", err)
	}

	answer, err := e.llm.Invoke(ctx, []*llm.Message{
		llm.UserMessage(promptText),
	})
	if err != nil {
		return nil, xerr.Wrapf(err, "invoke llm failed: %v", err)
	}

	res, err := jsonx.Parse[[]*memory.ExtractedStatement](e.jsonParser, answer)
	if err != nil {
		return nil, err
	}

	return res, nil
}
