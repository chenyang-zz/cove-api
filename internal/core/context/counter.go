package context

import (
	"encoding/json"

	"github.com/boxify/api-go/internal/core/llm"
	coretool "github.com/boxify/api-go/internal/core/tool"
	"github.com/pkoukk/tiktoken-go"
)

const defaultEncodingName = "cl100k_base"

// TiktokenCounter 使用 tiktoken 对模型消息和工具 schema 做统一计数。
type TiktokenCounter struct {
	encoding *tiktoken.Tiktoken
}

// NewTiktokenCounter 创建指定编码的 token 计数器。
//
// encodingName 为空时使用 cl100k_base；编码不存在时返回错误。
func NewTiktokenCounter(encodingName string) (*TiktokenCounter, error) {
	if encodingName == "" {
		encodingName = defaultEncodingName
	}
	encoding, err := tiktoken.GetEncoding(encodingName)
	if err != nil {
		return nil, err
	}
	return &TiktokenCounter{encoding: encoding}, nil
}

// CountMessages 返回消息序列化后占用的 token 数。
func (c *TiktokenCounter) CountMessages(messages []*llm.Message) int {
	return c.countJSON(messages)
}

// CountTools 返回完整工具描述和参数 schema 占用的 token 数。
func (c *TiktokenCounter) CountTools(tools []coretool.Descriptor) int {
	return c.countJSON(tools)
}

func (c *TiktokenCounter) countJSON(value any) int {
	if c == nil || c.encoding == nil {
		return 0
	}
	data, err := json.Marshal(value)
	if err != nil {
		return 0
	}
	return len(c.encoding.Encode(string(data), nil, nil))
}
