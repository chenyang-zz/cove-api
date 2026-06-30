package mcp

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/google/uuid"
)

const (
	DefaultTTL = 5 * time.Minute

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
