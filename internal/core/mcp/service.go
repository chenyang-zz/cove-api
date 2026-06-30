package mcp

import (
	"context"
	"time"
)

type ToolClient interface {
	ListTools(ctx context.Context, server ServerConfig) ([]ToolInfo, error)
}

type Options struct {
	Client ToolClient
	Cache  ToolCache
	TTL    time.Duration
	Now    func() time.Time
}

type Service struct {
	client ToolClient
	cache  ToolCache
	ttl    time.Duration
	now    func() time.Time
}

func NewService(options Options) *Service {
	ttl := options.TTL
	if ttl <= 0 {
		ttl = DefaultTTL
	}
	now := options.Now
	if now == nil {
		now = time.Now
	}
	cache := options.Cache
	if cache == nil {
		cache = NewMemoryToolCache()
	}
	client := options.Client
	if client == nil {
		client = NewSDKToolClient(nil)
	}
	return &Service{
		client: client,
		cache:  cache,
		ttl:    ttl,
		now:    now,
	}
}

func (s *Service) BuildToolList(ctx context.Context, server ServerConfig) ([]ToolInfo, error) {
	s = s.withDefaults()
	key := cacheKey(server)
	if entry, ok, err := s.cache.Get(ctx, key); err != nil {
		return nil, err
	} else if ok && s.cache.Valid(server, entry) {
		return cloneTools(entry.Tools), nil
	}

	tools, err := s.client.ListTools(ctx, server)
	if err != nil {
		return nil, err
	}
	tools = cloneTools(tools)
	if err := s.cache.Set(ctx, key, s.newEntry(server, tools)); err != nil {
		return nil, err
	}
	return tools, nil
}

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

func (s *Service) withDefaults() *Service {
	if s != nil {
		if s.ttl <= 0 {
			s.ttl = DefaultTTL
		}
		if s.now == nil {
			s.now = time.Now
		}
		if s.cache == nil {
			s.cache = NewMemoryToolCache()
		}
		if s.client == nil {
			s.client = NewSDKToolClient(nil)
		}
		return s
	}
	return NewService(Options{})
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
