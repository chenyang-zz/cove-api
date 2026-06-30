package mcp

import (
	"net/http"
	"testing"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestSDKToolClientTransportAppendsAPIKeyQuery(t *testing.T) {
	// 验证 query placement 会把 API key 追加到 MCP endpoint URL 上。
	transport, err := NewSDKToolClient(nil).transport(ServerConfig{
		Transport: TransportStreamableHTTP,
		URL:       "https://example.com/mcp",
		AuthType:  AuthAPIKey,
		AuthConfig: map[string]string{
			"placement":   "query",
			"query_param": "key",
			"key":         "plain-key",
		},
	})
	if err != nil {
		t.Fatalf("transport error = %v", err)
	}
	got := transport.(*sdkmcp.StreamableClientTransport).Endpoint
	if got != "https://example.com/mcp?key=plain-key" {
		t.Fatalf("endpoint = %q, want query key appended", got)
	}
}

func TestSDKToolClientTransportMergesAPIKeyQuery(t *testing.T) {
	// 验证 URL 已有 query 时会追加 API key，且已有同名参数会被认证配置覆盖。
	transport, err := NewSDKToolClient(nil).transport(ServerConfig{
		Transport: TransportSSE,
		URL:       "https://example.com/sse?foo=bar&key=old",
		AuthType:  AuthAPIKey,
		AuthConfig: map[string]string{
			"placement": "query",
			"key":       "plain-key",
		},
	})
	if err != nil {
		t.Fatalf("transport error = %v", err)
	}
	got := transport.(*sdkmcp.SSEClientTransport).Endpoint
	if got != "https://example.com/sse?foo=bar&key=plain-key" {
		t.Fatalf("endpoint = %q, want query key merged", got)
	}
}

func TestSDKToolClientAPIKeyDefaultsToHeader(t *testing.T) {
	// 验证旧数据没有 placement 时仍按 header 方式注入 API key。
	roundTripper := recordingRoundTripper{}
	client := NewSDKToolClient(&http.Client{Transport: &roundTripper}).authHTTPClient(ServerConfig{
		AuthType: AuthAPIKey,
		AuthConfig: map[string]string{
			"key": "plain-key",
		},
	})
	req, err := http.NewRequest(http.MethodGet, "https://example.com/mcp", nil)
	if err != nil {
		t.Fatalf("NewRequest error = %v", err)
	}
	_, err = client.Do(req)
	if err != nil {
		t.Fatalf("client Do error = %v", err)
	}
	if roundTripper.header.Get("X-Api-Key") != "plain-key" {
		t.Fatalf("X-Api-Key = %q, want plain-key", roundTripper.header.Get("X-Api-Key"))
	}
}

type recordingRoundTripper struct {
	header http.Header
}

func (t *recordingRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	t.header = req.Header.Clone()
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       http.NoBody,
		Header:     http.Header{},
		Request:    req,
	}, nil
}
