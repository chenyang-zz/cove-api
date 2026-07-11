package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

// SDKToolClient 使用官方 Go SDK 发现和调用 MCP 工具。
type SDKToolClient struct {
	httpClient *http.Client
}

// NewSDKToolClient 创建 SDK client；httpClient 为空时使用带建连超时的默认客户端。
//
// 默认客户端不设置整体 Client.Timeout，避免截断可能较长的 CallTool 请求；
// 发现路径的上界由 Service.DiscoverTimeout 通过 context 控制。
func NewSDKToolClient(httpClient *http.Client) *SDKToolClient {
	if httpClient == nil {
		httpClient = defaultHTTPClient()
	}
	return &SDKToolClient{httpClient: httpClient}
}

// defaultHTTPClient 返回带 TCP/TLS 建连超时的 HTTP 客户端。
//
// 不设置 Client.Timeout，以便工具调用可按请求 ctx 运行较长时间。
func defaultHTTPClient() *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DialContext = (&net.Dialer{
		Timeout:   DefaultDialTimeout,
		KeepAlive: 30 * time.Second,
	}).DialContext
	if transport.TLSHandshakeTimeout <= 0 || transport.TLSHandshakeTimeout > DefaultDialTimeout {
		transport.TLSHandshakeTimeout = DefaultDialTimeout
	}
	return &http.Client{Transport: transport}
}

// ListTools 建立一次临时 session，读取工具列表后立即关闭连接。
func (c *SDKToolClient) ListTools(ctx context.Context, server ServerConfig) ([]ToolInfo, error) {
	session, err := c.OpenSession(ctx, server)
	if err != nil {
		return nil, err
	}
	defer session.Close()
	return session.ListTools(ctx)
}

// OpenSession 连接指定 MCP server，并返回可复用的 SDK session 适配器。
func (c *SDKToolClient) OpenSession(ctx context.Context, server ServerConfig) (ToolSession, error) {
	transport, err := c.transport(server)
	if err != nil {
		return nil, err
	}
	client := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "boxify-api-go", Version: "0.1.0"}, nil)
	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		return nil, fmt.Errorf("连接mcp服务器失败 %v", err)
	}
	return &sdkToolSession{session: session}, nil
}

type sdkToolSession struct {
	session *sdkmcp.ClientSession
}

func (s *sdkToolSession) ListTools(ctx context.Context) ([]ToolInfo, error) {
	if s == nil || s.session == nil {
		return nil, fmt.Errorf("mcp session is nil")
	}

	result, err := s.session.ListTools(ctx, &sdkmcp.ListToolsParams{})
	if err != nil {
		return nil, err
	}
	return toolInfosFromSDK(result.Tools), nil
}

func (s *sdkToolSession) CallTool(ctx context.Context, name string, input map[string]any) (*CallResult, error) {
	if s == nil || s.session == nil {
		return nil, fmt.Errorf("mcp session is nil")
	}
	result, err := s.session.CallTool(ctx, &sdkmcp.CallToolParams{Name: name, Arguments: input})
	if err != nil {
		return nil, err
	}
	out := &CallResult{
		Content:           make([]Content, 0, len(result.Content)),
		StructuredContent: result.StructuredContent,
		IsError:           result.IsError,
	}
	for _, item := range result.Content {
		if item == nil {
			continue
		}
		data, err := json.Marshal(item)
		if err != nil {
			return nil, err
		}
		content, err := contentFromJSON(data)
		if err != nil {
			return nil, err
		}
		out.Content = append(out.Content, content)
	}
	return out, nil
}

func (s *sdkToolSession) Close() error {
	if s == nil || s.session == nil {
		return nil
	}
	return s.session.Close()
}

func toolInfosFromSDK(tools []*sdkmcp.Tool) []ToolInfo {
	out := make([]ToolInfo, 0, len(tools))
	for _, tool := range tools {
		if tool == nil {
			continue
		}
		out = append(out, ToolInfo{
			Name:         tool.Name,
			Description:  tool.Description,
			Title:        tool.Title,
			InputSchema:  tool.InputSchema,
			OutputSchema: tool.OutputSchema,
			Annotations:  tool.Annotations,
			Icons:        tool.Icons,
		})
	}
	return out
}

func (c *SDKToolClient) transport(server ServerConfig) (sdkmcp.Transport, error) {
	httpClient := c.authHTTPClient(server)
	endpoint, err := authEndpointURL(server)
	if err != nil {
		return nil, err
	}
	switch server.Transport {
	case TransportSSE:
		return &sdkmcp.SSEClientTransport{Endpoint: endpoint, HTTPClient: httpClient}, nil
	case "", TransportStreamableHTTP:
		return &sdkmcp.StreamableClientTransport{Endpoint: endpoint, HTTPClient: httpClient}, nil
	default:
		return nil, fmt.Errorf("unsupported MCP transport %q", server.Transport)
	}
}

func (c *SDKToolClient) authHTTPClient(server ServerConfig) *http.Client {
	base := defaultHTTPClient()
	if c != nil && c.httpClient != nil {
		base = c.httpClient
	}
	headers := map[string]string{}
	switch server.AuthType {
	case AuthBearer:
		if token := server.AuthConfig["token"]; token != "" {
			headers["Authorization"] = "Bearer " + token
		}
	case AuthAPIKey:
		if key := server.AuthConfig["key"]; key != "" && apiKeyPlacement(server) != "query" {
			header := server.AuthConfig["header"]
			if header == "" {
				header = "X-Api-Key"
			}
			headers[header] = key
		}
	}
	if len(headers) == 0 {
		return base
	}
	clone := *base
	clone.Transport = headerRoundTripper{
		base:    base.Transport,
		headers: headers,
	}
	return &clone
}

func authEndpointURL(server ServerConfig) (string, error) {
	if server.AuthType != AuthAPIKey || apiKeyPlacement(server) != "query" || server.AuthConfig["key"] == "" {
		return server.URL, nil
	}
	parsed, err := url.Parse(server.URL)
	if err != nil {
		return "", fmt.Errorf("invalid MCP endpoint URL %q: %w", server.URL, err)
	}
	queryParam := server.AuthConfig["query_param"]
	if queryParam == "" {
		queryParam = "key"
	}
	values := parsed.Query()
	values.Set(queryParam, server.AuthConfig["key"])
	parsed.RawQuery = values.Encode()
	return parsed.String(), nil
}

func apiKeyPlacement(server ServerConfig) string {
	placement := strings.ToLower(server.AuthConfig["placement"])
	if placement == "" {
		return "header"
	}
	return placement
}

type headerRoundTripper struct {
	base    http.RoundTripper
	headers map[string]string
}

func (t headerRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	next := t.base
	if next == nil {
		next = http.DefaultTransport
	}
	clone := req.Clone(req.Context())
	for key, value := range t.headers {
		clone.Header.Set(key, value)
	}
	return next.RoundTrip(clone)
}
