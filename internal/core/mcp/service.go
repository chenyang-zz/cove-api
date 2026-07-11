package mcp

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// ToolClient 提供不复用 session 的 MCP 工具发现能力。
type ToolClient interface {
	ListTools(ctx context.Context, server ServerConfig) ([]ToolInfo, error)
}

// ToolSession 表示一次可复用的 MCP 服务连接。
//
// 同一个 session 的工具调用由 OpenedTools 串行化；Close 应允许重复调用。
type ToolSession interface {
	ListTools(ctx context.Context) ([]ToolInfo, error)
	CallTool(ctx context.Context, name string, input map[string]any) (*CallResult, error)
	Close() error
}

// SessionOpener 创建指定 MCP 服务的一次可复用连接。
type SessionOpener interface {
	OpenSession(ctx context.Context, server ServerConfig) (ToolSession, error)
}

// failState 记录单个 MCP server 最近一次发现失败的冷却信息。
type failState struct {
	until       time.Time
	message     string
	fingerprint string
}

// Service 统一管理 MCP 工具发现缓存和单轮可复用工具会话。
type Service struct {
	client          ToolClient
	sessionOpener   SessionOpener
	cache           ToolCache
	ttl             time.Duration
	discoverTimeout time.Duration
	failCooldown    time.Duration
	now             func() time.Time

	failMu   sync.Mutex
	failures map[string]failState
}

// NewService 创建 MCP 服务。
//
// 先填充完整默认值（SDK client、内存缓存、默认超时与冷却），再应用 opts。
// 选项中的无效零值会被忽略并保留默认。
func NewService(opts ...Option) *Service {
	options := defaultOptions()
	for _, opt := range opts {
		if opt != nil {
			opt(&options)
		}
	}

	// 后处理：补齐依赖默认实例，并纠正可能被错误写入的零值。
	if options.TTL <= 0 {
		options.TTL = DefaultTTL
	}
	if options.DiscoverTimeout <= 0 {
		options.DiscoverTimeout = DefaultDiscoverTimeout
	}
	if options.FailCooldown <= 0 {
		options.FailCooldown = DefaultFailCooldown
	}
	if options.Now == nil {
		options.Now = time.Now
	}
	if options.Cache == nil {
		options.Cache = NewMemoryToolCache()
	}
	defaultClient := NewSDKToolClient(defaultHTTPClient())
	if options.Client == nil {
		options.Client = defaultClient
	}
	if options.SessionOpener == nil {
		if opener, ok := options.Client.(SessionOpener); ok {
			options.SessionOpener = opener
		} else {
			options.SessionOpener = defaultClient
		}
	}

	return &Service{
		client:          options.Client,
		sessionOpener:   options.SessionOpener,
		cache:           options.Cache,
		ttl:             options.TTL,
		discoverTimeout: options.DiscoverTimeout,
		failCooldown:    options.FailCooldown,
		now:             options.Now,
		failures:        map[string]failState{},
	}
}

// BuildToolList 返回 MCP 工具列表；有效缓存直接命中，否则同步刷新远端。
//
// 发现路径受 DiscoverTimeout 限制。失败后进入 FailCooldown：冷却期内不再访问远端，
// 若存在指纹匹配的过期 runtime 缓存则返回该 stale 列表。
func (s *Service) BuildToolList(ctx context.Context, server ServerConfig) ([]ToolInfo, error) {
	s = s.withDefaults()
	entry, ok, err := s.cache.Get(ctx, cacheKey(server))
	if err != nil {
		return nil, err
	}
	if ok && s.cache.Valid(server, entry) {
		return cloneTools(entry.Tools), nil
	}
	// 冷却期内直接复用 stale 或快速失败，避免再次阻塞对话/配置页。
	if err := s.cooldownError(server); err != nil {
		if tools, staleOK := staleTools(server, entry, ok); staleOK {
			return tools, nil
		}
		return nil, err
	}

	tools, err := s.listToolsRemote(ctx, server)
	if err != nil {
		s.rememberFailure(server, err)
		if tools, staleOK := staleTools(server, entry, ok); staleOK {
			return tools, nil
		}
		return nil, err
	}
	s.clearFailure(server)
	tools = cloneTools(tools)
	if err := s.cache.Set(ctx, cacheKey(server), s.newEntry(server, tools)); err != nil {
		return nil, err
	}
	return tools, nil
}

// RefreshToolList 跳过 TTL 缓存与失败冷却，强制重新拉取远端 MCP 工具列表。
//
// 仍受 DiscoverTimeout 约束。拉取成功后更新运行时缓存并清除失败冷却；
// 远端调用或缓存写入失败时返回对应错误（不降级到 stale）。
func (s *Service) RefreshToolList(ctx context.Context, server ServerConfig) ([]ToolInfo, error) {
	s = s.withDefaults()
	tools, err := s.listToolsRemote(ctx, server)
	if err != nil {
		s.rememberFailure(server, err)
		return nil, err
	}
	s.clearFailure(server)
	tools = cloneTools(tools)
	if err := s.cache.Set(ctx, cacheKey(server), s.newEntry(server, tools)); err != nil {
		return nil, err
	}
	return tools, nil
}

