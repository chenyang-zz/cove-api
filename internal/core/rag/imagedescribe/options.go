package imagedescribe

import (
	"github.com/boxify/api-go/internal/core/jsonx"
	"github.com/boxify/api-go/internal/core/rag/imagecompress"
)

const (
	defaultMaxTokens = int64(1024)
	defaultPrompt    = `请仔细观察这张图片，用中文输出 JSON，包含以下字段：
- description: 对图片内容的详细描述（一段话，尽量具体，便于后续检索）
- ocr_text: 图片中出现的所有文字（没有则空字符串）
- objects: 图片中的主要物体列表（字符串数组）
- scene: 图片的场景类别（如：办公室、户外、文档截图、人物 等，一个词）

只输出 JSON，不要任何额外说明。`
)

type Options struct {
	Prompt     string
	MaxTokens  int64
	Compressor Compressor
	Parser     jsonx.Parser
}

type Option func(*Options)

func WithPrompt(prompt string) Option {
	return func(opts *Options) {
		if prompt != "" {
			opts.Prompt = prompt
		}
	}
}

func WithMaxTokens(maxTokens int64) Option {
	return func(opts *Options) {
		if maxTokens > 0 {
			opts.MaxTokens = maxTokens
		}
	}
}

func WithCompressor(compressor Compressor) Option {
	return func(opts *Options) {
		if compressor != nil {
			opts.Compressor = compressor
		}
	}
}

func WithParser(parser jsonx.Parser) Option {
	return func(opts *Options) {
		if parser != nil {
			opts.Parser = parser
		}
	}
}

func defaultCompressor() Compressor {
	return imagecompress.NewCompressor()
}

func defaultParser() jsonx.Parser {
	return jsonx.NewParser()
}
