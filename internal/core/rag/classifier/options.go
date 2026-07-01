package classifier

import (
	"github.com/boxify/api-go/internal/core/jsonx"
	coreprompt "github.com/boxify/api-go/internal/core/prompt"
	ragprompt "github.com/boxify/api-go/internal/core/rag/prompt"
)

const (
	defaultTemperature  = 0.2
	defaultMaxTokens    = int64(200)
	defaultSnippetRunes = 1500
)

var defaultPrompt = coreprompt.MustTemplateText(ragprompt.Templates, ragprompt.ContentClassifierTemplate)

type Options struct {
	Prompt       string
	Temperature  float64
	MaxTokens    int64
	SnippetRunes int
	Parser       jsonx.Parser
	promptTmpl   bool
}

type Option func(*Options)

func WithPrompt(prompt string) Option {
	return func(opts *Options) {
		if prompt != "" {
			opts.Prompt = prompt
			opts.promptTmpl = false
		}
	}
}

func WithTemperature(temperature float64) Option {
	return func(opts *Options) {
		if temperature >= 0 {
			opts.Temperature = temperature
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

func WithSnippetRunes(snippetRunes int) Option {
	return func(opts *Options) {
		if snippetRunes > 0 {
			opts.SnippetRunes = snippetRunes
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

func defaultParser() jsonx.Parser {
	return jsonx.NewParser()
}
