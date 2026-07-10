package toolconfig

import (
	"context"
	"errors"
	"slices"
	"testing"

	coremcp "github.com/boxify/api-go/internal/core/mcp"
	domaintoolmcp "github.com/boxify/api-go/internal/domain/tools/mcp"
	"github.com/boxify/api-go/internal/models"
	"github.com/boxify/api-go/internal/repository"
	"github.com/boxify/api-go/internal/svc"
	"github.com/boxify/api-go/internal/transport/http/request"
	"github.com/boxify/api-go/internal/xerr"
	"github.com/google/uuid"
)

// 验证工具配置列表来自内置 Catalog，且没有数据库配置时默认启用并忽略未知历史记录。
func TestListToolConfigsUsesBuiltinCatalogAndDefaultsEnabled(t *testing.T) {
	repo := &fakeToolConfigRepo{rows: []*models.ToolConfig{{ID: uuid.New(), ToolKey: "removed_tool", Enabled: false}}}
	logic := NewListToolConfigsLogic(context.Background(), newToolConfigTestServiceContext(repo))

	out, err := logic.ListToolConfigs(uuid.New())
	if err != nil {
		t.Fatalf("ListToolConfigs error = %v, want nil", err)
	}
	if len(out.BuiltinTools) != 2 {
		t.Fatalf("ListToolConfigs builtin len = %d, want 2", len(out.BuiltinTools))
	}
	keys := []string{out.BuiltinTools[0].ToolKey, out.BuiltinTools[1].ToolKey}
	if !slices.Equal(keys, []string{"current_time", "knowledge_search"}) {
		t.Fatalf("ListToolConfigs keys = %#v, want builtin catalog keys", keys)
	}
	for _, item := range out.BuiltinTools {
		if !item.Enabled || item.ToolType != builtinToolType || item.Name == "" || item.Description == "" || item.Icon == "" {
			t.Fatalf("ListToolConfigs item = %+v, want enabled builtin display metadata", item)
		}
	}
}

// 验证数据库中最新的用户配置会覆盖内置工具的默认启用状态。
func TestListToolConfigsAppliesLatestPersistedState(t *testing.T) {
	repo := &fakeToolConfigRepo{rows: []*models.ToolConfig{
		{ID: uuid.New(), ToolKey: "current_time", Enabled: false},
		{ID: uuid.New(), ToolKey: "current_time", Enabled: true},
	}}
	logic := NewListToolConfigsLogic(context.Background(), newToolConfigTestServiceContext(repo))

	out, err := logic.ListToolConfigs(uuid.New())
	if err != nil {
		t.Fatalf("ListToolConfigs error = %v, want nil", err)
	}
	if out.BuiltinTools[0].ToolKey != "current_time" || out.BuiltinTools[0].Enabled {
		t.Fatalf("current_time = %+v, want latest persisted disabled state", out.BuiltinTools[0])
	}
}

// TestListToolConfigsGroupsFreshMCPTools 验证启用的 MCP server 使用运行时缓存刷新，并按 server 分组返回默认启用工具。
func TestListToolConfigsGroupsFreshMCPTools(t *testing.T) {
	userID := uuid.New()
	server := &models.MCPServer{ID: uuid.New(), UserID: userID, Name: "搜索服务", Enabled: true, Status: "ready"}
	repo := &fakeToolConfigRepo{}
	svcCtx := newToolConfigTestServiceContext(repo)
	svcCtx.MCPServerRepo.(*fakeToolConfigMCPServerRepo).rows = []*models.MCPServer{server}
	client := &fakeToolConfigMCPClient{tools: []coremcp.ToolInfo{{Name: "search", Title: "远端搜索", Description: "查找网页"}}}
	svcCtx.MCPToolService = coremcp.NewService(coremcp.Options{Client: client})

	out, err := NewListToolConfigsLogic(context.Background(), svcCtx).ListToolConfigs(userID)
	if err != nil {
		t.Fatalf("ListToolConfigs error = %v, want nil", err)
	}
	if client.calls != 1 || len(out.MCPServers) != 1 {
		t.Fatalf("MCP calls/groups = %d/%d, want 1/1", client.calls, len(out.MCPServers))
	}
	group := out.MCPServers[0]
	if group.ServerID != server.ID || group.CacheState != domaintoolmcp.CacheFresh || group.CacheExpiresAt == nil || len(group.Tools) != 1 {
		t.Fatalf("MCP group = %+v, want fresh group with one tool", group)
	}
	if group.Tools[0].Name != "远端搜索" || !group.Tools[0].Enabled || group.Tools[0].ToolType != mcpToolType {
		t.Fatalf("MCP tool = %+v, want default-enabled runtime tool", group.Tools[0])
	}
}

