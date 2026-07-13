package imagedescribe

import (
	corellm "github.com/boxify/api-go/internal/core/llm"
	coreprompt "github.com/boxify/api-go/internal/core/prompt"
	"github.com/boxify/api-go/internal/core/rag/imagecompress"
	ragprompt "github.com/boxify/api-go/internal/core/rag/prompt"
)

const (
	defaultMaxTokens = int64(1024)
)

var defaultPrompt = coreprompt.MustRender(ragprompt.Templates, ragprompt.ImageDescriptionTemplate, nil)

// Options 定义 Describer 的长期配置。
type Options struct {
	Prompt     string
	MaxTokens  int64
	Compressor Compressor
	// Vision 可选覆盖构造时传入的 corellm.VisionClient。
	Vision corellm.VisionClient
}

// Option 修改 Describer 的长期配置。
type Option func(*Options)

// WithPrompt 设置发送给视觉模型的最终提示词文本。
func WithPrompt(prompt string) Option {
	return func(opts *Options) {
		if prompt != "" {
			opts.Prompt = prompt
		}
	}
}

// WithMaxTokens 设置视觉模型最大输出 token 数。
func WithMaxTokens(maxTokens int64) Option {
	return func(opts *Options) {
		if maxTokens > 0 {
			opts.MaxTokens = maxTokens
		}
	}
}

// WithCompressor 设置图片压缩器。
func WithCompressor(compressor Compressor) Option {
	return func(opts *Options) {
		if compressor != nil {
			opts.Compressor = compressor
		}
	}
}

// WithVisionClient 覆盖默认注入的 corellm.VisionClient。
func WithVisionClient(vision corellm.VisionClient) Option {
	return func(opts *Options) {
		if vision != nil {
			opts.Vision = vision
		}
	}
}

// defaultCompressor 返回图片描述默认使用的压缩器。
func defaultCompressor() Compressor {
	return imagecompress.NewCompressor()
}
