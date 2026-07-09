package skills

import (
	"reflect"
	"testing"
)

// 验证 NewRegistry 注册参考项目中的三个内置技能模板。
func TestNewRegistryRegistersBuiltinSkillTemplates(t *testing.T) {
	registry, err := NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry() error = %v, want nil", err)
	}

	items := registry.List()
	if len(items) != 3 {
		t.Fatalf("Registry.List() len = %d, want 3", len(items))
	}
	if items[0].ID != IDKBStudy || items[0].Key != KeyKBStudy || items[0].Name != "知识库学习" {
		t.Fatalf("first builtin skill = %#v, want kb_study", items[0])
	}
	if !reflect.DeepEqual(items[0].ToolKeys, []string{ToolKnowledgeSearch}) {
		t.Fatalf("kb_study ToolKeys = %#v, want knowledge_search", items[0].ToolKeys)
	}
	if len(items[0].Config.QuickPrompt) != 4 || items[0].Config.QuickPrompt[0] != "帮我梳理知识库里的核心知识点" {
		t.Fatalf("kb_study quick prompts = %#v, want reference prompts", items[0].Config.QuickPrompt)
	}
	if items[0].Config.FewShots == nil || len(items[0].Config.FewShots) != 0 {
		t.Fatalf("kb_study few shots = %#v, want configured empty slice", items[0].Config.FewShots)
	}
	if items[1].ID != IDStockAnalysis || items[1].ToolKeys[0] != ToolWebSearch {
		t.Fatalf("stock_analysis = %#v, want web_search template", items[1])
	}
	if items[2].ID != IDTranslatePolish || len(items[2].ToolKeys) != 0 {
		t.Fatalf("translate_polish = %#v, want no tool keys", items[2])
	}
}

// 验证返回的模板副本被修改后不会污染内置注册表。
func TestNewRegistryReturnsIndependentTemplates(t *testing.T) {
	registry, err := NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry() error = %v, want nil", err)
	}

	item, ok := registry.LookupByKey(KeyKBStudy)
	if !ok {
		t.Fatalf("Registry.LookupByKey(%q) ok = false, want true", KeyKBStudy)
	}
	item.ToolKeys[0] = "mutated"
	item.Config.QuickPrompt[0] = "mutated"
	item.Config.FewShots = append(item.Config.FewShots, FewShot{Input: "mutated", Output: "mutated"})

	again, ok := registry.LookupByKey(KeyKBStudy)
	if !ok {
		t.Fatalf("Registry.LookupByKey(%q) second ok = false, want true", KeyKBStudy)
	}
	if !reflect.DeepEqual(again.ToolKeys, []string{ToolKnowledgeSearch}) || again.Config.QuickPrompt[0] != "帮我梳理知识库里的核心知识点" || len(again.Config.FewShots) != 0 {
		t.Fatalf("builtin definition was mutated = %#v", again)
	}
}
