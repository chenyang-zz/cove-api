package tool

import (
	"context"
	"reflect"
	"strings"
	"testing"
)

// 验证点：静态工具集应返回注册时的描述和工具列表，并避免调用方修改内部切片。
func TestStaticSetDescribesAndReturnsTools(t *testing.T) {
	ctx := context.Background()
	tools := []Tool{newTextTool("search", "search ok"), newTextTool("summarize", "summarize ok")}
	set := NewStaticSet(SetDescriptor{
		Name:        "rag",
		Description: "RAG tools.",
		Tags:        []string{"rag", "knowledge"},
		Annotations: map[string]any{
			"owner": "core",
		},
	}, tools...)

	gotDesc, err := set.Describe(ctx)
	if err != nil {
		t.Fatalf("StaticSet.Describe() error = %v, want nil", err)
	}
	if gotDesc.Name != "rag" || gotDesc.Description != "RAG tools." {
		t.Fatalf("StaticSet.Describe() = %#v, want rag descriptor", gotDesc)
	}
	if !reflect.DeepEqual(gotDesc.Tags, []string{"rag", "knowledge"}) {
		t.Fatalf("StaticSet.Describe().Tags = %#v, want rag and knowledge", gotDesc.Tags)
	}
	if gotDesc.Annotations["owner"] != "core" {
		t.Fatalf("StaticSet.Describe().Annotations[owner] = %#v, want core", gotDesc.Annotations["owner"])
	}

	gotTools, err := set.Tools(ctx)
	if err != nil {
		t.Fatalf("StaticSet.Tools() error = %v, want nil", err)
	}
	if len(gotTools) != 2 {
		t.Fatalf("StaticSet.Tools() len = %d, want 2", len(gotTools))
	}
	gotTools[0] = newTextTool("mutated", "mutated")

	gotToolsAgain, err := set.Tools(ctx)
	if err != nil {
		t.Fatalf("StaticSet.Tools() second call error = %v, want nil", err)
	}
	gotToolDesc, err := gotToolsAgain[0].Describe(ctx)
	if err != nil {
		t.Fatalf("Tool.Describe() error = %v, want nil", err)
	}
	if gotToolDesc.Name != "search" {
		t.Fatalf("StaticSet.Tools() first tool name = %q, want search", gotToolDesc.Name)
	}
}

// 验证点：Catalog 注册工具集时应拒绝 nil、空名称和重复名称。
func TestCatalogRegisterSetRejectsInvalidSets(t *testing.T) {
	ctx := context.Background()
	catalog := NewCatalog()

	if err := catalog.RegisterSet(ctx, nil); err == nil {
		t.Fatalf("Catalog.RegisterSet(nil) error = nil, want error")
	}

	emptyNameSet := NewStaticSet(SetDescriptor{Name: " "}, newTextTool("noop", "noop"))
	if err := catalog.RegisterSet(ctx, emptyNameSet); err == nil {
		t.Fatalf("Catalog.RegisterSet(empty name) error = nil, want error")
	}

	first := NewStaticSet(SetDescriptor{Name: "rag"}, newTextTool("rag_search", "ok"))
	if err := catalog.RegisterSet(ctx, first); err != nil {
		t.Fatalf("Catalog.RegisterSet(first) error = %v, want nil", err)
	}

	duplicate := NewStaticSet(SetDescriptor{Name: "rag"}, newTextTool("rag_summary", "ok"))
	if err := catalog.RegisterSet(ctx, duplicate); err == nil {
		t.Fatalf("Catalog.RegisterSet(duplicate) error = nil, want error")
	}
}

// 验证点：Catalog 应按工具集名称稳定返回工具集描述。
func TestCatalogListSetsReturnsStableDescriptors(t *testing.T) {
	ctx := context.Background()
	catalog := NewCatalog()
	for _, name := range []string{"memory", "rag", "document"} {
		if err := catalog.RegisterSet(ctx, NewStaticSet(SetDescriptor{Name: name}, newTextTool(name+"_tool", "ok"))); err != nil {
			t.Fatalf("Catalog.RegisterSet(%q) error = %v, want nil", name, err)
		}
	}

	got, err := catalog.ListSets(ctx)
	if err != nil {
		t.Fatalf("Catalog.ListSets() error = %v, want nil", err)
	}
	gotNames := setDescriptorNames(got)
	wantNames := []string{"document", "memory", "rag"}
	if !reflect.DeepEqual(gotNames, wantNames) {
		t.Fatalf("Catalog.ListSets() names = %#v, want %#v", gotNames, wantNames)
	}
}

