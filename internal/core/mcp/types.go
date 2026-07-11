package mcp

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/google/uuid"
)

const (
	// DefaultTTL 是运行时工具列表缓存的默认有效期。
	DefaultTTL = 5 * time.Minute
	// DefaultDiscoverTimeout 是 Connect + ListTools 发现路径的默认超时。
	// 对话组装等同步路径不应依赖操作系统级 ~60s 连接超时。
	DefaultDiscoverTimeout = 5 * time.Second
	// DefaultFailCooldown 是发现失败后跳过远端探测的默认冷却窗口。
	DefaultFailCooldown = 30 * time.Second
	// DefaultDialTimeout 是 MCP HTTP 传输层 TCP/TLS 建连超时。
	DefaultDialTimeout = 3 * time.Second

	TransportSSE            = "sse"
	TransportStreamableHTTP = "streamable_http"

	AuthNone   = "none"
	AuthBearer = "bearer"
	AuthAPIKey = "api_key"
)

type ToolMeta struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type ToolInfo struct {
	Name         string `json:"name"`
	Description  string `json:"description"`
	Title        string `json:"title,omitempty"`
	InputSchema  any    `json:"input_schema,omitempty"`
	OutputSchema any    `json:"output_schema,omitempty"`
	Annotations  any    `json:"annotations,omitempty"`
	Icons        any    `json:"icons,omitempty"`
}

// Content 表示 MCP 工具调用返回的一段标准化内容。
//
// Raw 保留 MCP 原始 JSON，便于调用方处理 resource 等扩展内容；Data 保存已解码的
// image/audio 二进制数据。未知内容类型仍会通过 Raw 原样保留。
type Content struct {
	Type     string          `json:"type"`
	Text     string          `json:"text,omitempty"`
	Data     []byte          `json:"data,omitempty"`
	MIMEType string          `json:"mime_type,omitempty"`
	URI      string          `json:"uri,omitempty"`
	Raw      json.RawMessage `json:"raw,omitempty"`
}

// CallResult 表示一次 MCP 工具调用的标准化结果。
//
// IsError 为 true 表示远端工具返回了可供模型观察和纠正的业务错误，不等同于
// transport 或 protocol error。StructuredContent 会保持远端返回的原始结构。
type CallResult struct {
	Content           []Content `json:"content"`
	StructuredContent any       `json:"structured_content,omitempty"`
	IsError           bool      `json:"is_error,omitempty"`
}

type ServerConfig struct {
	ID         uuid.UUID
	Transport  string
	URL        string
	AuthType   string
	AuthConfig map[string]string
	UpdatedAt  time.Time
	ToolsCache []ToolMeta
	SyncedAt   *time.Time
}

type CacheEntry struct {
	Fingerprint string
	ExpiresAt   time.Time
	Tools       []ToolInfo
}

type CacheStatus struct {
	Valid       bool
	Source      string
	Fingerprint string
	ExpiresAt   time.Time
}

// contentFromJSON 解析 MCP 工具调用返回的 JSON 内容，支持标准化字段和 resource 扩展字段。
func contentFromJSON(data []byte) (Content, error) {
	var wire struct {
		Type     string `json:"type"`
		Text     string `json:"text"`
		Data     string `json:"data"`
		MIMEType string `json:"mimeType"`
		URI      string `json:"uri"`
		Resource *struct {
			URI string `json:"uri"`
		} `json:"resource"`
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		return Content{}, err
	}
	content := Content{
		Type:     wire.Type,
		Text:     wire.Text,
		MIMEType: wire.MIMEType,
		URI:      wire.URI,
		Raw:      append(json.RawMessage(nil), data...),
	}
	if content.URI == "" && wire.Resource != nil {
		content.URI = wire.Resource.URI
	}
	if wire.Data != "" {
		decoded, err := base64.StdEncoding.DecodeString(wire.Data)
		if err != nil {
			return Content{}, err
		}
		content.Data = decoded
	}
	return content, nil
}

func Fingerprint(server ServerConfig) string {
	payload := struct {
		ID         string            `json:"id"`
		Transport  string            `json:"transport"`
		URL        string            `json:"url"`
		AuthType   string            `json:"auth_type"`
		AuthConfig map[string]string `json:"auth_config"`
	}{
		ID:         server.ID.String(),
		Transport:  server.Transport,
		URL:        server.URL,
		AuthType:   server.AuthType,
		AuthConfig: orderedAuthConfig(server.AuthConfig),
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Sprintf("%s:%s:%s:%s", server.ID.String(), server.Transport, server.URL, server.AuthType)
	}
	sum := sha256.Sum256(data)
	return fmt.Sprintf("%x", sum[:])
}

func MetasFromTools(tools []ToolInfo) []ToolMeta {
	if len(tools) == 0 {
		return nil
	}
	out := make([]ToolMeta, 0, len(tools))
	for _, tool := range tools {
		out = append(out, ToolMeta{Name: tool.Name, Description: tool.Description})
	}
	return out
}

func cloneTools(in []ToolInfo) []ToolInfo {
	if len(in) == 0 {
		return nil
	}
	out := make([]ToolInfo, len(in))
	copy(out, in)
	return out
}

func orderedAuthConfig(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	keys := make([]string, 0, len(in))
	for key := range in {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make(map[string]string, len(in))
	for _, key := range keys {
		out[key] = in[key]
	}
	return out
}
