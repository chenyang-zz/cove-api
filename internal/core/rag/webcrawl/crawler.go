package webcrawl

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// Crawler 抓取网页并提取可读正文。
//
// Crawler 默认包含 URLGuard、HTMLExtractor 和带重定向校验的 HTTP client。
type Crawler struct {
	Options
}

// NewCrawler 创建带默认配置的网页抓取器。
//
// opts 会按传入顺序覆盖默认值；未传 HTTPClient 时会根据 Timeout、MaxRedirects 和 URLGuard 构造默认 client。
func NewCrawler(opts ...Option) *Crawler {
	crawler := &Crawler{
		Options: Options{
			Timeout:      defaultTimeout,
			MaxRedirects: defaultMaxRedirects,
			RetryCount:   defaultRetryCount,
			MaxBodyBytes: defaultMaxBodyBytes,
			Extractor:    NewHTMLExtractor(),
			URLGuard:     NewURLGuard(),
		},
	}
	for _, opt := range opts {
		if opt != nil {
			opt(&crawler.Options)
		}
	}
	if crawler.HTTPClient == nil {
		crawler.HTTPClient = defaultHTTPClient(crawler.Timeout, crawler.MaxRedirects, crawler.URLGuard)
	}
	return crawler
}

// Fetch 获取网页标题和正文。
//
// Fetch 会先校验 URL，再发起 HTTP 请求并调用 Extractor。HTTPClient、URLGuard 或 Extractor 缺失时返回错误。
func (c *Crawler) Fetch(ctx context.Context, input Input) (*Output, error) {
	if c == nil || c.HTTPClient == nil {
		return nil, errors.New("rag web crawler http client is nil")
	}
	if c.URLGuard == nil {
		return nil, errors.New("rag web crawler url guard is nil")
	}
	if c.Extractor == nil {
		return nil, errors.New("rag web crawler extractor is nil")
	}
	rawURL := strings.TrimSpace(input.URL)
	if err := c.URLGuard.Validate(ctx, rawURL); err != nil {
		return nil, err
	}

	var lastErr error
	for attempt := 0; attempt <= c.RetryCount; attempt++ {
		// 默认重试只覆盖抓取错误；提取器错误直接返回，让调用方看到解析失败原因。
		page, err := c.fetchOnce(ctx, rawURL)
		if err == nil {
			return c.Extractor.Extract(ctx, page)
		}
		lastErr = err
	}
	return nil, lastErr
}

// fetchOnce 发起单次 HTTP 请求并返回最终 URL 和 HTML 字节。
func (c *Crawler) fetchOnce(ctx context.Context, rawURL string) (Page, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return Page{}, err
	}
	applyBrowserHeaders(req)
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return Page{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= http.StatusInternalServerError {
		return Page{}, fmt.Errorf("web page server error: %s", resp.Status)
	}
	if resp.StatusCode >= http.StatusBadRequest {
		return Page{}, fmt.Errorf("web page request error: %s", resp.Status)
	}
	body, err := readLimitedBody(resp.Body, c.MaxBodyBytes)
	if err != nil {
		return Page{}, err
	}

	// 重定向后使用最终 URL，方便上层展示实际抓取来源。
	finalURL := rawURL
	if resp.Request != nil && resp.Request.URL != nil {
		finalURL = resp.Request.URL.String()
	}
	return Page{URL: finalURL, HTML: body}, nil
}

// readLimitedBody 读取响应体，并在配置上限时拒绝超限内容。
func readLimitedBody(reader io.Reader, maxBytes int64) ([]byte, error) {
	if maxBytes <= 0 {
		return io.ReadAll(reader)
	}
	// 多读 1 字节用于区分“刚好到上限”和“超过上限”，避免把边界值误判为超限。
	body, err := io.ReadAll(io.LimitReader(reader, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > maxBytes {
		return nil, fmt.Errorf("web page body exceeds %d bytes", maxBytes)
	}
	return body, nil
}

// applyBrowserHeaders 设置浏览器风格请求头，减少常见站点的简单拦截。
func applyBrowserHeaders(req *http.Request) {
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")
	req.Header.Set("Cache-Control", "no-cache")
}
