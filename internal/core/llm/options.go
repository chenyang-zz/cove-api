package llm

type ModelCallOptions struct {
	Temperature *float64
	MaxTokens   *int64
}

// EmbeddingOptions 表示一次批量向量化调用的可选参数。
type EmbeddingOptions struct {
	BatchSize int
}

type ModelCallOption func(*ModelCallOptions)

// EmbeddingOption 修改一次批量向量化调用的可选参数。
type EmbeddingOption func(*EmbeddingOptions)

func NewChatOptions(opts ...ModelCallOption) ModelCallOptions {
	var out ModelCallOptions
	for _, opt := range opts {
		if opt != nil {
			opt(&out)
		}
	}
	return out
}

func WithTemperature(value float64) ModelCallOption {
	return func(opts *ModelCallOptions) {
		opts.Temperature = &value
	}
}

// NewEmbeddingOptions 返回合并后的批量向量化调用参数。
func NewEmbeddingOptions(opts ...EmbeddingOption) EmbeddingOptions {
	var out EmbeddingOptions
	for _, opt := range opts {
		if opt != nil {
			opt(&out)
		}
	}
	return out
}

func WithMaxTokens(value int64) ModelCallOption {
	return func(opts *ModelCallOptions) {
		opts.MaxTokens = &value
	}
}

// WithEmbeddingBatchSize 设置批量向量化请求的单批文本数量，非正数会被忽略。
func WithEmbeddingBatchSize(batchSize int) EmbeddingOption {
	return func(opts *EmbeddingOptions) {
		if batchSize > 0 {
			opts.BatchSize = batchSize
		}
	}
}
