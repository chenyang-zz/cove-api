/**
 * @Time   : 2026/6/29 18:18
 * @Author : chenyangzhao542@gmail.com
 * @File   : mcp.go
 **/

package mapper

import (
	coremcp "github.com/boxify/api-go/internal/core/mcp"
	"github.com/boxify/api-go/internal/models"
	"github.com/boxify/api-go/internal/transport/http/request"
	"github.com/boxify/api-go/internal/transport/http/response"
)

func MCPServerToResponse(row *models.MCPServer, authMasked string) *response.MCPServerResponse {
	toolsCache := make([]*response.MCPMeta, 0, len(row.ToolsCache))
	for _, rowCache := range row.ToolsCache {
		toolsCache = append(toolsCache, &response.MCPMeta{
			Name:        rowCache.Name,
			Description: rowCache.Description,
		})
	}

	return &response.MCPServerResponse{
		ID:         row.ID,
		Name:       row.Name,
		Transport:  request.TransportType(row.Transport),
		Url:        row.Url,
		AuthType:   request.AuthType(row.AuthType),
		AuthMasked: authMasked,
		Enabled:    row.Enabled,
		Status:     row.Status,
		LastError:  row.LastError,
		ToolsCache: toolsCache,
		SyncedAt:   row.SyncedAt,
		CreatedAt:  row.CreatedAt,
		UpdatedAt:  row.UpdatedAt,
	}
}

func MCPServerToCoreServerConfig(row *models.MCPServer, authConfig models.MCPAuthConfig) coremcp.ServerConfig {
	return coremcp.ServerConfig{
		ID:         row.ID,
		Transport:  row.Transport,
		URL:        row.Url,
		AuthType:   row.AuthType,
		AuthConfig: map[string]string(authConfig),
		UpdatedAt:  row.UpdatedAt,
	}
}

func MCPToolMetasToModelMetas(metas []coremcp.ToolMeta) models.MCPMetas {
	out := make(models.MCPMetas, 0, len(metas))
	for _, meta := range metas {
		out = append(out, &models.MCPMeta{Name: meta.Name, Description: meta.Description})
	}
	return out
}
