package llm

import "testing"

func TestNewEmbeddingOptionsAppliesBatchSize(t *testing.T) {
	// 验证 embedding option 会记录正数批次大小，供具体 LLM client 分批请求向量接口。
	opts := NewEmbeddingOptions(WithEmbeddingBatchSize(10))

	if opts.BatchSize != 10 {
		t.Fatalf("EmbeddingOptions.BatchSize = %d, want 10", opts.BatchSize)
	}
}

func TestNewEmbeddingOptionsIgnoresInvalidBatchSize(t *testing.T) {
	// 验证无效批次大小不会覆盖已有配置，避免调用方传 0 时破坏默认行为。
	opts := NewEmbeddingOptions(WithEmbeddingBatchSize(10), WithEmbeddingBatchSize(0), WithEmbeddingBatchSize(-1))

	if opts.BatchSize != 10 {
		t.Fatalf("EmbeddingOptions.BatchSize = %d, want 10", opts.BatchSize)
	}
}
