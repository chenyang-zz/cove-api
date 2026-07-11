package mcp

import "time"

// Options 配置 MCP 工具发现、会话建立和运行时缓存依赖。
//
// 通常通过 NewService 的函数式 Option 注入；构造器会先填充完整默认值，再应用选项。
type Options struct {
	Client          ToolClient
	SessionOpener   SessionOpener
	Cache           ToolCache
	TTL             time.Duration
	DiscoverTimeout time.Duration
	FailCooldown    time.Duration
	Now             func() time.Time
}

// Option 修改 Service 的长期配置。
type Option func(*Options)

// defaultOptions 返回构造器使用的完整默认配置。
//
// Client / SessionOpener / Cache 的默认实例在 NewService 中创建，避免共享可变依赖。
func defaultOptions() Options {
	return Options{
		TTL:             DefaultTTL,
		DiscoverTimeout: DefaultDiscoverTimeout,
		FailCooldown:    DefaultFailCooldown,
		Now:             time.Now,
	}
}

// WithClient 设置工具发现客户端。
//
// client 为 nil 时忽略，保留默认 SDK client。
func WithClient(client ToolClient) Option {
	return func(opts *Options) {
		if client != nil {
			opts.Client = client
		}
	}
}

// WithSessionOpener 设置可复用 session 的打开器。
//
// opener 为 nil 时忽略；未配置时默认复用 Client（若其实现 SessionOpener）。
func WithSessionOpener(opener SessionOpener) Option {
	return func(opts *Options) {
		if opener != nil {
			opts.SessionOpener = opener
		}
	}
}

// WithCache 设置运行时工具列表缓存。
//
// cache 为 nil 时忽略，保留默认 MemoryToolCache。
func WithCache(cache ToolCache) Option {
	return func(opts *Options) {
		if cache != nil {
			opts.Cache = cache
		}
	}
}

// WithTTL 设置运行时工具列表缓存 TTL。
//
// ttl 小于等于 0 时忽略，保留默认值。
func WithTTL(ttl time.Duration) Option {
	return func(opts *Options) {
		if ttl > 0 {
			opts.TTL = ttl
		}
	}
}

// WithDiscoverTimeout 设置 Connect + ListTools 发现路径超时。
//
// timeout 小于等于 0 时忽略，保留默认值。
func WithDiscoverTimeout(timeout time.Duration) Option {
	return func(opts *Options) {
		if timeout > 0 {
			opts.DiscoverTimeout = timeout
		}
	}
}

// WithFailCooldown 设置发现失败后的冷却窗口。
//
// cooldown 小于等于 0 时忽略，保留默认值。
func WithFailCooldown(cooldown time.Duration) Option {
	return func(opts *Options) {
		if cooldown > 0 {
			opts.FailCooldown = cooldown
		}
	}
}

// WithNow 设置用于 TTL 与失败冷却判定的时钟。
//
// now 为 nil 时忽略，保留 time.Now。
func WithNow(now func() time.Time) Option {
	return func(opts *Options) {
		if now != nil {
			opts.Now = now
		}
	}
}
