package mcp

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
)

type fakeToolClient struct {
	calls int
	tools []ToolInfo
	err   error
}

func (c *fakeToolClient) ListTools(ctx context.Context, server ServerConfig) ([]ToolInfo, error) {
	c.calls++
	if c.err != nil {
		return nil, c.err
	}
	return c.tools, nil
}

type fakeToolCache struct {
	entry CacheEntry
	found bool
	set   *CacheEntry
}

func (c *fakeToolCache) Get(ctx context.Context, key string) (CacheEntry, bool, error) {
	return c.entry, c.found, nil
}

func (c *fakeToolCache) Set(ctx context.Context, key string, entry CacheEntry) error {
	c.set = &entry
	return nil
}

func (c *fakeToolCache) Valid(server ServerConfig, entry CacheEntry) bool {
	return entry.Fingerprint == Fingerprint(server) && time.Now().Before(entry.ExpiresAt)
}

func TestServiceBuildToolListUsesValidRuntimeCache(t *testing.T) {
	// 验证运行期缓存命中时直接返回完整工具信息，不访问远端 MCP 服务。
	server := ServerConfig{ID: uuid.New(), UpdatedAt: time.Now()}
	want := []ToolInfo{{
		Name:         "search",
		Description:  "web search",
		Title:        "Search",
		InputSchema:  map[string]any{"type": "object"},
		OutputSchema: map[string]any{"type": "string"},
	}}
	client := &fakeToolClient{tools: []ToolInfo{{Name: "remote"}}}
	cache := &fakeToolCache{
		found: true,
		entry: CacheEntry{
			Fingerprint: Fingerprint(server),
			ExpiresAt:   time.Now().Add(time.Minute),
			Tools:       want,
		},
	}

	got, err := NewService(Options{Client: client, Cache: cache}).BuildToolList(context.Background(), server)
	if err != nil {
		t.Fatalf("BuildToolList error = %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("tools = %#v, want %#v", got, want)
	}
	if client.calls != 0 {
		t.Fatalf("client calls = %d, want 0", client.calls)
	}
}

func TestServiceBuildToolListRefreshesWhenFingerprintChanges(t *testing.T) {
	// 验证 updated_at 变化导致指纹失效，并重新拉取远端工具清单。
	server := ServerConfig{ID: uuid.New(), UpdatedAt: time.Now()}
	want := []ToolInfo{{Name: "fresh", Description: "fresh desc", InputSchema: map[string]any{"type": "object"}}}
	client := &fakeToolClient{tools: want}
	cache := &fakeToolCache{
		found: true,
		entry: CacheEntry{
			Fingerprint: "old-fingerprint",
			ExpiresAt:   time.Now().Add(time.Minute),
			Tools:       []ToolInfo{{Name: "stale"}},
		},
	}

	got, err := NewService(Options{Client: client, Cache: cache}).BuildToolList(context.Background(), server)
	if err != nil {
		t.Fatalf("BuildToolList error = %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("tools = %#v, want %#v", got, want)
	}
	if client.calls != 1 {
		t.Fatalf("client calls = %d, want 1", client.calls)
	}
	if cache.set == nil || cache.set.Fingerprint != Fingerprint(server) {
		t.Fatalf("cache set = %#v, want refreshed fingerprint", cache.set)
	}
}

func TestServiceBuildToolListIgnoresDatabaseDisplayToolsCache(t *testing.T) {
	// 验证数据库 tools_cache 只是展示数据，运行期缓存未命中时必须访问远端 MCP 服务。
	now := time.Now()
	syncedAt := now
	server := ServerConfig{
		ID:         uuid.New(),
		UpdatedAt:  now,
		ToolsCache: []ToolMeta{{Name: "db-tool", Description: "from db"}},
		SyncedAt:   &syncedAt,
	}
	remote := []ToolInfo{{Name: "remote", Description: "remote desc", InputSchema: map[string]any{"type": "object"}}}
	client := &fakeToolClient{tools: remote}
	service := NewService(Options{Client: client, TTL: 5 * time.Minute, Now: func() time.Time { return now }})

	got, err := service.BuildToolList(context.Background(), server)
	if err != nil {
		t.Fatalf("BuildToolList error = %v", err)
	}
	if !reflect.DeepEqual(got, remote) {
		t.Fatalf("tools = %#v, want remote tools %#v", got, remote)
	}
	if client.calls != 1 {
		t.Fatalf("client calls = %d, want 1", client.calls)
	}
	status, err := service.CacheStatus(context.Background(), server)
	if err != nil {
		t.Fatalf("CacheStatus error = %v", err)
	}
	if !status.Valid || status.Source != "runtime" {
		t.Fatalf("cache status = %#v, want runtime source after remote fetch", status)
	}
}

func TestServiceBuildToolListRefreshesWhenTTLExpires(t *testing.T) {
	// 验证 TTL 过期后不会继续使用旧缓存，会重新访问远端 MCP 服务。
	syncedAt := time.Now().Add(-10 * time.Minute)
	server := ServerConfig{
		ID:         uuid.New(),
		UpdatedAt:  time.Now(),
		ToolsCache: []ToolMeta{{Name: "db-tool"}},
		SyncedAt:   &syncedAt,
	}
	client := &fakeToolClient{tools: []ToolInfo{{Name: "remote-tool", InputSchema: map[string]any{"type": "object"}}}}

	got, err := NewService(Options{Client: client, TTL: time.Minute}).BuildToolList(context.Background(), server)
	if err != nil {
		t.Fatalf("BuildToolList error = %v", err)
	}
	if len(got) != 1 || got[0].Name != "remote-tool" {
		t.Fatalf("tools = %#v, want remote tools", got)
	}
	if client.calls != 1 {
		t.Fatalf("client calls = %d, want 1", client.calls)
	}
}

func TestServiceBuildToolListReturnsRemoteError(t *testing.T) {
	// 验证没有有效缓存时，远端 MCP 错误会原样返回给业务层处理。
	wantErr := errors.New("remote unavailable")
	client := &fakeToolClient{err: wantErr}

	_, err := NewService(Options{Client: client}).BuildToolList(context.Background(), ServerConfig{
		ID:        uuid.New(),
		UpdatedAt: time.Now(),
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("BuildToolList error = %v, want %v", err, wantErr)
	}
}

func TestServiceCacheStatusReportsRuntimeAndMissOnly(t *testing.T) {
	// 验证 CacheStatus 只识别运行期缓存，数据库展示数据不产生有效缓存状态。
	now := time.Now()
	server := ServerConfig{ID: uuid.New(), UpdatedAt: now}
	cache := &fakeToolCache{
		found: true,
		entry: CacheEntry{
			Fingerprint: Fingerprint(server),
			ExpiresAt:   now.Add(time.Minute),
			Tools:       []ToolInfo{{Name: "runtime", InputSchema: map[string]any{"type": "object"}}},
		},
	}

	status, err := NewService(Options{Cache: cache}).CacheStatus(context.Background(), server)
	if err != nil {
		t.Fatalf("CacheStatus runtime error = %v", err)
	}
	if !status.Valid || status.Source != "runtime" {
		t.Fatalf("runtime status = %#v, want valid runtime", status)
	}

	syncedAt := now
	status, err = NewService(Options{Now: func() time.Time { return now }}).CacheStatus(context.Background(), ServerConfig{
		ID:         uuid.New(),
		UpdatedAt:  now,
		ToolsCache: []ToolMeta{{Name: "db"}},
		SyncedAt:   &syncedAt,
	})
	if err != nil {
		t.Fatalf("CacheStatus display cache error = %v", err)
	}
	if status.Valid {
		t.Fatalf("display cache status = %#v, want invalid", status)
	}

	status, err = NewService(Options{}).CacheStatus(context.Background(), ServerConfig{ID: uuid.New(), UpdatedAt: now})
	if err != nil {
		t.Fatalf("CacheStatus miss error = %v", err)
	}
	if status.Valid {
		t.Fatalf("miss status = %#v, want invalid", status)
	}
}

func TestFingerprintUsesConnectionConfigurationOnly(t *testing.T) {
	// 验证展示字段变化不会影响缓存指纹，连接配置或认证变化才会使缓存失效。
	id := uuid.New()
	base := ServerConfig{
		ID:        id,
		Transport: TransportStreamableHTTP,
		URL:       "https://example.com/mcp",
		AuthType:  AuthBearer,
		AuthConfig: map[string]string{
			"token": "token-a",
		},
		UpdatedAt: time.Now(),
		ToolsCache: []ToolMeta{
			{Name: "old", Description: "old desc"},
		},
	}
	displayChanged := base
	displayChanged.UpdatedAt = base.UpdatedAt.Add(time.Hour)
	displayChanged.ToolsCache = []ToolMeta{{Name: "new", Description: "new desc"}}
	if Fingerprint(base) != Fingerprint(displayChanged) {
		t.Fatalf("fingerprint changed after display-only fields changed")
	}

	urlChanged := base
	urlChanged.URL = "https://example.com/other"
	if Fingerprint(base) == Fingerprint(urlChanged) {
		t.Fatalf("fingerprint did not change after URL changed")
	}

	authChanged := base
	authChanged.AuthConfig = map[string]string{"token": "token-b"}
	if Fingerprint(base) == Fingerprint(authChanged) {
		t.Fatalf("fingerprint did not change after auth config changed")
	}
}

func TestMetasFromToolsStripsRuntimeOnlyFields(t *testing.T) {
	// 验证写入数据库前只保留工具元信息，不包含 schema/title 等运行期字段。
	tools := []ToolInfo{{
		Name:         "search",
		Description:  "web search",
		Title:        "Search",
		InputSchema:  map[string]any{"type": "object"},
		OutputSchema: map[string]any{"type": "string"},
		Annotations:  map[string]any{"title": "Search"},
		Icons:        []map[string]any{{"src": "icon.png"}},
	}}

	got := MetasFromTools(tools)
	want := []ToolMeta{{Name: "search", Description: "web search"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("metas = %#v, want %#v", got, want)
	}
}
