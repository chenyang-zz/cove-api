// Package prompt 验证 Agent prompt 的定义边界。
//
// 本文件确保 Agent prompt 只暴露模板文件、模板名和数据结构；模板读取和渲染必须通过
// core/prompt 完成。
package prompt

import (
	"strings"
	"testing"

	coreprompt "github.com/boxify/api-go/internal/core/prompt"
)

// 验证点：ReAct 系统模板应可由 core/prompt 渲染业务通过 SystemPrompt 注入的信息、工具名称和描述。
func TestReActSystemTemplateCanBeRenderedByCorePrompt(t *testing.T) {
	out, err := coreprompt.Render(Templates, ReActSystemTemplate, ReActSystemData{
		Tools: []ReActToolData{
			{Name: "knowledge_search", Description: "检索知识库"},
			{Name: "current_time", Description: "获取当前时间"},
		},
		SystemPrompt: "你是「Cove」的智能助手。\n\n回答要简洁。",
	})
	if err != nil {
		t.Fatalf("core prompt Render(react system) error = %v, want nil", err)
	}
	for _, want := range []string{"Cove", "- knowledge_search：检索知识库", "- current_time：获取当前时间", "回答要简洁。"} {
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
}

// 验证点：ReAct 系统模板不传 SystemPrompt 时不应包含业务品牌身份。
func TestReActSystemTemplateOmitsBusinessIdentityWhenSystemPromptIsEmpty(t *testing.T) {
	out, err := coreprompt.Render(Templates, ReActSystemTemplate, ReActSystemData{
		Tools: []ReActToolData{{Name: "current_time", Description: "获取当前时间"}},
	})
	if err != nil {
		t.Fatalf("core prompt Render(react system without intro) error = %v, want nil", err)
	}
	for _, forbidden := range []string{"彗记", "Cove", "你是「"} {
		if strings.Contains(out, forbidden) {
			t.Fatalf("rendered react system prompt = %q, want no business identity containing %q", out, forbidden)
		}
	}
}

// 验证点：ReAct 系统模板在没有附加人设时不应渲染附加段落。
func TestReActSystemTemplateOmitsEmptySystemPromptSection(t *testing.T) {
	out, err := coreprompt.Render(Templates, ReActSystemTemplate, ReActSystemData{
		Tools: []ReActToolData{{Name: "current_time", Description: "获取当前时间"}},
	})
	if err != nil {
		t.Fatalf("core prompt Render(react system without system prompt) error = %v, want nil", err)
	}
	if strings.Contains(out, "附加人设/风格要求") {
		t.Fatalf("rendered react system prompt = %q, want no extra persona section", out)
	}
}

// 验证点：ReAct 系统模板原文应可由 core/prompt 读取，agent/prompt 不承担解析职责。
func TestReActSystemTemplateTextCanBeReadByCorePrompt(t *testing.T) {
	out, err := coreprompt.TemplateText(Templates, ReActSystemTemplate)
	if err != nil {
		t.Fatalf("TemplateText(react system) error = %v, want nil", err)
	}
	if !strings.Contains(out, "{{ .SystemPrompt }}") || !strings.Contains(out, "{{ range .Tools }}") {
		t.Fatalf("TemplateText(react system) = %q, want raw Go template placeholders", out)
	}
	for _, forbidden := range []string{"彗记", "Cove", "AssistantIntro"} {
		if strings.Contains(out, forbidden) {
			t.Fatalf("TemplateText(react system) = %q, want no business brand or AssistantIntro marker %q", out, forbidden)
		}
	}
}
