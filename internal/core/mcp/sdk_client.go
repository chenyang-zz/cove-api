package mcp

import (
	"context"
	"fmt"
	"net/http"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

type SDKToolClient struct {
	httpClient *http.Client
}

func NewSDKToolClient(httpClient *http.Client) *SDKToolClient {
	return &SDKToolClient{httpClient: httpClient}
}

func (c *SDKToolClient) ListTools(ctx context.Context, server ServerConfig) ([]ToolInfo, error) {
	transport, err := c.transport(server)
	if err != nil {
		return nil, err
	}
	client := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "boxify-api-go", Version: "0.1.0"}, nil)
	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		return nil, err
	}
	defer session.Close()

	result, err := session.ListTools(ctx, &sdkmcp.ListToolsParams{})
	if err != nil {
		return nil, err
	}
	out := make([]ToolInfo, 0, len(result.Tools))
	for _, tool := range result.Tools {
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
	return out, nil
}

func (c *SDKToolClient) transport(server ServerConfig) (sdkmcp.Transport, error) {
	httpClient := c.authHTTPClient(server)
	switch server.Transport {
	case TransportSSE:
		return &sdkmcp.SSEClientTransport{Endpoint: server.URL, HTTPClient: httpClient}, nil
	case "", TransportStreamableHTTP:
		return &sdkmcp.StreamableClientTransport{Endpoint: server.URL, HTTPClient: httpClient}, nil
	default:
		return nil, fmt.Errorf("unsupported MCP transport %q", server.Transport)
	}
}

func (c *SDKToolClient) authHTTPClient(server ServerConfig) *http.Client {
	base := http.DefaultClient
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
		if key := server.AuthConfig["key"]; key != "" {
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
