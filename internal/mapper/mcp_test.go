package mapper_test

import (
	"testing"

	coremcp "github.com/boxify/api-go/internal/core/mcp"
	"github.com/boxify/api-go/internal/mapper"
	"github.com/boxify/api-go/internal/models"
	"github.com/google/uuid"
)

func TestMCPServerToResponseMapsToolsCacheWithoutNilPrefix(t *testing.T) {
	row := &models.MCPServer{
		ID:   uuid.New(),
		Name: "demo",
		ToolsCache: models.MCPMetas{
			{Name: "search", Description: "web search"},
		},
	}

	got := mapper.MCPServerToResponse(row, "masked-secret")

	if len(got.ToolsCache) != 1 {
		t.Fatalf("ToolsCache len = %d, want 1; value=%#v", len(got.ToolsCache), got.ToolsCache)
	}
	if got.ToolsCache[0] == nil {
		t.Fatal("ToolsCache[0] = nil, want meta")
	}
	if got.ToolsCache[0].Name != "search" || got.ToolsCache[0].Description != "web search" {
		t.Fatalf("ToolsCache[0] = %#v", got.ToolsCache[0])
	}
	if got.AuthMasked != "masked-secret" {
		t.Fatalf("AuthMasked = %q, want masked-secret", got.AuthMasked)
	}
}

func TestMCPServerToCoreServerConfigMapsConnectionFields(t *testing.T) {
	// 验证 mapper 将数据库模型转换为 core 配置，但不会传递数据库展示缓存。
	serverID := uuid.New()
	row := &models.MCPServer{
		ID:        serverID,
		Transport: "sse",
		Url:       "https://example.com/mcp",
		AuthType:  "bearer",
		ToolsCache: models.MCPMetas{
			nil,
			{Name: "search", Description: "web search"},
		},
	}

	got := mapper.MCPServerToCoreServerConfig(row, models.MCPAuthConfig{
		"token":       "plain-token",
		"placement":   "query",
		"query_param": "key",
	})

	if got.ID != serverID || got.Transport != "sse" || got.URL != "https://example.com/mcp" || got.AuthType != "bearer" {
		t.Fatalf("core config connection fields = %#v", got)
	}
	if got.AuthConfig["token"] != "plain-token" {
		t.Fatalf("AuthConfig[token] = %q, want plain-token", got.AuthConfig["token"])
	}
	if got.AuthConfig["placement"] != "query" || got.AuthConfig["query_param"] != "key" {
		t.Fatalf("AuthConfig = %#v, want query placement fields", got.AuthConfig)
	}
	if len(got.ToolsCache) != 0 {
		t.Fatalf("ToolsCache = %#v, want empty display cache", got.ToolsCache)
	}
}

func TestMCPToolMetasToModelMetasKeepsDisplayFieldsOnly(t *testing.T) {
	// 验证写入数据库展示缓存时只保留 name 和 description 元信息。
	got := mapper.MCPToolMetasToModelMetas([]coremcp.ToolMeta{
		{Name: "search", Description: "web search"},
	})

	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	if got[0].Name != "search" || got[0].Description != "web search" {
		t.Fatalf("meta = %#v", got[0])
	}
}
