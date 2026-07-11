package mcp

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

type fakeToolClient struct {
	calls   int
	tools   []ToolInfo
	err     error
	block   bool
	lastCtx context.Context
}

func (c *fakeToolClient) ListTools(ctx context.Context, server ServerConfig) ([]ToolInfo, error) {
	c.calls++
	c.lastCtx = ctx
	if c.block {
		<-ctx.Done()
		return nil, ctx.Err()
	}
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

type fakeToolSession struct {
	tools      []ToolInfo
	result     *CallResult
	listErr    error
	callErr    error
	closeErr   error
	listCalls  int
	callCalls  int
	closeCalls int
	lastName   string
	lastInput  map[string]any
}

func (s *fakeToolSession) ListTools(context.Context) ([]ToolInfo, error) {
	s.listCalls++
	if s.listErr != nil {
		return nil, s.listErr
	}
	return cloneTools(s.tools), nil
}

func (s *fakeToolSession) CallTool(_ context.Context, name string, input map[string]any) (*CallResult, error) {
	s.callCalls++
	s.lastName = name
	s.lastInput = input
	if s.callErr != nil {
		return nil, s.callErr
	}
	return s.result, nil
}

func (s *fakeToolSession) Close() error {
	s.closeCalls++
	return s.closeErr
}

type fakeSessionOpener struct {
	session ToolSession
	err     error
	calls   int
	block   bool
	lastCtx context.Context
}

func (o *fakeSessionOpener) OpenSession(ctx context.Context, _ ServerConfig) (ToolSession, error) {
	o.calls++
	o.lastCtx = ctx
	if o.block {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	if o.err != nil {
		return nil, o.err
	}
	return o.session, nil
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

// TestNewServiceAppliesDefaultsAndIgnoresZeroOptions 验证构造器使用默认超时/冷却，并忽略无效零值选项。
func TestNewServiceAppliesDefaultsAndIgnoresZeroOptions(t *testing.T) {
	service := NewService(WithTTL(0), WithDiscoverTimeout(0), WithFailCooldown(0), WithClient(nil), WithCache(nil), WithNow(nil))
	if service == nil {
		t.Fatal("NewService() = nil, want non-nil service")
	}
	if service.ttl != DefaultTTL {
		t.Fatalf("NewService ttl = %v, want default %v", service.ttl, DefaultTTL)
	}
	if service.discoverTimeout != DefaultDiscoverTimeout {
		t.Fatalf("NewService discoverTimeout = %v, want default %v", service.discoverTimeout, DefaultDiscoverTimeout)
	}
	if service.failCooldown != DefaultFailCooldown {
		t.Fatalf("NewService failCooldown = %v, want default %v", service.failCooldown, DefaultFailCooldown)
	}
	if service.client == nil || service.sessionOpener == nil || service.cache == nil || service.now == nil {
		t.Fatal("NewService defaults left client/sessionOpener/cache/now nil")
	}

	wantTTL := 2 * time.Minute
	wantDiscover := 100 * time.Millisecond
	wantCooldown := 15 * time.Second
	custom := NewService(WithTTL(wantTTL), WithDiscoverTimeout(wantDiscover), WithFailCooldown(wantCooldown))
	if custom.ttl != wantTTL || custom.discoverTimeout != wantDiscover || custom.failCooldown != wantCooldown {
		t.Fatalf("NewService custom timeouts = %v/%v/%v, want %v/%v/%v",
			custom.ttl, custom.discoverTimeout, custom.failCooldown, wantTTL, wantDiscover, wantCooldown)
	}
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

	got, err := NewService(WithClient(client), WithCache(cache)).BuildToolList(context.Background(), server)
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

	got, err := NewService(WithClient(client), WithCache(cache)).BuildToolList(context.Background(), server)
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
	service := NewService(WithClient(client), WithTTL(5*time.Minute), WithNow(func() time.Time { return now }))

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

	got, err := NewService(WithClient(client), WithTTL(time.Minute)).BuildToolList(context.Background(), server)
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

	_, err := NewService(WithClient(client)).BuildToolList(context.Background(), ServerConfig{
		ID:        uuid.New(),
		UpdatedAt: time.Now(),
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("BuildToolList error = %v, want %v", err, wantErr)
	}
}

// TestServiceRefreshToolListBypassesValidCache 验证显式刷新始终访问远端并覆盖有效 TTL 缓存。
func TestServiceRefreshToolListBypassesValidCache(t *testing.T) {
	server := ServerConfig{ID: uuid.New()}
	client := &fakeToolClient{tools: []ToolInfo{{Name: "fresh"}}}
	cache := &fakeToolCache{
		found: true,
		entry: CacheEntry{
			Fingerprint: Fingerprint(server),
			ExpiresAt:   time.Now().Add(time.Minute),
			Tools:       []ToolInfo{{Name: "cached"}},
		},
	}

	tools, err := NewService(WithClient(client), WithCache(cache)).RefreshToolList(context.Background(), server)
	if err != nil {
		t.Fatalf("RefreshToolList error = %v, want nil", err)
	}
	if client.calls != 1 || len(tools) != 1 || tools[0].Name != "fresh" {
		t.Fatalf("RefreshToolList calls/tools = %d/%#v, want remote fresh tool", client.calls, tools)
	}
	if cache.set == nil || len(cache.set.Tools) != 1 || cache.set.Tools[0].Name != "fresh" {
		t.Fatalf("RefreshToolList cache = %#v, want refreshed entry", cache.set)
	}
}

// TestServiceOpenToolsUsesCacheAndLazilyOpensSession 验证缓存命中时不立即连接，并在首次调用后复用和幂等关闭 session。
func TestServiceOpenToolsUsesCacheAndLazilyOpensSession(t *testing.T) {
	server := ServerConfig{ID: uuid.New()}
	session := &fakeToolSession{result: &CallResult{Content: []Content{{Type: "text", Text: "ok"}}}}
	opener := &fakeSessionOpener{session: session}
	cache := &fakeToolCache{
		found: true,
		entry: CacheEntry{
			Fingerprint: Fingerprint(server),
			ExpiresAt:   time.Now().Add(time.Minute),
			Tools:       []ToolInfo{{Name: "cached"}},
		},
	}
	service := NewService(WithClient(&fakeToolClient{}), WithSessionOpener(opener), WithCache(cache))

	opened, err := service.OpenTools(context.Background(), server)
	if err != nil {
		t.Fatalf("OpenTools error = %v, want nil", err)
	}
	if opener.calls != 0 || len(opened.Tools()) != 1 || opened.Tools()[0].Name != "cached" {
		t.Fatalf("OpenTools opener/tools = %d/%#v, want lazy cached tools", opener.calls, opened.Tools())
	}
	for range 2 {
		if _, err := opened.CallTool(context.Background(), "cached", map[string]any{"q": "x"}); err != nil {
			t.Fatalf("CallTool error = %v, want nil", err)
		}
	}
	if opener.calls != 1 || session.callCalls != 2 {
		t.Fatalf("CallTool opener/session calls = %d/%d, want 1/2", opener.calls, session.callCalls)
	}
	if err := opened.Close(); err != nil {
		t.Fatalf("first Close error = %v, want nil", err)
	}
	if err := opened.Close(); err != nil {
		t.Fatalf("second Close error = %v, want nil", err)
	}
	if session.closeCalls != 1 {
		t.Fatalf("session close calls = %d, want 1", session.closeCalls)
	}
}

// TestServiceOpenToolsRefreshesWithReusableSession 验证缓存失效时使用同一个 session 完成列表刷新和后续调用。
func TestServiceOpenToolsRefreshesWithReusableSession(t *testing.T) {
	server := ServerConfig{ID: uuid.New()}
	session := &fakeToolSession{
		tools:  []ToolInfo{{Name: "fresh"}},
		result: &CallResult{Content: []Content{{Type: "text", Text: "done"}}},
	}
	opener := &fakeSessionOpener{session: session}
	cache := &fakeToolCache{}
	service := NewService(WithClient(&fakeToolClient{}), WithSessionOpener(opener), WithCache(cache))

	opened, err := service.OpenTools(context.Background(), server)
	if err != nil {
		t.Fatalf("OpenTools error = %v, want nil", err)
	}
	if opener.calls != 1 || session.listCalls != 1 || cache.set == nil {
		t.Fatalf("OpenTools opener/list/cache = %d/%d/%#v, want refreshed session", opener.calls, session.listCalls, cache.set)
	}
	if _, err := opened.CallTool(context.Background(), "fresh", nil); err != nil {
		t.Fatalf("CallTool error = %v, want nil", err)
	}
	if opener.calls != 1 || session.callCalls != 1 {
		t.Fatalf("CallTool opener/session calls = %d/%d, want 1/1", opener.calls, session.callCalls)
	}
	if err := opened.Close(); err != nil {
		t.Fatalf("Close error = %v, want nil", err)
	}
}

// TestServiceOpenToolsClosesSessionWhenRefreshFails 验证远端列举工具失败时会立即释放刚建立的 session。
func TestServiceOpenToolsClosesSessionWhenRefreshFails(t *testing.T) {
	want := errors.New("list failed")
	session := &fakeToolSession{listErr: want}
	opener := &fakeSessionOpener{session: session}
	service := NewService(WithClient(&fakeToolClient{}), WithSessionOpener(opener))

	_, err := service.OpenTools(context.Background(), ServerConfig{ID: uuid.New()})
	if !errors.Is(err, want) {
		t.Fatalf("OpenTools error = %v, want %v", err, want)
	}
	if session.closeCalls != 1 {
		t.Fatalf("session close calls = %d, want 1", session.closeCalls)
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

	status, err := NewService(WithCache(cache)).CacheStatus(context.Background(), server)
	if err != nil {
		t.Fatalf("CacheStatus runtime error = %v", err)
	}
	if !status.Valid || status.Source != "runtime" {
		t.Fatalf("runtime status = %#v, want valid runtime", status)
	}

	syncedAt := now
	status, err = NewService(WithNow(func() time.Time { return now })).CacheStatus(context.Background(), ServerConfig{
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

	status, err = NewService().CacheStatus(context.Background(), ServerConfig{ID: uuid.New(), UpdatedAt: now})
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

// TestServiceOpenToolsRespectsDiscoverTimeout 验证发现路径在 DiscoverTimeout 内失败，不无限阻塞。
func TestServiceOpenToolsRespectsDiscoverTimeout(t *testing.T) {
	server := ServerConfig{ID: uuid.New()}
	opener := &fakeSessionOpener{block: true}
	service := NewService(
		WithSessionOpener(opener),
		WithDiscoverTimeout(30*time.Millisecond),
		WithFailCooldown(time.Minute),
	)

	start := time.Now()
	_, err := service.OpenTools(context.Background(), server)
	elapsed := time.Since(start)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("OpenTools error = %v, want context.DeadlineExceeded", err)
	}
	if elapsed > 500*time.Millisecond {
		t.Fatalf("OpenTools elapsed = %v, want roughly DiscoverTimeout", elapsed)
	}
	if opener.calls != 1 {
		t.Fatalf("opener calls = %d, want 1", opener.calls)
	}
}

// TestServiceOpenToolsFailCooldownSkipsRemote 验证发现失败后冷却期内不再访问远端。
func TestServiceOpenToolsFailCooldownSkipsRemote(t *testing.T) {
	server := ServerConfig{ID: uuid.New()}
	want := errors.New("offline")
	opener := &fakeSessionOpener{err: want}
	service := NewService(
		WithSessionOpener(opener),
		WithDiscoverTimeout(time.Second),
		WithFailCooldown(time.Minute),
	)

	_, err := service.OpenTools(context.Background(), server)
	if !errors.Is(err, want) {
		t.Fatalf("first OpenTools error = %v, want %v", err, want)
	}
	_, err = service.OpenTools(context.Background(), server)
	if err == nil || !strings.Contains(err.Error(), "fail cooldown") {
		t.Fatalf("second OpenTools error = %v, want fail cooldown", err)
	}
	if opener.calls != 1 {
		t.Fatalf("opener calls = %d, want 1 after cooldown", opener.calls)
	}
}

// TestServiceOpenToolsRetriesAfterFailCooldown 验证冷却结束后会重新探测远端。
func TestServiceOpenToolsRetriesAfterFailCooldown(t *testing.T) {
	server := ServerConfig{ID: uuid.New()}
	now := time.Now()
	opener := &fakeSessionOpener{err: errors.New("offline")}
	service := NewService(
		WithSessionOpener(opener),
		WithDiscoverTimeout(time.Second),
		WithFailCooldown(10*time.Second),
		WithNow(func() time.Time { return now }),
	)

	if _, err := service.OpenTools(context.Background(), server); err == nil {
		t.Fatal("first OpenTools error = nil, want offline")
	}
	now = now.Add(11 * time.Second)
	if _, err := service.OpenTools(context.Background(), server); err == nil {
		t.Fatal("second OpenTools error = nil, want offline after cooldown")
	}
	if opener.calls != 2 {
		t.Fatalf("opener calls = %d, want 2 after cooldown expired", opener.calls)
	}
}

// TestServiceOpenToolsReturnsStaleOnDiscoverFailure 验证远端失败时复用指纹匹配的过期 runtime 工具列表。
func TestServiceOpenToolsReturnsStaleOnDiscoverFailure(t *testing.T) {
	server := ServerConfig{ID: uuid.New(), URL: "https://example.com/mcp"}
	staleTools := []ToolInfo{{Name: "stale", InputSchema: map[string]any{"type": "object"}}}
	cache := &fakeToolCache{
		found: true,
		entry: CacheEntry{
			Fingerprint: Fingerprint(server),
			ExpiresAt:   time.Now().Add(-time.Minute),
			Tools:       staleTools,
		},
	}
	opener := &fakeSessionOpener{err: errors.New("offline")}
	service := NewService(WithSessionOpener(opener), WithCache(cache), WithFailCooldown(time.Minute))

	opened, err := service.OpenTools(context.Background(), server)
	if err != nil {
		t.Fatalf("OpenTools error = %v, want stale success", err)
	}
	if opener.calls != 1 {
		t.Fatalf("opener calls = %d, want 1 for first failure", opener.calls)
	}
	if got := opened.Tools(); len(got) != 1 || got[0].Name != "stale" {
		t.Fatalf("tools = %#v, want stale tools", got)
	}

	// 冷却期内应直接返回 stale，不再打远端。
	opened, err = service.OpenTools(context.Background(), server)
	if err != nil {
		t.Fatalf("cooldown OpenTools error = %v, want stale success", err)
	}
	if opener.calls != 1 {
		t.Fatalf("opener calls = %d, want still 1 during cooldown", opener.calls)
	}
	if got := opened.Tools(); len(got) != 1 || got[0].Name != "stale" {
		t.Fatalf("cooldown tools = %#v, want stale tools", got)
	}
}

// TestServiceOpenToolsClearsFailCooldownOnSuccess 验证发现成功后会清除失败冷却，后续可立即再次发现。
func TestServiceOpenToolsClearsFailCooldownOnSuccess(t *testing.T) {
	server := ServerConfig{ID: uuid.New()}
	session := &fakeToolSession{tools: []ToolInfo{{Name: "fresh"}}}
	opener := &fakeSessionOpener{err: errors.New("offline")}
	now := time.Now()
	// 使用空缓存，避免成功后命中 TTL 掩盖冷却状态。
	cache := &fakeToolCache{}
	service := NewService(
		WithSessionOpener(opener),
		WithCache(cache),
		WithFailCooldown(10*time.Second),
		WithNow(func() time.Time { return now }),
	)

	if _, err := service.OpenTools(context.Background(), server); err == nil {
		t.Fatal("first OpenTools error = nil, want offline")
	}
	// 冷却结束后成功发现。
	now = now.Add(11 * time.Second)
	opener.err = nil
	opener.session = session
	// fakeToolCache.Set 不会让后续 Get 命中，便于验证 clearFailure 后仍会探测。
	if _, err := service.OpenTools(context.Background(), server); err != nil {
		t.Fatalf("recovery OpenTools error = %v, want nil", err)
	}
	// 若冷却未清除，下一次会因 cooldown 直接失败且 opener 不再增加。
	opener.err = errors.New("offline-again")
	if _, err := service.OpenTools(context.Background(), server); err == nil {
		t.Fatal("post-success OpenTools error = nil, want offline-again from new probe")
	}
	if opener.calls != 3 {
		t.Fatalf("opener calls = %d, want 3 (fail, success, fail)", opener.calls)
	}
}

// TestServiceRefreshToolListBypassesFailCooldown 验证手动刷新忽略失败冷却仍访问远端。
func TestServiceRefreshToolListBypassesFailCooldown(t *testing.T) {
	server := ServerConfig{ID: uuid.New()}
	client := &fakeToolClient{err: errors.New("offline")}
	service := NewService(WithClient(client), WithFailCooldown(time.Minute), WithDiscoverTimeout(time.Second))

	if _, err := service.BuildToolList(context.Background(), server); err == nil {
		t.Fatal("BuildToolList error = nil, want offline")
	}
	if client.calls != 1 {
		t.Fatalf("client calls after build = %d, want 1", client.calls)
	}
	// 冷却期内 Build 不应再打远端。
	if _, err := service.BuildToolList(context.Background(), server); err == nil {
		t.Fatal("cooldown BuildToolList error = nil, want cooldown error")
	}
	if client.calls != 1 {
		t.Fatalf("client calls during cooldown = %d, want 1", client.calls)
	}

	client.err = nil
	client.tools = []ToolInfo{{Name: "fresh"}}
	tools, err := service.RefreshToolList(context.Background(), server)
	if err != nil {
		t.Fatalf("RefreshToolList error = %v, want nil", err)
	}
	if client.calls != 2 || len(tools) != 1 || tools[0].Name != "fresh" {
		t.Fatalf("RefreshToolList calls/tools = %d/%#v, want force remote", client.calls, tools)
	}
}

// TestServiceBuildToolListRespectsDiscoverTimeout 验证 ListTools 发现路径遵守 DiscoverTimeout。
func TestServiceBuildToolListRespectsDiscoverTimeout(t *testing.T) {
	client := &fakeToolClient{block: true}
	service := NewService(WithClient(client), WithDiscoverTimeout(30*time.Millisecond))

	start := time.Now()
	_, err := service.BuildToolList(context.Background(), ServerConfig{ID: uuid.New()})
	elapsed := time.Since(start)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("BuildToolList error = %v, want context.DeadlineExceeded", err)
	}
	if elapsed > 500*time.Millisecond {
		t.Fatalf("BuildToolList elapsed = %v, want roughly DiscoverTimeout", elapsed)
	}
}

// TestServiceFailCooldownClearsWhenFingerprintChanges 验证连接配置变更后旧失败冷却不再生效。
func TestServiceFailCooldownClearsWhenFingerprintChanges(t *testing.T) {
	server := ServerConfig{ID: uuid.New(), URL: "https://a.example/mcp"}
	opener := &fakeSessionOpener{err: errors.New("offline")}
	service := NewService(WithSessionOpener(opener), WithFailCooldown(time.Minute))

	if _, err := service.OpenTools(context.Background(), server); err == nil {
		t.Fatal("first OpenTools error = nil, want offline")
	}
	server.URL = "https://b.example/mcp"
	if _, err := service.OpenTools(context.Background(), server); err == nil {
		t.Fatal("OpenTools after fingerprint change error = nil, want offline from new probe")
	}
	if opener.calls != 2 {
		t.Fatalf("opener calls = %d, want 2 after fingerprint change", opener.calls)
	}
}

// TestStaleRequiresMatchingFingerprintAndTools 验证 Stale 辅助函数的匹配条件。
func TestStaleRequiresMatchingFingerprintAndTools(t *testing.T) {
	server := ServerConfig{ID: uuid.New(), URL: "https://example.com/mcp"}
	if Stale(server, CacheEntry{Fingerprint: Fingerprint(server)}) {
		t.Fatal("Stale(empty tools) = true, want false")
	}
	if Stale(server, CacheEntry{Fingerprint: "other", Tools: []ToolInfo{{Name: "x"}}}) {
		t.Fatal("Stale(mismatched fp) = true, want false")
	}
	if !Stale(server, CacheEntry{Fingerprint: Fingerprint(server), Tools: []ToolInfo{{Name: "x"}}}) {
		t.Fatal("Stale(match) = false, want true")
	}
}