// TestListToolConfigsFallsBackToStaleMCPSnapshot 验证 MCP 刷新失败时回退 PG 快照并标记 stale，而不影响整个列表。
func TestListToolConfigsFallsBackToStaleMCPSnapshot(t *testing.T) {
	userID := uuid.New()
	server := &models.MCPServer{
		ID: uuid.New(), UserID: userID, Name: "离线服务", Enabled: true,
		ToolsCache: models.MCPMetas{&models.MCPMeta{Name: "cached", Description: "缓存工具"}},
	}
	repo := &fakeToolConfigRepo{}
	svcCtx := newToolConfigTestServiceContext(repo)
	svcCtx.MCPServerRepo.(*fakeToolConfigMCPServerRepo).rows = []*models.MCPServer{server}
	svcCtx.MCPToolService = coremcp.NewService(coremcp.Options{Client: &fakeToolConfigMCPClient{err: errors.New("offline")}})

	out, err := NewListToolConfigsLogic(context.Background(), svcCtx).ListToolConfigs(userID)
	if err != nil {
		t.Fatalf("ListToolConfigs error = %v, want stale fallback", err)
	}
	group := out.MCPServers[0]
	if group.CacheState != domaintoolmcp.CacheStale || group.CacheError == nil || len(group.Tools) != 1 || group.Tools[0].Name != "cached" {
		t.Fatalf("MCP group = %+v, want stale PG snapshot", group)
	}
}

// TestListToolConfigsMarksEmptyWhenMCPRefreshAndSnapshotAreUnavailable 验证远端失败且没有 PG 快照时返回 empty 组。
func TestListToolConfigsMarksEmptyWhenMCPRefreshAndSnapshotAreUnavailable(t *testing.T) {
	userID := uuid.New()
	server := &models.MCPServer{ID: uuid.New(), UserID: userID, Name: "空服务", Enabled: true}
	svcCtx := newToolConfigTestServiceContext(&fakeToolConfigRepo{})
	svcCtx.MCPServerRepo.(*fakeToolConfigMCPServerRepo).rows = []*models.MCPServer{server}
	svcCtx.MCPToolService = coremcp.NewService(coremcp.Options{Client: &fakeToolConfigMCPClient{err: errors.New("offline")}})

	out, err := NewListToolConfigsLogic(context.Background(), svcCtx).ListToolConfigs(userID)
	if err != nil {
		t.Fatalf("ListToolConfigs error = %v, want empty fallback", err)
	}
	if out.MCPServers[0].CacheState != domaintoolmcp.CacheEmpty || len(out.MCPServers[0].Tools) != 0 {
		t.Fatalf("MCP group = %+v, want empty state", out.MCPServers[0])
	}
}

// TestListToolConfigsUsesDisabledMCPSnapshotWithoutRemoteCall 验证禁用 server 只展示 PG 快照且不会访问远端 MCP。
func TestListToolConfigsUsesDisabledMCPSnapshotWithoutRemoteCall(t *testing.T) {
	userID := uuid.New()
	server := &models.MCPServer{
		ID: uuid.New(), UserID: userID, Name: "禁用服务", Enabled: false,
		ToolsCache: models.MCPMetas{&models.MCPMeta{Name: "cached"}},
	}
	client := &fakeToolConfigMCPClient{tools: []coremcp.ToolInfo{{Name: "remote"}}}
	repo := &fakeToolConfigRepo{}
	svcCtx := newToolConfigTestServiceContext(repo)
	svcCtx.MCPServerRepo.(*fakeToolConfigMCPServerRepo).rows = []*models.MCPServer{server}
	svcCtx.MCPToolService = coremcp.NewService(coremcp.Options{Client: client})

	out, err := NewListToolConfigsLogic(context.Background(), svcCtx).ListToolConfigs(userID)
	if err != nil {
		t.Fatalf("ListToolConfigs error = %v, want nil", err)
	}
	if client.calls != 0 || out.MCPServers[0].CacheState != domaintoolmcp.CacheDisabled || len(out.MCPServers[0].Tools) != 1 {
		t.Fatalf("disabled group calls/state/tools = %d/%s/%d, want 0/disabled/1", client.calls, out.MCPServers[0].CacheState, len(out.MCPServers[0].Tools))
	}
}

