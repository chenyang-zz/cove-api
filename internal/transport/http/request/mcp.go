/**
 * @Time   : 2026/6/29 16:34
 * @Author : chenyangzhao542@gmail.com
 * @File   : mcp.go
 **/

package request

type TransportType string

const (
	SSETransport   TransportType = "sse"
	StreamableHTTP TransportType = "streamable_http"
)

type AuthType string

const (
	None   AuthType = "none"
	Bearer AuthType = "bearer"
	ApiKey AuthType = "api_key"
)

type MCPAuthConfig struct {
	Token      string `json:"token" binding:"omitempty"`
	Header     string `json:"header" binding:"omitempty"`
	Key        string `json:"key" binding:"omitempty"`
	Placement  string `json:"placement" binding:"omitempty,oneof=header query"`
	QueryParam string `json:"query_param" binding:"omitempty"`
}

type CreateMCPServerRequest struct {
	Name       string         `json:"name" binding:"required,min=1,max=128"`
	Transport  TransportType  `json:"transport" binding:"required,oneof=sse streamable_http"`
	Url        string         `json:"url" binding:"required,url"`
	AuthType   AuthType       `json:"auth_type" binding:"required,oneof=none bearer api_key"`
	AuthConfig *MCPAuthConfig `json:"auth_config" binding:"omitempty"`
	Enabled    *bool          `json:"enabled" binding:"omitempty"`
}

type UriMCPServerIDRequest struct {
	ID string `uri:"mcp_id" binding:"required"`
}

type UpdateMCPServerRequest struct {
	UriMCPServerIDRequest
	Name       *string        `json:"name" binding:"omitempty,min=1,max=128"`
	Transport  *TransportType `json:"transport" binding:"omitempty,oneof=sse streamable_http"`
	Url        *string        `json:"url" binding:"omitempty,url"`
	AuthType   *AuthType      `json:"auth_type" binding:"omitempty,oneof=none bearer api_key"`
	AuthConfig *MCPAuthConfig `json:"auth_config" binding:"omitempty"`
	Enabled    *bool          `json:"enabled" binding:"omitempty"`
}

type ToggleMCPServerRequest struct {
	UriMCPServerIDRequest
	Enabled *bool `json:"enabled" binding:"required"`
}