// OpenTools 返回一组可在本轮调用的 MCP 工具及其连接 lease。
//
// 有效 TTL 缓存会直接提供工具元数据，并把 session 建立延迟到首次 CallTool；缓存
// 失效时会在 DiscoverTimeout 内建立 session、刷新工具列表并复用该连接。发现失败后
// 进入 FailCooldown；若存在指纹匹配的 stale 工具列表则降级返回 lazy lease。
// 调用方必须调用 Close。
func (s *Service) OpenTools(ctx context.Context, server ServerConfig) (*OpenedTools, error) {
	s = s.withDefaults()
	entry, ok, err := s.cache.Get(ctx, cacheKey(server))
	if err != nil {
		return nil, err
	}
	if ok && s.cache.Valid(server, entry) {
		return newOpenedTools(server, entry.Tools, s.sessionOpener, nil), nil
	}
	// 冷却期内优先返回 stale tools，否则立即失败，避免再次等待发现超时。
	if err := s.cooldownError(server); err != nil {
		if tools, staleOK := staleTools(server, entry, ok); staleOK {
			return newOpenedTools(server, tools, s.sessionOpener, nil), nil
		}
		return nil, err
	}

	session, tools, err := s.openAndList(ctx, server)
	if err != nil {
		s.rememberFailure(server, err)
		if tools, staleOK := staleTools(server, entry, ok); staleOK {
			return newOpenedTools(server, tools, s.sessionOpener, nil), nil
		}
		return nil, err
	}
	s.clearFailure(server)
	tools = cloneTools(tools)
	if err := s.cache.Set(ctx, cacheKey(server), s.newEntry(server, tools)); err != nil {
		_ = session.Close()
		return nil, err
	}
	return newOpenedTools(server, tools, s.sessionOpener, session), nil
}

// CacheStatus 返回指定 server 当前运行时缓存是否有效及其过期时间。
func (s *Service) CacheStatus(ctx context.Context, server ServerConfig) (CacheStatus, error) {
	s = s.withDefaults()
	key := cacheKey(server)
	entry, ok, err := s.cache.Get(ctx, key)
	if err != nil {
		return CacheStatus{}, err
	}
	if ok && s.cache.Valid(server, entry) {
		return CacheStatus{Valid: true, Source: "runtime", Fingerprint: entry.Fingerprint, ExpiresAt: entry.ExpiresAt}, nil
	}
	return CacheStatus{Valid: false, Fingerprint: Fingerprint(server)}, nil
}

func (s *Service) listToolsRemote(ctx context.Context, server ServerConfig) ([]ToolInfo, error) {
	discoverCtx, cancel := context.WithTimeout(ctx, s.discoverTimeout)
	defer cancel()
	return s.client.ListTools(discoverCtx, server)
}

// openAndList 在发现超时内建立 session 并列举工具；ListTools 失败时关闭 session。
func (s *Service) openAndList(ctx context.Context, server ServerConfig) (ToolSession, []ToolInfo, error) {
	discoverCtx, cancel := context.WithTimeout(ctx, s.discoverTimeout)
	defer cancel()

	session, err := s.sessionOpener.OpenSession(discoverCtx, server)
	if err != nil {
		return nil, nil, err
	}
	tools, err := session.ListTools(discoverCtx)
	if err != nil {
		_ = session.Close()
		return nil, nil, err
	}
	return session, tools, nil
}

func (s *Service) cooldownError(server ServerConfig) error {
	key := cacheKey(server)
	fp := Fingerprint(server)

	s.failMu.Lock()
	defer s.failMu.Unlock()
	if s.failures == nil {
		return nil
	}
	state, ok := s.failures[key]
	if !ok {
		return nil
	}
	// 连接配置变更后旧冷却不再适用。
	if state.fingerprint != fp {
		delete(s.failures, key)
		return nil
	}
	if !s.now().Before(state.until) {
		return nil
	}
	return fmt.Errorf("mcp server in fail cooldown: %s", state.message)
}

func (s *Service) rememberFailure(server ServerConfig, err error) {
	if err == nil {
		return
	}
	key := cacheKey(server)
	s.failMu.Lock()
	defer s.failMu.Unlock()
	if s.failures == nil {
		s.failures = map[string]failState{}
	}
	s.failures[key] = failState{
		until:       s.now().Add(s.failCooldown),
		message:     err.Error(),
		fingerprint: Fingerprint(server),
	}
}

func (s *Service) clearFailure(server ServerConfig) {
	key := cacheKey(server)
	s.failMu.Lock()
	defer s.failMu.Unlock()
	if s.failures == nil {
		return
	}
	delete(s.failures, key)
}

func staleTools(server ServerConfig, entry CacheEntry, ok bool) ([]ToolInfo, bool) {
	if !ok || !Stale(server, entry) {
		return nil, false
	}
	return cloneTools(entry.Tools), true
}

func (s *Service) withDefaults() *Service {
	if s != nil {
		if s.ttl <= 0 {
			s.ttl = DefaultTTL
		}
		if s.discoverTimeout <= 0 {
			s.discoverTimeout = DefaultDiscoverTimeout
		}
		if s.failCooldown <= 0 {
			s.failCooldown = DefaultFailCooldown
		}
		if s.now == nil {
			s.now = time.Now
		}
		if s.cache == nil {
			s.cache = NewMemoryToolCache()
		}
		if s.failures == nil {
			s.failures = map[string]failState{}
		}
		if s.client == nil {
			client := NewSDKToolClient(defaultHTTPClient())
			s.client = client
			if s.sessionOpener == nil {
				s.sessionOpener = client
			}
		}
		if s.sessionOpener == nil {
			if opener, ok := s.client.(SessionOpener); ok {
				s.sessionOpener = opener
			} else {
				s.sessionOpener = NewSDKToolClient(defaultHTTPClient())
			}
		}
		return s
	}
	return NewService()
}

func (s *Service) newEntry(server ServerConfig, tools []ToolInfo) CacheEntry {
	return CacheEntry{
		Fingerprint: Fingerprint(server),
		ExpiresAt:   s.now().Add(s.ttl),
		Tools:       cloneTools(tools),
	}
}

func cacheKey(server ServerConfig) string {
	return server.ID.String()
}
