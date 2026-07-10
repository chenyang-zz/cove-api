package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	coremcp "github.com/boxify/api-go/internal/core/mcp"
	coretool "github.com/boxify/api-go/internal/core/tool"
	"github.com/boxify/api-go/internal/models"
)

// Definitions 从实时 MCP 工具元数据构建稳定领域定义。
func Definitions(server *models.MCPServer, tools []coremcp.ToolInfo) []*Definition {
	if server == nil {
		return nil
	}
	out := make([]*Definition, 0, len(tools))
	for _, info := range tools {
		if strings.TrimSpace(info.Name) == "" {
			continue
		}
		name := strings.TrimSpace(info.Title)
		if name == "" {
			name = strings.TrimSpace(info.Name)
		}
		out = append(out, &Definition{
			Key:         ToolKey(server.ID, info.Name),
			RawName:     info.Name,
			Name:        name,
			Description: info.Description,
			ServerID:    server.ID,
			ServerName:  server.Name,
			Info:        info,
		})
	}
	return out
}

// SnapshotDefinitions 从 PG ToolsCache 构建展示用领域定义。
func SnapshotDefinitions(server *models.MCPServer) []*Definition {
	if server == nil {
		return nil
	}
	tools := make([]coremcp.ToolInfo, 0, len(server.ToolsCache))
	for _, item := range server.ToolsCache {
		if item != nil {
			tools = append(tools, coremcp.ToolInfo{Name: item.Name, Description: item.Description})
		}
	}
	return Definitions(server, tools)
}

// NewTool 将领域定义适配为可由 core agent 调用的工具。
//
// transport/protocol error 会直接返回；MCP IsError 仍作为完整 Output 返回，供模型观察。
func NewTool(definition *Definition, opened *coremcp.OpenedTools) coretool.Tool {
	if definition == nil {
		return nil
	}
	descriptor := coretool.Descriptor{
		Name:        definition.Key,
		Description: definition.Description,
		Schema: coretool.Schema{
			Parameters: schemaFromAny(definition.Info.InputSchema),
		},
		Annotations: map[string]any{
			"display_name": definition.Name,
			"icon":         "🔌",
			"tool_type":    "mcp",
			"server_id":    definition.ServerID.String(),
			"server_name":  definition.ServerName,
			"remote_name":  definition.RawName,
		},
	}
	return coretool.NewFuncTool(descriptor, func(ctx context.Context, input coretool.Input) (coretool.Output, error) {
		if opened == nil {
			return coretool.Output{}, fmt.Errorf("mcp opened tools is nil")
		}
		result, err := opened.CallTool(ctx, definition.RawName, map[string]any(input))
		if err != nil {
			return coretool.Output{}, err
		}
		return outputFromResult(result), nil
	})
}

func schemaFromAny(value any) coretool.ParametersSchema {
	if value == nil {
		return coretool.NewParametersSchema(nil)
	}
	data, err := json.Marshal(value)
	if err != nil {
		return coretool.NewParametersSchema(nil)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return coretool.NewParametersSchema(nil)
	}
	return coretool.NewParametersSchema(raw)
}

func outputFromResult(result *coremcp.CallResult) coretool.Output {
	if result == nil {
		return coretool.Output{}
	}
	parts := make([]coretool.Part, 0, len(result.Content))
	texts := make([]string, 0, len(result.Content)+1)
	for _, item := range result.Content {
		part := coretool.Part{Type: item.Type, Text: item.Text, Data: item.Data, MIME: item.MIMEType}
		switch item.Type {
		case "text":
			texts = appendNonEmpty(texts, item.Text)
		case "image", "audio":
			texts = append(texts, fmt.Sprintf("[MCP %s content: %s]", item.Type, item.MIMEType))
		default:
			if len(item.Raw) > 0 {
				part.Text = string(item.Raw)
				texts = append(texts, string(item.Raw))
			} else if item.URI != "" {
				part.Text = item.URI
				texts = append(texts, item.URI)
			}
		}
		parts = append(parts, part)
	}
	metadata := map[string]any{"mcp_is_error": result.IsError}
	if result.StructuredContent != nil {
		metadata["structured_content"] = result.StructuredContent
		if data, err := json.Marshal(result.StructuredContent); err == nil {
			texts = append(texts, string(data))
		}
	}
	if result.IsError {
		metadata["error"] = strings.Join(texts, "\n")
	}
	return coretool.Output{
		Text:     strings.Join(texts, "\n"),
		Parts:    parts,
		Metadata: metadata,
	}
}

func appendNonEmpty(values []string, value string) []string {
	if value == "" {
		return values
	}
	return append(values, value)
}
