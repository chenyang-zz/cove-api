package imagedescribe

import (
	"context"

	"github.com/boxify/api-go/internal/core/rag/imagecompress"
)

type VisionClient interface {
	Describe(ctx context.Context, prompt string, imageBase64 string, mime string, maxTokens int64) (string, error)
}

type Compressor interface {
	Compress(input imagecompress.Input) (*imagecompress.Output, error)
}

type Input struct {
	Data    []byte
	FileExt string
}

type Description struct {
	Description string   `json:"description"`
	OCRText     string   `json:"ocr_text"`
	Objects     []string `json:"objects"`
	Scene       string   `json:"scene"`
}
