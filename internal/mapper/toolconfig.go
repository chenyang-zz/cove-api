package mapper

import (
	"strings"
	"time"

	coretool "github.com/boxify/api-go/internal/core/tool"
	domaintoolmcp "github.com/boxify/api-go/internal/domain/tools/mcp"
	"github.com/boxify/api-go/internal/models"
	"github.com/boxify/api-go/internal/transport/http/response"
)

const (
	builtinToolType = "builtin"
	mcpToolType     = "mcp"
)

// BuiltinToolToResponse 将内置工具描述转换为工具配置响应。
func BuiltinToolToResponse(descriptor coretool.Descriptor, enabled bool) *response.ToolConfigResponse {
	return &response.ToolConfigResponse{
		ToolKey:     descriptor.Name,
		Name:        annotationString(descriptor.Annotations, "display_name", descriptor.Name),
		Description: annotationString(descriptor.Annotations, "display_description", descriptor.Description),
		Icon:        annotationString(descriptor.Annotations, "icon", ""),
		ToolType:    builtinToolType,
		NeedsConfig: annotationBool(descriptor.Annotations, "needs_config"),
		ConfigHit:   annotationString(descriptor.Annotations, "config_hint", ""),
		Enabled:     enabled,
	}
}

// MCPToolToResponse 将 MCP 工具定义和已解析的启用状态转换为工具配置响应。
func MCPToolToResponse(definition *domaintoolmcp.Definition, enabled bool) *response.ToolConfigResponse {
	if definition == nil {
		return nil
	}
	return &response.ToolConfigResponse{
		ToolKey:     definition.Key,
		Name:        definition.Name,
		Description: definition.Description,
		Icon:        "🔌",
		ToolType:    mcpToolType,
		Enabled:     enabled,
	}
}

// MCPToolGroupToResponse 将 MCP server、工具列表和已计算的缓存信息转换为分组响应。
func MCPToolGroupToResponse(
	server *models.MCPServer,
	tools []*response.ToolConfigResponse,
	cacheState domaintoolmcp.CacheState,
	cacheExpiresAt *time.Time,
	cacheErr *string,
) *response.MCPToolGroupResponse {
	if server == nil {
		return nil
	}
	return &response.MCPToolGroupResponse{
		ServerID:       server.ID,
		ServerName:     server.Name,
		Enabled:        server.Enabled,
		Status:         server.Status,
		LastError:      server.LastError,
		SyncedAt:       server.SyncedAt,
		CacheState:     cacheState,
		CacheExpiresAt: cacheExpiresAt,
		CacheError:     cacheErr,
		Tools:          tools,
	}
}

// ToolConfigListToResponse 将内置工具与 MCP 分组转换为工具配置列表响应。
func ToolConfigListToResponse(
	builtinTools []*response.ToolConfigResponse,
	mcpServers []*response.MCPToolGroupResponse,
) *response.ToolConfigListResponse {
	return &response.ToolConfigListResponse{
		BuiltinTools: builtinTools,
		MCPServers:   mcpServers,
	}
}

func annotationString(annotations map[string]any, key string, fallback string) string {
	value, ok := annotations[key].(string)
	if !ok || strings.TrimSpace(value) == "" {
		return fallback
	}
	return strings.TrimSpace(value)
}

func annotationBool(annotations map[string]any, key string) bool {
	value, _ := annotations[key].(bool)
	return value
}
