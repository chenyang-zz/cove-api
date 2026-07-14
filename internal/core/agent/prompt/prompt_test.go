// Package prompt 验证 Agent prompt 的定义边界。
//
// 本文件确保 Agent prompt 只暴露模板文件、模板名和数据结构；模板读取和渲染必须通过
// core/prompt 完成。业务人设由调用方拼在协议段之前，模板本身不包含 SystemPrompt。
package prompt

import (
	"strings"
	"testing"

	coreprompt "github.com/boxify/api-go/internal/core/prompt"
)

// 验证点：ReAct 系统模板只渲染工具列表与协议，不含业务人设。
func TestReActSystemTemplateRendersProtocolOnly(t *testing.T) {
	out, err := coreprompt.Render(Templates, ReActSystemTemplate, ReActSystemData{
		Tools: []ReActToolData{
			{Name: "knowledge_search", Description: "检索知识库"},
			{Name: "current_time", Description: "获取当前时间"},
		},
	})
	if err != nil {
		t.Fatalf("core prompt Render(react system) error = %v, want nil", err)
	}
	for _, want := range []string{"- knowledge_search：检索知识库", "- current_time：获取当前时间", "Final Answer", "可用工具"} {
		if !strings.Contains(out, want) {
			t.Fatalf("rendered react system prompt = %q, want to contain %q", out, want)
		}
	}
	if strings.Contains(out, "可用工具：-") {
		t.Fatalf("rendered react system prompt = %q, want tool list on a new line", out)
	}
	if strings.Contains(out, "{{") || strings.Contains(out, "}}") {
		t.Fatalf("rendered react system prompt = %q, want executed template", out)
	}
	for _, forbidden := range []string{"Cove", "彗记", "# Soul"} {
		if strings.Contains(out, forbidden) {
			t.Fatalf("rendered protocol = %q, want no business identity %q", out, forbidden)
		}
	}
}

// 验证点：ReAct 系统模板原文应可由 core/prompt 读取，且不含 SystemPrompt 占位。
func TestReActSystemTemplateTextCanBeReadByCorePrompt(t *testing.T) {
	out, err := coreprompt.TemplateText(Templates, ReActSystemTemplate)
	if err != nil {
		t.Fatalf("TemplateText(react system) error = %v, want nil", err)
	}
	if !strings.Contains(out, "{{ range .Tools }}") {
		t.Fatalf("TemplateText(react system) = %q, want tools range placeholder", out)
	}
	if strings.Contains(out, "SystemPrompt") {
		t.Fatalf("TemplateText(react system) = %q, want protocol-only template without SystemPrompt", out)
	}
	for _, forbidden := range []string{"彗记", "Cove", "AssistantIntro"} {
		if strings.Contains(out, forbidden) {
			t.Fatalf("TemplateText(react system) = %q, want no business brand or AssistantIntro marker %q", out, forbidden)
		}
	}
}
