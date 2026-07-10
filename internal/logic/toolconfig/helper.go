package toolconfig

import (
	"context"
	"log/slog"
	"sort"
	"sync"
	"time"

	coremcp "github.com/boxify/api-go/internal/core/mcp"
	coretool "github.com/boxify/api-go/internal/core/tool"
	domaintools "github.com/boxify/api-go/internal/domain/tools"
	domaintoolmcp "github.com/boxify/api-go/internal/domain/tools/mcp"
	"github.com/boxify/api-go/internal/mapper"
	"github.com/boxify/api-go/internal/models"
	"github.com/boxify/api-go/internal/svc"
	"github.com/boxify/api-go/internal/transport/http/response"
)

const (
	builtinToolType          = "builtin"
	mcpToolType              = "mcp"
	mcpRefreshConcurrencyMax = 4
)

// builtinToolResponses 返回内置工具的配置响应列表
func builtinToolResponses(ctx context.Context, svcCtx *svc.ServiceContext) ([]*response.ToolConfigResponse, error) {
	catalog, err := domaintools.NewCatalog(svcCtx)
	if err != nil {
		return nil, err
	}
	registry, err := catalog.BuildRegistry(ctx, coretool.Selection{})
	if err != nil {
		return nil, err
	}
	descriptors := registry.List(nil)
	items := make([]*response.ToolConfigResponse, 0, len(descriptors))
	for _, descriptor := range descriptors {
		items = append(items, mapper.BuiltinToolToResponse(descriptor, true))
	}
	return items, nil
}

// mcpToolGroups 返回 MCP 工具组的响应列表
func mcpToolGroups(
	ctx context.Context,
	svcCtx *svc.ServiceContext,
	servers []*models.MCPServer,
	enabledByKey map[string]bool,
	log *slog.Logger,
) []*response.MCPToolGroupResponse {
	groups := make([]*response.MCPToolGroupResponse, len(servers))
	sem := make(chan struct{}, mcpRefreshConcurrencyMax)
	var wg sync.WaitGroup
	for index, server := range servers {
		index, server := index, server
		if server == nil {
			continue
		}

		// 如果 MCP 服务未启用，则直接返回数据库快照
		if !server.Enabled {
			groups[index] = mcpSnapshotGroup(server, enabledByKey, domaintoolmcp.CacheDisabled, nil)
			continue
		}
		wg.Go(func() {
			select {
			case <-ctx.Done():
				cacheErr := ctx.Err().Error()
				groups[index] = mcpSnapshotGroup(server, enabledByKey, fallbackCacheState(server), &cacheErr)
				return
			case sem <- struct{}{}:
			}
			defer func() { <-sem }()
			groups[index] = loadMCPToolGroup(ctx, svcCtx, server, enabledByKey, log)
		})
	}
	wg.Wait()

	filtered := groups[:0]
	for _, group := range groups {
		if group != nil {
			filtered = append(filtered, group)
		}
	}
	sort.SliceStable(filtered, func(i, j int) bool {
		if filtered[i].ServerName == filtered[j].ServerName {
			return filtered[i].ServerID.String() < filtered[j].ServerID.String()
		}
		return filtered[i].ServerName < filtered[j].ServerName
	})
	return filtered
}

// loadMCPToolGroup 尝试从 MCP 服务加载工具组，如果失败则回退到数据库快照
func loadMCPToolGroup(
	ctx context.Context,
	svcCtx *svc.ServiceContext,
	server *models.MCPServer,
	enabledByKey map[string]bool,
	log *slog.Logger,
) *response.MCPToolGroupResponse {
	definitions, status, err := liveMCPDefinitions(ctx, svcCtx, server)
	if err != nil {
		cacheErr := err.Error()
		if log != nil {
			log.WarnContext(ctx, "刷新MCP工具缓存失败，回退数据库快照",
				slog.String("server_id", server.ID.String()),
				slog.String("error", cacheErr),
			)
		}
		return mcpSnapshotGroup(server, enabledByKey, fallbackCacheState(server), &cacheErr)
	}
	var expiresAt *time.Time
	if status.Valid {
		value := status.ExpiresAt
		expiresAt = &value
	}
	return mcpGroupFromDefinitions(server, definitions, enabledByKey, domaintoolmcp.CacheFresh, expiresAt, nil)
}