// 验证关闭尚未持久化的内置工具会创建用户配置，并正确保留 false 值。
func TestToggleToolCreatesPersistedStateForBuiltinTool(t *testing.T) {
	repo := &fakeToolConfigRepo{}
	logic := NewToggleToolLogic(context.Background(), newToolConfigTestServiceContext(repo))
	enabled := false

	err := logic.ToggleTool(uuid.New(), &request.ToggleToolRequest{
		UriToolKeyRequest: request.UriToolKeyRequest{ToolKey: " current_time "},
		Enabled:           &enabled,
	})
	if err != nil {
		t.Fatalf("ToggleTool error = %v, want nil", err)
	}
	if repo.created == nil || repo.created.ToolKey != "current_time" || repo.created.ToolType != builtinToolType || repo.created.Enabled {
		t.Fatalf("created = %+v, want disabled builtin config", repo.created)
	}
	if repo.created.ID == uuid.Nil {
		t.Fatal("created ID = nil UUID, want generated ID")
	}
}

// 验证已有工具配置只更新 enabled 字段，不创建重复记录。
func TestToggleToolUpdatesOnlyEnabledForExistingConfig(t *testing.T) {
	configID := uuid.New()
	repo := &fakeToolConfigRepo{rows: []*models.ToolConfig{{ID: configID, ToolKey: "knowledge_search", Enabled: false}}}
	logic := NewToggleToolLogic(context.Background(), newToolConfigTestServiceContext(repo))
	enabled := true

	err := logic.ToggleTool(uuid.New(), &request.ToggleToolRequest{
		UriToolKeyRequest: request.UriToolKeyRequest{ToolKey: "knowledge_search"},
		Enabled:           &enabled,
	})
	if err != nil {
		t.Fatalf("ToggleTool error = %v, want nil", err)
	}
	if repo.created != nil || repo.updatedID != configID || repo.updated == nil || !repo.updated.Enabled {
		t.Fatalf("toggle calls created=%+v updatedID=%s updated=%+v", repo.created, repo.updatedID, repo.updated)
	}
	if !slices.Equal(repo.updatedFields.Columns(), []string{"enabled"}) {
		t.Fatalf("updated fields = %#v, want enabled only", repo.updatedFields.Columns())
	}
}

// 验证未知工具无法创建配置。
func TestToggleToolRejectsUnknownTool(t *testing.T) {
	repo := &fakeToolConfigRepo{}
	logic := NewToggleToolLogic(context.Background(), newToolConfigTestServiceContext(repo))
	enabled := true

	err := logic.ToggleTool(uuid.New(), &request.ToggleToolRequest{
		UriToolKeyRequest: request.UriToolKeyRequest{ToolKey: "missing"},
		Enabled:           &enabled,
	})
	if xerr.From(err).Kind != xerr.KindNotFound {
		t.Fatalf("ToggleTool error = %v, want not found", err)
	}
	if repo.created != nil {
		t.Fatalf("created = %+v, want nil", repo.created)
	}
}

// TestToggleToolCreatesMCPToolConfig 验证 MCP 工具可从禁用 server 的 PG 快照校验，并创建 tool_type=mcp 的关闭配置。
func TestToggleToolCreatesMCPToolConfig(t *testing.T) {
	userID := uuid.New()
	server := &models.MCPServer{
		ID: uuid.New(), UserID: userID, Name: "MCP", Enabled: false,
		ToolsCache: models.MCPMetas{&models.MCPMeta{Name: "search"}},
	}
	repo := &fakeToolConfigRepo{}
	svcCtx := newToolConfigTestServiceContext(repo)
	svcCtx.MCPServerRepo.(*fakeToolConfigMCPServerRepo).rows = []*models.MCPServer{server}
	enabled := false
	toolKey := domaintoolmcp.ToolKey(server.ID, "search")

	err := NewToggleToolLogic(context.Background(), svcCtx).ToggleTool(userID, &request.ToggleToolRequest{
		UriToolKeyRequest: request.UriToolKeyRequest{ToolKey: toolKey},
		Enabled:           &enabled,
	})
	if err != nil {
		t.Fatalf("ToggleTool error = %v, want nil", err)
	}
	if repo.created == nil || repo.created.ToolKey != toolKey || repo.created.ToolType != mcpToolType || repo.created.Enabled {
		t.Fatalf("created = %+v, want disabled MCP config", repo.created)
	}
}

type fakeToolConfigRepo struct {
	rows          []*models.ToolConfig
	listErr       error
	created       *models.ToolConfig
	updatedID     uuid.UUID
	updated       *models.ToolConfig
	updatedFields *repository.ToolConfigUpdateFields
}

