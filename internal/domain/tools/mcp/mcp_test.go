package mcp

import (
	"context"
	"strings"
	"testing"

	coremcp "github.com/boxify/api-go/internal/core/mcp"
	coretool "github.com/boxify/api-go/internal/core/tool"
	"github.com/boxify/api-go/internal/models"
	"github.com/google/uuid"
)

// TestToolKeyIsStableValidAndServerScoped 验证 MCP key 稳定、可解析、长度合法且跨 server 不冲突。
func TestToolKeyIsStableValidAndServerScoped(t *testing.T) {
	serverID := uuid.New()
	key := ToolKey(serverID, "搜索/超长工具名称 with spaces and symbols !!!")
	if key != ToolKey(serverID, "搜索/超长工具名称 with spaces and symbols !!!") {
		t.Fatalf("ToolKey is not stable: %q", key)
	}
	if len(key) > 64 {
		t.Fatalf("ToolKey len = %d, want <= 64", len(key))
	}
	for _, char := range key {
		if !((char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') || char == '_' || char == '-') {
			t.Fatalf("ToolKey contains invalid char %q: %q", char, key)
		}
	}
	parsed, ok := ParseToolKey(key)
	if !ok || parsed != serverID {
		t.Fatalf("ParseToolKey = %s/%v, want %s/true", parsed, ok, serverID)
	}
	if key == ToolKey(uuid.New(), "搜索/超长工具名称 with spaces and symbols !!!") {
		t.Fatal("ToolKey collision across servers")
	}
}

// TestNewToolPreservesSchemaAndMCPResult 验证 MCP 工具适配会透传完整 schema、调用原始名称并保留错误结果内容。
func TestNewToolPreservesSchemaAndMCPResult(t *testing.T) {
	server := &models.MCPServer{ID: uuid.New(), Name: "搜索服务", Enabled: true}
	info := coremcp.ToolInfo{
		Name:        "remote-search",
		Title:       "远端搜索",
		Description: "执行远端搜索",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{"type": "string"},
			},
			"required": []any{"query"},
			"$defs":    map[string]any{"filter": map[string]any{"type": "object"}},
		},
	}
	session := &fakeDomainMCPSession{
		tools: []coremcp.ToolInfo{info},
		result: &coremcp.CallResult{
			Content: []coremcp.Content{
				{Type: "text", Text: "远端失败"},
				{Type: "image", Data: []byte{1, 2}, MIMEType: "image/png"},
			},
			StructuredContent: map[string]any{"code": "bad_request"},
			IsError:           true,
		},
	}
	service := coremcp.NewService(coremcp.Options{SessionOpener: fakeDomainMCPOpener{session: session}})
	opened, err := service.OpenTools(context.Background(), coremcp.ServerConfig{ID: server.ID})
	if err != nil {
		t.Fatalf("OpenTools error = %v, want nil", err)
	}
	defer opened.Close()
	definitions := Definitions(server, opened.Tools())
	if len(definitions) != 1 || definitions[0].Name != "远端搜索" {
		t.Fatalf("Definitions = %#v, want titled tool", definitions)
	}
	tool := NewTool(definitions[0], opened)
	descriptor, err := tool.Describe(context.Background())
	if err != nil {
		t.Fatalf("Describe error = %v, want nil", err)
	}
	if descriptor.Schema.Parameters.Map()["$defs"] == nil {
		t.Fatalf("Describe schema = %#v, want $defs", descriptor.Schema.Parameters.Map())
	}
	output, err := tool.Invoke(context.Background(), coretool.Input{"query": "hello"})
	if err != nil {
		t.Fatalf("Invoke error = %v, want nil for MCP IsError result", err)
	}
	if session.lastName != "remote-search" || session.lastInput["query"] != "hello" {
		t.Fatalf("CallTool name/input = %q/%#v, want remote raw name and input", session.lastName, session.lastInput)
	}
	if !strings.Contains(output.Text, "远端失败") || !strings.Contains(output.Text, `"code":"bad_request"`) {
		t.Fatalf("Output text = %q, want text and structured content", output.Text)
	}
	if len(output.Parts) != 2 || output.Parts[1].MIME != "image/png" || len(output.Parts[1].Data) != 2 {
		t.Fatalf("Output parts = %#v, want preserved image", output.Parts)
	}
	if output.Metadata["mcp_is_error"] != true || output.Metadata["error"] == nil {
		t.Fatalf("Output metadata = %#v, want MCP error observation", output.Metadata)
	}
}

type fakeDomainMCPOpener struct {
	session coremcp.ToolSession
}

func (o fakeDomainMCPOpener) OpenSession(context.Context, coremcp.ServerConfig) (coremcp.ToolSession, error) {
	return o.session, nil
}

type fakeDomainMCPSession struct {
	tools     []coremcp.ToolInfo
	result    *coremcp.CallResult
	lastName  string
	lastInput map[string]any
}

func (s *fakeDomainMCPSession) ListTools(context.Context) ([]coremcp.ToolInfo, error) {
	return append([]coremcp.ToolInfo(nil), s.tools...), nil
}

func (s *fakeDomainMCPSession) CallTool(_ context.Context, name string, input map[string]any) (*coremcp.CallResult, error) {
	s.lastName = name
	s.lastInput = input
	return s.result, nil
}

func (s *fakeDomainMCPSession) Close() error {
	return nil
}
