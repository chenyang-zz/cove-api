package imagedescribe

import (
	"github.com/boxify/api-go/internal/core/jsonx"
	coreprompt "github.com/boxify/api-go/internal/core/prompt"
	"github.com/boxify/api-go/internal/core/rag/imagecompress"
	ragprompt "github.com/boxify/api-go/internal/core/rag/prompt"
)

const (
	defaultMaxTokens = int64(1024)
)

var defaultPrompt = coreprompt.MustRender(ragprompt.Templates, ragprompt.ImageDescriptionTemplate, nil)

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