func newToolConfigTestServiceContext(repo *fakeToolConfigRepo) *svc.ServiceContext {
	return &svc.ServiceContext{
		ToolConfigRepo: repo,
		MCPServerRepo:  &fakeToolConfigMCPServerRepo{},
		MCPToolService: coremcp.NewService(coremcp.Options{Client: &fakeToolConfigMCPClient{}}),
	}
}

type fakeToolConfigMCPClient struct {
	tools []coremcp.ToolInfo
	err   error
	calls int
}

func (c *fakeToolConfigMCPClient) ListTools(_ context.Context, _ coremcp.ServerConfig) ([]coremcp.ToolInfo, error) {
	c.calls++
	if c.err != nil {
		return nil, c.err
	}
	return append([]coremcp.ToolInfo(nil), c.tools...), nil
}

type fakeToolConfigMCPServerRepo struct {
	rows []*models.MCPServer
}

func (r *fakeToolConfigMCPServerRepo) Create(_ context.Context, userID uuid.UUID, row *models.MCPServer) (*models.MCPServer, error) {
	row.UserID = userID
	r.rows = append(r.rows, row)
	return row, nil
}

func (r *fakeToolConfigMCPServerRepo) List(_ context.Context, userID uuid.UUID) ([]*models.MCPServer, error) {
	out := make([]*models.MCPServer, 0, len(r.rows))
	for _, row := range r.rows {
		if row != nil && row.UserID == userID {
			out = append(out, row)
		}
	}
	return out, nil
}

func (r *fakeToolConfigMCPServerRepo) FindByID(_ context.Context, userID uuid.UUID, id uuid.UUID) (*models.MCPServer, error) {
	for _, row := range r.rows {
		if row != nil && row.UserID == userID && row.ID == id {
			return row, nil
		}
	}
	return nil, xerr.NotFound("MCP服务不存在")
}

func (r *fakeToolConfigMCPServerRepo) Update(_ context.Context, _ uuid.UUID, row *models.MCPServer) (*models.MCPServer, error) {
	return row, nil
}

func (r *fakeToolConfigMCPServerRepo) UpdateFields(_ context.Context, _ uuid.UUID, _ uuid.UUID, row *models.MCPServer, _ *repository.MCPServerUpdateFields) (*models.MCPServer, error) {
	return row, nil
}

func (r *fakeToolConfigMCPServerRepo) Delete(_ context.Context, _ uuid.UUID, _ uuid.UUID) error {
	return nil
}

func (r *fakeToolConfigMCPServerRepo) FindByName(_ context.Context, userID uuid.UUID, name string) (*models.MCPServer, error) {
	for _, row := range r.rows {
		if row != nil && row.UserID == userID && row.Name == name {
			return row, nil
		}
	}
	return nil, xerr.NotFound("MCP服务不存在")
}

func (r *fakeToolConfigRepo) Create(_ context.Context, userID uuid.UUID, row *models.ToolConfig) (*models.ToolConfig, error) {
	row.UserID = userID
	r.created = row
	r.rows = append([]*models.ToolConfig{row}, r.rows...)
	return row, nil
}

func (r *fakeToolConfigRepo) List(_ context.Context, _ uuid.UUID) ([]*models.ToolConfig, error) {
	if r.listErr != nil {
		return nil, r.listErr
	}
	return append([]*models.ToolConfig(nil), r.rows...), nil
}

func (r *fakeToolConfigRepo) FindByID(_ context.Context, _ uuid.UUID, id uuid.UUID) (*models.ToolConfig, error) {
	for _, row := range r.rows {
		if row != nil && row.ID == id {
			return row, nil
		}
	}
	return nil, xerr.NotFound("工具配置不存在")
}

func (r *fakeToolConfigRepo) Update(_ context.Context, _ uuid.UUID, row *models.ToolConfig) (*models.ToolConfig, error) {
	return row, nil
}

func (r *fakeToolConfigRepo) UpdateFields(_ context.Context, _ uuid.UUID, id uuid.UUID, row *models.ToolConfig, fields *repository.ToolConfigUpdateFields) (*models.ToolConfig, error) {
	r.updatedID = id
	r.updated = row
	r.updatedFields = fields
	return row, nil
}

func (r *fakeToolConfigRepo) Delete(_ context.Context, _ uuid.UUID, _ uuid.UUID) error {
	return errors.New("not implemented")
}
