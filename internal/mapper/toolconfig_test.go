package mapper

import (
	"testing"
	"time"

	coretool "github.com/boxify/api-go/internal/core/tool"
	domaintoolmcp "github.com/boxify/api-go/internal/domain/tools/mcp"
	"github.com/boxify/api-go/internal/models"
	"github.com/boxify/api-go/internal/transport/http/response"
	"github.com/google/uuid"
)

// TestBuiltinToolToResponseMapsDescriptorAnnotations 验证内置工具描述及展示注解会完整转换为响应字段。
func TestBuiltinToolToResponseMapsDescriptorAnnotations(t *testing.T) {
	out := BuiltinToolToResponse(coretool.Descriptor{
		Name:        "current_time",
		Description: "fallback description",
		Annotations: map[string]any{
			"display_name":        " 当前时间 ",
			"display_description": " 查询时间 ",
			"icon":                "clock",
			"needs_config":        true,
			"config_hint":         "timezone",
		},
	}, false)

	if out.ToolKey != "current_time" || out.Name != "当前时间" || out.Description != "查询时间" || out.Icon != "clock" {
		t.Fatalf("BuiltinToolToResponse display fields = %+v, want mapped annotations", out)
	}
	if out.ToolType != "builtin" || out.Enabled || !out.NeedsConfig || out.ConfigHit != "timezone" {
		t.Fatalf("BuiltinToolToResponse config fields = %+v, want builtin disabled configured response", out)
	}
}

// TestMCPToolAndGroupToResponseMapResolvedValues 验证 MCP 工具与 server 状态只按 logic 已解析的值构建分组响应。
func TestMCPToolAndGroupToResponseMapResolvedValues(t *testing.T) {
	serverID := uuid.New()
	syncedAt := time.Now().Add(-time.Minute)
	expiresAt := time.Now().Add(time.Minute)
	lastError := "previous"
	cacheError := "offline"
	server := &models.MCPServer{
		ID: serverID, Name: "搜索服务", Enabled: true, Status: "ready",
		LastError: &lastError, SyncedAt: &syncedAt,
	}
	tool := MCPToolToResponse(&domaintoolmcp.Definition{
		Key: "mcp_key", Name: "网页搜索", Description: "搜索网页",
	}, false)
	group := MCPToolGroupToResponse(server, []*response.ToolConfigResponse{tool}, domaintoolmcp.CacheStale, &expiresAt, &cacheError)

	if tool.ToolKey != "mcp_key" || tool.ToolType != "mcp" || tool.Enabled || tool.Icon != "🔌" {
		t.Fatalf("MCPToolToResponse = %+v, want disabled MCP tool", tool)
	}
	if group.ServerID != serverID || group.ServerName != "搜索服务" || group.CacheState != domaintoolmcp.CacheStale {
		t.Fatalf("MCPToolGroupToResponse = %+v, want mapped server and stale state", group)
	}
	if group.CacheExpiresAt != &expiresAt || group.CacheError != &cacheError || len(group.Tools) != 1 {
		t.Fatalf("MCPToolGroupToResponse cache/tools = %+v, want explicit values", group)
	}
}

// TestToolConfigListToResponseKeepsGroups 验证顶层工具配置响应保留内置工具和 MCP 分组。
func TestToolConfigListToResponseKeepsGroups(t *testing.T) {
	builtin := []*response.ToolConfigResponse{{ToolKey: "current_time"}}
	groups := []*response.MCPToolGroupResponse{{ServerID: uuid.New()}}
	out := ToolConfigListToResponse(builtin, groups)
	if len(out.BuiltinTools) != 1 || len(out.MCPServers) != 1 {
		t.Fatalf("ToolConfigListToResponse = %+v, want one builtin and one MCP group", out)
	}
}
