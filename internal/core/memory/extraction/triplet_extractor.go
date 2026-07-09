/**
 * @Time   : 2026/6/23 09:24
 * @Author : chenyangzhao542@gmail.com
 * @File   : triplet_extractor.go
 **/

package extraction

import (
	"context"
	"time"

	"github.com/boxify/api-go/internal/core/jsonx"
	"github.com/boxify/api-go/internal/core/llm"
	"github.com/boxify/api-go/internal/core/memory"
	"github.com/boxify/api-go/internal/util"
	"github.com/boxify/api-go/internal/xerr"
	"golang.org/x/sync/errgroup"
)

// 三元组萃取: 从单条陈述抽取实体与 (主语, 谓词, 宾语)  三元组.
//
// 按受控词表 (13 类实体 + 13 类谓词) 约束 LLM 输出. 指代未解析的陈述直接跳过.
// 失败返回空结果

type TripletExtractor struct {
	llm        llm.Client
	prompt     memory.Prompter
	jsonParser jsonx.Parser
}

func NewTripletExtractor(llm llm.Client, prompt memory.Prompter, jsonParser jsonx.Parser) *TripletExtractor {
	return &TripletExtractor{
		llm:        llm,
		prompt:     prompt,
		jsonParser: jsonParser,
	}
}

// Extract 从单条抽取实体三元组
func (e *TripletExtractor) Extract(ctx context.Context, statement *memory.ExtractedStatement, context string,
	dialogAt time.Time) (*memory.TripletExtractionResult,
	error) {
	if statement.HasUnsolvedReference {
		return &memory.TripletExtractionResult{}, nil
	}

	promptText, err := e.prompt.TripletExtract(&memory.TripletPromptInput{
		Statement:   statement.Statement,
		Context:     context,
		EntityTypes: []string{},
		Predicates:  []string{},
		ValidAt:     "NULL",
		InvalidAt:   "NULL",
		DialogAt:    util.ISO8601OrNULL(dialogAt),
	})
	if err != nil {
		return nil, xerr.Wrapf(err, "解析抽取三元组提示词失败")
	}

	response, err := e.llm.Invoke(ctx, []*llm.Message{
		llm.UserMessage(promptText),
	})
	if err != nil {
		return nil, xerr.Wrapf(err, "invoke llm failed: %v", err)
	}

	res, err := jsonx.Parse[*memory.TripletExtractionResult](e.jsonParser, response)
	if err != nil {
		return nil, err
	}

	return res, nil
}

// BatchExtract 批量抽取实体三元组
func (e *TripletExtractor) BatchExtract(ctx context.Context, statements []*memory.ExtractedStatement, context string,
	dialogAt time.Time, concurrency int) ([]*memory.TripletExtractionResult, error) {

	if concurrency == 0 {
		concurrency = 4
	}

	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(concurrency)

	results := make([]*memory.TripletExtractionResult, len(statements))

	for i, statement := range statements {
		g.Go(func() error {
			res, err := e.Extract(ctx, statement, context, dialogAt)
			if err != nil {
				return err
			}
			results[i] = res
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return nil, xerr.Wrapf(err, "batch extract failed: %v", err)
	}

	return results, nil
}
