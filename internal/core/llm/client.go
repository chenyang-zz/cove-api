package llm

import (
	"context"
)

type Client interface {
	Invoke(ctx context.Context, messages []*Message, opts ...ModelCallOption) (string, error)
	Stream(ctx context.Context, messages []*Message, opts ...ModelCallOption) (<-chan string, error)
	Embed(ctx context.Context, texts []string, dimensions int, opts ...EmbeddingOption) ([][]float64, error)
	EmbedOne(ctx context.Context, text string, dimensions int) ([]float64, error)
	// Vision(ctxt context.Context, prompt string) (string, error)
	// Rerank(ctx context.Context, query string, documents []string, top_n int) error
}
