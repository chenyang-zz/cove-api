package webcrawl

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type fakeHTTPClient struct {
	requests []*http.Request
	resps    []*http.Response
	errs     []error
}

func (f *fakeHTTPClient) Do(req *http.Request) (*http.Response, error) {
	f.requests = append(f.requests, req)
	idx := len(f.requests) - 1
	if idx < len(f.errs) && f.errs[idx] != nil {
		return nil, f.errs[idx]
	}
	if idx < len(f.resps) {
		return f.resps[idx], nil
	}
	return nil, errors.New("unexpected request")
}

type fakeGuard struct {
	err error
}

func (f fakeGuard) Validate(ctx context.Context, rawURL string) error {
	return f.err
}

type recordingGuard struct {
	urls []string
	err  error
}

func (g *recordingGuard) Validate(ctx context.Context, rawURL string) error {
	g.urls = append(g.urls, rawURL)
	return g.err
}

type fakeExtractor struct {
	out *Output
	err error
}

func (f fakeExtractor) Extract(ctx context.Context, page Page) (*Output, error) {
	return f.out, f.err
}

type fakeResolver struct {
	ips []net.IP
	err error
}

func (f fakeResolver) LookupIP(ctx context.Context, host string) ([]net.IP, error) {
	return f.ips, f.err
}

func TestDefaultURLGuardRejectsUnsafeURLs(t *testing.T) {
	// 验证默认 URL guard 会拒绝非法 scheme、本地地址和解析到内网的域名。
	cases := []struct {
		name string
		url  string
	}{
		{name: "scheme", url: "file:///etc/passwd"},
		{name: "localhost", url: "http://localhost/a"},
		{name: "private-ip", url: "http://10.0.0.1/a"},
		{name: "missing-host", url: "https:///a"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			guard := NewURLGuard()
			if err := guard.Validate(context.Background(), tc.url); err == nil {
				t.Fatalf("Validate(%q) error = nil, want error", tc.url)
			}
		})
	}

	guard := NewURLGuard(WithResolver(fakeResolver{ips: []net.IP{net.ParseIP("192.168.1.1")}}))
	if err := guard.Validate(context.Background(), "https://example.com"); err == nil {
		t.Fatal("Validate private DNS result error = nil, want error")
	}
}

func TestCrawlerFetchSendsBrowserHeadersAndRetriesTransientError(t *testing.T) {
	// 验证 crawler 会做一次重试，并在请求中带上接近浏览器的默认 headers。
	client := &fakeHTTPClient{
		errs: []error{errors.New("timeout")},
		resps: []*http.Response{nil, {
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`<html><head><title>Doc</title></head><body><article>Hello crawler</article></body></html>`)),
			Header:     make(http.Header),
			Request:    mustRequest(t, "https://example.com/page"),
		}},
	}
	crawler := NewCrawler(
		WithHTTPClient(client),
		WithURLGuard(fakeGuard{}),
		WithRetryCount(1),
	)

	out, err := crawler.Fetch(context.Background(), Input{URL: "https://example.com/page"})
	if err != nil {
		t.Fatalf("Fetch error = %v", err)
	}
	if out.Title != "Doc" || !strings.Contains(out.Content, "Hello crawler") {
		t.Fatalf("Fetch output = %+v, want title and content", out)
	}
	if len(client.requests) != 2 {
		t.Fatalf("requests = %d, want 2", len(client.requests))
	}
	if got := client.requests[0].Header.Get("User-Agent"); !strings.Contains(got, "Mozilla") {
		t.Fatalf("User-Agent = %q, want browser-like header", got)
	}
}

func TestCrawlerRejectsUnsafeURLBeforeHTTP(t *testing.T) {
	// 验证 URL guard 失败时不会发起 HTTP 请求，避免 SSRF 防护被绕过。
	client := &fakeHTTPClient{}
	crawler := NewCrawler(WithHTTPClient(client), WithURLGuard(fakeGuard{err: errors.New("unsafe")}))

	if _, err := crawler.Fetch(context.Background(), Input{URL: "http://127.0.0.1"}); err == nil {
		t.Fatal("Fetch unsafe URL error = nil, want error")
	}
	if len(client.requests) != 0 {
		t.Fatalf("requests = %d, want 0", len(client.requests))
	}
}

func TestHTMLExtractorRemovesScriptsAndRequiresContent(t *testing.T) {
	// 验证默认 HTML extractor 会移除 script/style/title，并在无法提取正文时返回错误。
	extractor := NewHTMLExtractor()
	out, err := extractor.Extract(context.Background(), Page{
		URL:  "https://example.com",
		HTML: []byte(`<html><head><title>Title</title><style>.x{}</style></head><body><main>Hello <b>world</b><script>alert(1)</script></main></body></html>`),
	})
	if err != nil {
		t.Fatalf("Extract error = %v", err)
	}
	if out.Title != "Title" || !strings.Contains(out.Content, "Hello world") {
		t.Fatalf("Extract output = %+v, want title/content", out)
	}
	if strings.Contains(out.Content, "alert") || strings.Contains(out.Content, ".x") || strings.Contains(out.Content, "Title") {
		t.Fatalf("Extract content = %q, want script/style/title removed", out.Content)
	}

	if _, err := extractor.Extract(context.Background(), Page{URL: "https://example.com", HTML: []byte(`<html><body><script>x</script></body></html>`)}); err == nil {
		t.Fatal("Extract empty body error = nil, want error")
	}
}

