package classifier

import "context"

type TextClient interface {
	Classify(ctx context.Context, prompt string, temperature float64, maxTokens int64) (string, error)
}

type Input struct {
	Content      string
	ExistingTags []string
}

type Result struct {
	Tags []string
}
