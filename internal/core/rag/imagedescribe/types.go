package imagedescribe

import (
	"context"

	corellm "github.com/boxify/api-go/internal/core/llm"
	"github.com/boxify/api-go/internal/core/rag/imagecompress"
)

// Compressor 定义图片描述前的压缩能力。
type Compressor interface {
	Compress(input imagecompress.Input) (*imagecompress.Output, error)
}

// Input 表示一次图片描述请求。
//
// Data 是原始图片字节，FileExt 用于压缩阶段推断 MIME。
type Input struct {
	Data    []byte
	FileExt string
}

// Description 是看图结构化描述的业务别名，契约与 corellm.VisionDescription 对齐。
type Description = corellm.VisionDescription

// DescriberAPI 表示可被 worker 等调用方注入的描述能力。
//
// *Describer 实现该接口；测试可替换为本地 fake。
type DescriberAPI interface {
	Describe(ctx context.Context, input Input) (*Description, error)
}