// liveMCPDefinitions 尝试从 MCP 服务获取工具定义列表，如果失败则返回错误
func liveMCPDefinitions(ctx context.Context, svcCtx *svc.ServiceContext, server *models.MCPServer) ([]*domaintoolmcp.Definition, coremcp.CacheStatus, error) {
	serverConfig, err := domaintoolmcp.ServerConfig(server, svcCtx.SecretCipher)
	if err != nil {
		return nil, coremcp.CacheStatus{}, err
	}
	tools, err := svcCtx.MCPToolService.BuildToolList(ctx, serverConfig)
	if err != nil {
		return nil, coremcp.CacheStatus{}, err
	}
	status, err := svcCtx.MCPToolService.CacheStatus(ctx, serverConfig)
	if err != nil {
		return nil, coremcp.CacheStatus{}, err
	}
	return domaintoolmcp.Definitions(server, tools), status, nil
}

// availableMCPDefinitions 返回可用的 MCP 工具定义列表，如果 MCP 服务不可用则回退到数据库快照
func availableMCPDefinitions(ctx context.Context, svcCtx *svc.ServiceContext, server *models.MCPServer) ([]*domaintoolmcp.Definition, error) {
	if !server.Enabled {
		return domaintoolmcp.SnapshotDefinitions(server), nil
	}
	definitions, _, err := liveMCPDefinitions(ctx, svcCtx, server)
	if err == nil {
		return definitions, nil
	}
	snapshot := domaintoolmcp.SnapshotDefinitions(server)
	if len(snapshot) > 0 {
		return snapshot, nil
	}
	return nil, err
}

// mcpSnapshotGroup 返回 MCP 工具组的快照响应
func mcpSnapshotGroup(server *models.MCPServer, enabledByKey map[string]bool, state domaintoolmcp.CacheState, cacheErr *string) *response.MCPToolGroupResponse {
	return mcpGroupFromDefinitions(server, domaintoolmcp.SnapshotDefinitions(server), enabledByKey, state, nil, cacheErr)
}

// fallbackCacheState 返回 MCP 工具组的缓存状态，如果没有缓存则返回 CacheEmpty
func fallbackCacheState(server *models.MCPServer) domaintoolmcp.CacheState {
	if server != nil && len(server.ToolsCache) > 0 {
		return domaintoolmcp.CacheStale
	}
	return domaintoolmcp.CacheEmpty
}

// mcpGroupFromDefinitions 根据工具定义列表生成 MCP 工具组响应
func mcpGroupFromDefinitions(
	server *models.MCPServer,
	definitions []*domaintoolmcp.Definition,
	enabledByKey map[string]bool,
	cacheState domaintoolmcp.CacheState,
	cacheExpiresAt *time.Time,
	cacheErr *string,
) *response.MCPToolGroupResponse {
	tools := make([]*response.ToolConfigResponse, 0, len(definitions))
	for _, definition := range definitions {
		if definition == nil {
			continue
		}
		enabled := true
		if configured, ok := enabledByKey[definition.Key]; ok {
			enabled = configured
		}
		tools = append(tools, mapper.MCPToolToResponse(definition, enabled))
	}
	sort.SliceStable(tools, func(i, j int) bool {
		if tools[i].Name == tools[j].Name {
			return tools[i].ToolKey < tools[j].ToolKey
		}
		return tools[i].Name < tools[j].Name
	})
	return mapper.MCPToolGroupToResponse(server, tools, cacheState, cacheExpiresAt, cacheErr)
}

// toolEnabledMap 根据工具配置列表生成工具启用状态映射，重复配置只采用最新一条
func toolEnabledMap(rows []*models.ToolConfig) map[string]bool {
	enabledByKey := make(map[string]bool, len(rows))
	// 仓储按更新时间倒序返回；重复配置只采用最新一条。
	for _, row := range rows {
		if row == nil {
			continue
		}
		if _, exists := enabledByKey[row.ToolKey]; !exists {
			enabledByKey[row.ToolKey] = row.Enabled
		}
	}
	return enabledByKey
}
