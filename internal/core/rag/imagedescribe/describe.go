package imagedescribe

import (
	"context"
	"encoding/base64"
	"errors"
	"strings"

	"github.com/boxify/api-go/internal/core/rag/imagecompress"
	"github.com/boxify/api-go/internal/core/valuex"
)

const (
	maxDescriptionRunes = 2000
	maxOCRTextRunes     = 2000
	maxSceneRunes       = 64
)

type Describer struct {
	Options
	client VisionClient
}

func NewDescriber(client VisionClient, opts ...Option) *Describer {
	describer := &Describer{
		Options: Options{
			Prompt:     defaultPrompt,
			MaxTokens:  defaultMaxTokens,
			Compressor: defaultCompressor(),
			Parser:     defaultParser(),
		},
		client: client,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(&describer.Options)
		}
	}
	return describer
}

// Describe 根据图片内容生成描述。
func (d *Describer) Describe(ctx context.Context, input Input) (*Description, error) {
	if d == nil || d.client == nil {
		return nil, errors.New("rag image describer vision client is nil")
	}
	if d.Parser == nil {
		return nil, errors.New("rag image describer json parser is nil")
	}
	if d.Compressor == nil {
		return nil, errors.New("rag image describer compressor is nil")
	}

	compressed, err := d.Compressor.Compress(imagecompress.Input{
		Data:    input.Data,
		FileExt: input.FileExt,
	})
	if err != nil {
		return nil, err
	}

	// 多模态接口通常接收 base64 图片内容；压缩器负责控制体积和 MIME。
	imageBase64 := base64.StdEncoding.EncodeToString(compressed.Data)
	answer, err := d.client.Describe(ctx, d.Prompt, imageBase64, compressed.MIME, d.MaxTokens)
	if err != nil {
		return nil, err
	}

	return d.parse(answer), nil
}

type rawDescription struct {
	Description any `json:"description"`
	OCRText     any `json:"ocr_text"`
	Objects     any `json:"objects"`
	Scene       any `json:"scene"`
}

func (d *Describer) parse(answer string) *Description {
	var raw rawDescription
	if err := d.Parser.Unmarshal(answer, &raw); err != nil {
		// 模型没有按 JSON 返回时，将原文作为描述兜底，避免上层丢失可检索文本。
		return &Description{
			Description: truncate(answer, maxDescriptionRunes),
			Objects:     []string{},
		}
	}

	// 字段规整集中处理：裁剪长文本，只保留 objects 中的字符串，隔离模型输出的不稳定类型。
	return &Description{
		Description: truncate(valuex.String(raw.Description), maxDescriptionRunes),
		OCRText:     truncate(valuex.String(raw.OCRText), maxOCRTextRunes),
		Objects:     valuex.StringList(raw.Objects),
		Scene:       truncate(valuex.String(raw.Scene), maxSceneRunes),
	}
}

func truncate(text string, maxRunes int) string {
	text = strings.TrimSpace(text)
	if maxRunes <= 0 {
		return ""
	}
	runes := []rune(text)
	if len(runes) <= maxRunes {
		return text
	}
	return string(runes[:maxRunes])
}
