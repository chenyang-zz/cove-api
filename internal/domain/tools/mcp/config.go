package mcp

import (
	"maps"

	coremcp "github.com/boxify/api-go/internal/core/mcp"
	"github.com/boxify/api-go/internal/models"
	"github.com/boxify/api-go/internal/xerr"
)

// Decrypter 定义 MCP 认证字段解密所需的最小依赖。
type Decrypter interface {
	Decrypt(string) (string, error)
}

// ServerConfig 将数据库 MCP server 转换为包含明文认证信息的 core 配置。
//
// token 或 key 解密失败时返回内部错误；非敏感认证字段会原样保留。nil row 返回错误。
func ServerConfig(row *models.MCPServer, decrypter Decrypter) (coremcp.ServerConfig, error) {
	if row == nil {
		return coremcp.ServerConfig{}, xerr.Internal("MCP服务配置不存在", nil)
	}
	authConfig := make(map[string]string, len(row.AuthConfig))
	maps.Copy(authConfig, row.AuthConfig)
	for _, key := range []string{"token", "key"} {
		if authConfig[key] == "" {
			continue
		}
		if decrypter == nil {
			return coremcp.ServerConfig{}, xerr.Internal("MCP认证配置解密器未初始化", nil)
		}
		plain, err := decrypter.Decrypt(authConfig[key])
		if err != nil {
			return coremcp.ServerConfig{}, xerr.Internal("MCP认证配置解密失败", err)
		}
		authConfig[key] = plain
	}
	toolsCache := make([]coremcp.ToolMeta, 0, len(row.ToolsCache))
	for _, item := range row.ToolsCache {
		if item != nil {
			toolsCache = append(toolsCache, coremcp.ToolMeta{Name: item.Name, Description: item.Description})
		}
	}
	return coremcp.ServerConfig{
		ID:         row.ID,
		Transport:  row.Transport,
		URL:        row.Url,
		AuthType:   row.AuthType,
		AuthConfig: authConfig,
		UpdatedAt:  row.UpdatedAt,
		ToolsCache: toolsCache,
		SyncedAt:   row.SyncedAt,
	}, nil
}
