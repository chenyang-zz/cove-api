package webcrawl

import (
	"net/http"
	"time"
)

const (
	defaultTimeout      = 20 * time.Second
	defaultMaxRedirects = 5
	defaultRetryCount   = 1
	defaultMaxBodyBytes = 50 * 1024 * 1024
)

// Options 定义 Crawler 的长期配置。
type Options struct {
	HTTPClient   HTTPClient
	Extractor    Extractor
	URLGuard     URLGuard
	Timeout      time.Duration
	MaxRedirects int
	RetryCount   int
	MaxBodyBytes int64
}

// Option 修改 Crawler 的长期配置。
type Option func(*Options)

// WithHTTPClient 设置自定义 HTTP client。
func WithHTTPClient(client HTTPClient) Option {
	return func(opts *Options) {
		if client != nil {
			opts.HTTPClient = client
		}
	}
}

// WithExtractor 设置自定义网页正文提取器。
func WithExtractor(extractor Extractor) Option {
	return func(opts *Options) {
		if extractor != nil {
			opts.Extractor = extractor
		}
	}
}

// WithURLGuard 设置自定义 URL 安全校验器。
func WithURLGuard(guard URLGuard) Option {
	return func(opts *Options) {
		if guard != nil {
			opts.URLGuard = guard
		}
	}
}

// WithTimeout 设置默认 HTTP client 的请求超时时间。
func WithTimeout(timeout time.Duration) Option {
	return func(opts *Options) {
		if timeout > 0 {
			opts.Timeout = timeout
		}
	}
}

// WithMaxRedirects 设置默认 HTTP client 允许的最大重定向次数。
func WithMaxRedirects(maxRedirects int) Option {
	return func(opts *Options) {
		if maxRedirects >= 0 {
			opts.MaxRedirects = maxRedirects
		}
	}
}

// WithRetryCount 设置抓取瞬时失败后的重试次数。
func WithRetryCount(retryCount int) Option {
	return func(opts *Options) {
		if retryCount >= 0 {
			opts.RetryCount = retryCount
		}
	}
}

// WithMaxBodyBytes 设置网页响应体最大读取字节数。
//
// maxBytes 小于等于 0 时保留默认上限。
func WithMaxBodyBytes(maxBytes int64) Option {
	return func(opts *Options) {
		if maxBytes > 0 {
			opts.MaxBodyBytes = maxBytes
		}
	}
}

// defaultHTTPClient 构造带超时和重定向安全校验的 HTTP client。
func defaultHTTPClient(timeout time.Duration, maxRedirects int, guard URLGuard) *http.Client {
	return &http.Client{
		Timeout: timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= maxRedirects {
				return http.ErrUseLastResponse
			}
			if guard != nil {
				return guard.Validate(req.Context(), req.URL.String())
			}
			return nil
		},
	}
}
