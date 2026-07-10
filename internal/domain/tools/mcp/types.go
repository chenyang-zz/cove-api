package mcp

import (
	coremcp "github.com/boxify/api-go/internal/core/mcp"
	"github.com/google/uuid"
)

// CacheState 表示配置页中 MCP 工具组的缓存状态。
type CacheState string

const (
	// CacheFresh 表示工具定义来自有效运行时缓存或本次成功刷新。
	CacheFresh CacheState = "fresh"
	// CacheStale 表示远端刷新失败，当前展示 PG 快照。
	CacheStale CacheState = "stale"
	// CacheDisabled 表示 server 已禁用，当前仅展示 PG 快照。
	CacheDisabled CacheState = "disabled"
	// CacheEmpty 表示没有可展示的运行时或 PG 工具定义。
	CacheEmpty CacheState = "empty"
)

// Definition 描述一个归属于 MCP server 的稳定工具定义。
type Definition struct {
	Key         string
	RawName     string
	Name        string
	Description string
	ServerID    uuid.UUID
	ServerName  string
	Info        coremcp.ToolInfo
}