func TestHTMLExtractorTitleMaxRunes(t *testing.T) {
	// 验证标题截断默认值为 200 rune，并支持通过 option 覆盖。
	longTitle := strings.Repeat("题", 220)
	out, err := NewHTMLExtractor().Extract(context.Background(), Page{
		URL:  "https://example.com",
		HTML: []byte(`<html><head><title>` + longTitle + `</title></head><body><main>content</main></body></html>`),
	})
	if err != nil {
		t.Fatalf("Extract default title error = %v", err)
	}
	if out.Title != strings.Repeat("题", 200)+"..." {
		t.Fatalf("default title rune length = %d, want 200 plus ellipsis", len([]rune(strings.TrimSuffix(out.Title, "..."))))
	}

	out, err = NewHTMLExtractor(WithTitleMaxRunes(5)).Extract(context.Background(), Page{
		URL:  "https://example.com",
		HTML: []byte(`<html><head><title>` + longTitle + `</title></head><body><main>content</main></body></html>`),
	})
	if err != nil {
		t.Fatalf("Extract custom title error = %v", err)
	}
	if out.Title != strings.Repeat("题", 5)+"..." {
		t.Fatalf("custom title = %q, want 5 runes plus ellipsis", out.Title)
	}

	out, err = NewHTMLExtractor(WithTitleMaxRunes(0)).Extract(context.Background(), Page{
		URL:  "https://example.com",
		HTML: []byte(`<html><head><title>` + longTitle + `</title></head><body><main>content</main></body></html>`),
	})
	if err != nil {
		t.Fatalf("Extract zero title option error = %v", err)
	}
	if out.Title != strings.Repeat("题", 200)+"..." {
		t.Fatalf("zero option title rune length = %d, want default 200 plus ellipsis", len([]rune(strings.TrimSuffix(out.Title, "..."))))
	}
}

func TestCrawlerAllowsExtractorInjection(t *testing.T) {
	// 验证正文提取可由外部注入，core 不绑定具体 HTML 提取策略。
	client := &fakeHTTPClient{resps: []*http.Response{{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(`<html></html>`)),
		Header:     make(http.Header),
		Request:    mustRequest(t, "https://example.com"),
	}}}
	crawler := NewCrawler(
		WithHTTPClient(client),
		WithURLGuard(fakeGuard{}),
		WithExtractor(fakeExtractor{out: &Output{Title: "Injected", Content: "Injected content"}}),
	)

	out, err := crawler.Fetch(context.Background(), Input{URL: "https://example.com"})
	if err != nil {
		t.Fatalf("Fetch error = %v", err)
	}
	if out.Title != "Injected" || out.Content != "Injected content" {
		t.Fatalf("Fetch output = %+v, want injected extractor output", out)
	}
}

func TestCrawlerFetchLimitsBodyBytes(t *testing.T) {
	// 验证 crawler 可通过 MaxBodyBytes 限制网页响应体大小，避免 URL 导入读取超大页面。
	client := &fakeHTTPClient{resps: []*http.Response{{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(`<html><body>tiny</body></html>`)),
		Header:     make(http.Header),
		Request:    mustRequest(t, "https://example.com"),
	}}}
	crawler := NewCrawler(
		WithHTTPClient(client),
		WithURLGuard(fakeGuard{}),
		WithMaxBodyBytes(128),
	)
	if _, err := crawler.Fetch(context.Background(), Input{URL: "https://example.com"}); err != nil {
		t.Fatalf("Fetch small body error = %v, want nil", err)
	}

	client = &fakeHTTPClient{resps: []*http.Response{{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(strings.Repeat("x", 129))),
		Header:     make(http.Header),
		Request:    mustRequest(t, "https://example.com/large"),
	}}}
	crawler = NewCrawler(
		WithHTTPClient(client),
		WithURLGuard(fakeGuard{}),
		WithMaxBodyBytes(128),
	)
	if _, err := crawler.Fetch(context.Background(), Input{URL: "https://example.com/large"}); err == nil {
		t.Fatal("Fetch large body error = nil, want size limit error")
	}
}

func TestDefaultHTTPClientValidatesRedirectURL(t *testing.T) {
	// 验证默认 HTTP client 跟随重定向时会再次执行 URL guard，防止通过 redirect 绕过 SSRF 检查。
	guard := &recordingGuard{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/redirect" {
			http.Redirect(w, r, "/target", http.StatusFound)
			return
		}
		_, _ = w.Write([]byte(`<html><body><main>redirect target</main></body></html>`))
	}))
	defer server.Close()
	crawler := NewCrawler(WithURLGuard(guard))

	out, err := crawler.Fetch(context.Background(), Input{URL: server.URL + "/redirect"})
	if err != nil {
		t.Fatalf("Fetch redirect error = %v", err)
	}
	if !strings.Contains(out.Content, "redirect target") {
		t.Fatalf("Fetch redirect content = %q, want redirect target", out.Content)
	}
	if len(guard.urls) < 2 {
		t.Fatalf("guard urls = %#v, want initial and redirect URLs", guard.urls)
	}
}

func mustRequest(t *testing.T, rawURL string) *http.Request {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	return req
}
