/*
 * @Time   : 2026-07-10 12:05:36
 * @Author : chenyang
 * @File   : toolconfig.go
 */

package response

import (
	"time"

	domaintoolmcp "github.com/boxify/api-go/internal/domain/tools/mcp"
	"github.com/google/uuid"
)

type ToolConfigListResponse struct {
	BuiltinTools []*ToolConfigResponse   `json:"builtin_tools"`
	MCPServers   []*MCPToolGroupResponse `json:"mcp_servers"`
}

type MCPToolGroupResponse struct {
	ServerID       uuid.UUID                `json:"server_id"`
	ServerName     string                   `json:"server_name"`
	Enabled        bool                     `json:"enabled"`
	Status         string                   `json:"status"`
	LastError      *string                  `json:"last_error"`
	SyncedAt       *time.Time               `json:"synced_at"`
	CacheState     domaintoolmcp.CacheState `json:"cache_state"`
	CacheExpiresAt *time.Time               `json:"cache_expires_at"`
	CacheError     *string                  `json:"cache_error"`
	Tools          []*ToolConfigResponse    `json:"tools"`
}

type ToolConfigResponse struct {
	ToolKey     string `json:"tool_key"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Icon        string `json:"icon"`
	ToolType    string `json:"tool_type"`
	NeedsConfig bool   `json:"needs_config"`
	ConfigHit   string `json:"config_hit"`
	Enabled     bool   `json:"enabled"`
}