// 验证点：空选择条件应展开全部工具集，并生成可被 Runner 调用的扁平 Registry。
func TestCatalogBuildRegistryIncludesAllSetsByDefault(t *testing.T) {
	ctx := context.Background()
	catalog := NewCatalog()
	if err := catalog.RegisterSet(ctx, NewStaticSet(SetDescriptor{Name: "rag"}, newTextTool("rag_search", "rag ok"))); err != nil {
		t.Fatalf("Catalog.RegisterSet(rag) error = %v, want nil", err)
	}
	if err := catalog.RegisterSet(ctx, NewStaticSet(SetDescriptor{Name: "memory"}, newTextTool("memory_lookup", "memory ok"))); err != nil {
		t.Fatalf("Catalog.RegisterSet(memory) error = %v, want nil", err)
	}

	registry, err := catalog.BuildRegistry(ctx, Selection{})
	if err != nil {
		t.Fatalf("Catalog.BuildRegistry(empty selection) error = %v, want nil", err)
	}
	gotNames := descriptorNames(registry.List(nil))
	wantNames := []string{"memory_lookup", "rag_search"}
	if !reflect.DeepEqual(gotNames, wantNames) {
		t.Fatalf("Catalog.BuildRegistry(empty selection) tools = %#v, want %#v", gotNames, wantNames)
	}

	output, err := NewRunner(registry).Invoke(ctx, "memory_lookup", nil)
	if err != nil {
		t.Fatalf("Runner.Invoke(memory_lookup) error = %v, want nil", err)
	}
	if output.Text != "memory ok" {
		t.Fatalf("Runner.Invoke(memory_lookup).Text = %q, want memory ok", output.Text)
	}
}

// 验证点：Selection.SetNames 应只展开指定工具集。
func TestCatalogBuildRegistryFiltersBySetNames(t *testing.T) {
	ctx := context.Background()
	catalog := NewCatalog()
	if err := catalog.RegisterSet(ctx, NewStaticSet(SetDescriptor{Name: "rag"}, newTextTool("rag_search", "rag ok"))); err != nil {
		t.Fatalf("Catalog.RegisterSet(rag) error = %v, want nil", err)
	}
	if err := catalog.RegisterSet(ctx, NewStaticSet(SetDescriptor{Name: "memory"}, newTextTool("memory_lookup", "memory ok"))); err != nil {
		t.Fatalf("Catalog.RegisterSet(memory) error = %v, want nil", err)
	}

	registry, err := catalog.BuildRegistry(ctx, Selection{SetNames: []string{"rag"}})
	if err != nil {
		t.Fatalf("Catalog.BuildRegistry(set rag) error = %v, want nil", err)
	}
	gotNames := descriptorNames(registry.List(nil))
	wantNames := []string{"rag_search"}
	if !reflect.DeepEqual(gotNames, wantNames) {
		t.Fatalf("Catalog.BuildRegistry(set rag) tools = %#v, want %#v", gotNames, wantNames)
	}
}

// 验证点：Selection.ToolNames 应只保留指定工具。
func TestCatalogBuildRegistryFiltersByToolNames(t *testing.T) {
	ctx := context.Background()
	catalog := NewCatalog()
	if err := catalog.RegisterSet(ctx, NewStaticSet(
		SetDescriptor{Name: "rag"},
		newTextTool("rag_search", "search ok"),
		newTextTool("rag_summary", "summary ok"),
	)); err != nil {
		t.Fatalf("Catalog.RegisterSet(rag) error = %v, want nil", err)
	}

	registry, err := catalog.BuildRegistry(ctx, Selection{ToolNames: []string{"rag_summary"}})
	if err != nil {
		t.Fatalf("Catalog.BuildRegistry(tool rag_summary) error = %v, want nil", err)
	}
	gotNames := descriptorNames(registry.List(nil))
	wantNames := []string{"rag_summary"}
	if !reflect.DeepEqual(gotNames, wantNames) {
		t.Fatalf("Catalog.BuildRegistry(tool rag_summary) tools = %#v, want %#v", gotNames, wantNames)
	}
}

// 验证点：多个工具集展开后出现同名工具时应返回错误，避免调用目标歧义。
func TestCatalogBuildRegistryRejectsDuplicateToolNames(t *testing.T) {
	ctx := context.Background()
	catalog := NewCatalog()
	if err := catalog.RegisterSet(ctx, NewStaticSet(SetDescriptor{Name: "rag"}, newTextTool("search", "rag search"))); err != nil {
		t.Fatalf("Catalog.RegisterSet(rag) error = %v, want nil", err)
	}
	if err := catalog.RegisterSet(ctx, NewStaticSet(SetDescriptor{Name: "memory"}, newTextTool("search", "memory search"))); err != nil {
		t.Fatalf("Catalog.RegisterSet(memory) error = %v, want nil", err)
	}

	_, err := catalog.BuildRegistry(ctx, Selection{})
	if err == nil {
		t.Fatalf("Catalog.BuildRegistry(duplicate tool names) error = nil, want duplicate error")
	}
	if !strings.Contains(err.Error(), "already registered") {
		t.Fatalf("Catalog.BuildRegistry(duplicate tool names) error = %q, want already registered", err.Error())
	}
}

func newTextTool(name string, text string) Tool {
	return NewFuncTool(Descriptor{Name: name}, func(ctx context.Context, input Input) (Output, error) {
		return Output{Text: text}, nil
	})
}

func setDescriptorNames(descriptors []SetDescriptor) []string {
	names := make([]string, 0, len(descriptors))
	for _, descriptor := range descriptors {
		names = append(names, descriptor.Name)
	}
	return names
}
