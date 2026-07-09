// Package prompts 验证业务提示词的集中注册行为。
package prompts

import (
	"strings"
	"testing"

	coreprompt "github.com/boxify/api-go/internal/core/prompt"
)

// 验证点：Register 应把平铺模板注册为现有 agent 和 memory 逻辑名称。
func TestRegisterMakesFlatTemplatesAvailableByLogicalName(t *testing.T) {
	manager := coreprompt.NewManager()
	if err := Register(manager); err != nil {
		t.Fatalf("Register() error = %v, want nil", err)
	}

	cases := []struct {
		name   string
		marker string
	}{
		{name: "agent/optimize_prompt", marker: "RawPrompt"},
		{name: "memory/statement_extract", marker: "Content"},
		{name: "memory/triplet_extract", marker: "Statement"},
		{name: "memory/dedup_entity", marker: "EntityA"},
		{name: "memory/generate_community_metadata", marker: "Members"},
	}
	for _, tc := range cases {
		text, err := manager.TemplateText(tc.name)
		if err != nil {
			t.Fatalf("TemplateText(%s) error = %v, want nil", tc.name, err)
		}
		if !strings.Contains(text, tc.marker) {
			t.Fatalf("TemplateText(%s) = %q, want marker %q", tc.name, text, tc.marker)
		}
	}
}

// 验证点：Register 接收 nil Manager 时应返回清晰错误而不是 panic。
func TestRegisterRejectsNilManager(t *testing.T) {
	err := Register(nil)
	if err == nil || !strings.Contains(err.Error(), "prompt manager") {
		t.Fatalf("Register(nil) error = %v, want prompt manager error", err)
	}
}

// TestRegisterExcludesCorePrompts 验证外部注册表不会注册由 core/agent 自己 embed 的 ReAct 模板。
func TestRegisterExcludesCorePrompts(t *testing.T) {
	manager := coreprompt.NewManager()
	if err := Register(manager); err != nil {
		t.Fatalf("Register error = %v, want nil", err)
	}

	if _, err := manager.TemplateText("agent/react_system"); err == nil {
		t.Fatal("TemplateText(agent/react_system) error = nil, want unregistered core prompt")
	}
}
