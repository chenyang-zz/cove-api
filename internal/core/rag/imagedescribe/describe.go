package imagedescribe

import (
	"context"
	"encoding/base64"
	"errors"

	corellm "github.com/boxify/api-go/internal/core/llm"
	"github.com/boxify/api-go/internal/core/rag/imagecompress"
)

// Describer 压缩图片并调用 corellm.VisionClient 生成结构化描述。
//
// Describer 不绑定具体模型 SDK；默认直接使用 corellm.VisionClient，无需外部适配器。
// 可通过 WithVisionClient 在构造后覆盖，或直接向 NewDescriber 传入自定义 VisionClient。
type Describer struct {
	Options
	vision corellm.VisionClient
}

// NewDescriber 创建图片描述器。
//
// vision 为 nil 时构造仍会成功，后续 Describe 会返回明确错误，便于测试或延迟注入。
func NewDescriber(vision corellm.VisionClient, opts ...Option) *Describer {
	describer := &Describer{
		Options: Options{
			Prompt:     defaultPrompt,
			MaxTokens:  defaultMaxTokens,
			Compressor: defaultCompressor(),
		},
		vision: vision,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(&describer.Options)
		}
	}
	// Option 可覆盖 vision（自定义客户端）。
	if describer.Options.Vision != nil {
		describer.vision = describer.Options.Vision
	}
	return describer
}

// Describe 根据图片内容生成结构化描述。
//
// Describe 会先压缩图片，再调用 VisionClient；返回的 Description 来自 VisionResult 的结构化字段。
func (d *Describer) Describe(ctx context.Context, input Input) (*Description, error) {
	if d == nil || d.vision == nil {
		return nil, errors.New("rag image describer vision client is nil")
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
	opts := make([]corellm.ModelCallOption, 0, 1)
	if d.MaxTokens > 0 {
		opts = append(opts, corellm.WithMaxTokens(d.MaxTokens))
	}
	result, err := d.vision.Vision(ctx, d.Prompt, imageBase64, compressed.MIME, opts...)
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, errors.New("rag image describer vision result is nil")
	}
	desc := result.Description
	return &desc, nil
}
