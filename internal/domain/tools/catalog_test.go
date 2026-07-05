package tools

import (
	"context"
	"reflect"
	"sort"
	"testing"
	"time"

	coretool "github.com/boxify/api-go/internal/core/tool"
	"github.com/boxify/api-go/internal/domain/tools/builtin"
	"github.com/boxify/api-go/internal/svc"
)

// 验证 NewCatalog 缺少 ServiceContext 时返回错误，避免误以为知识库工具可用。
func TestNewCatalogRequiresServiceContext(t *testing.T) {
	if _, err := NewCatalog(nil); err == nil {
		t.Fatal("NewCatalog(nil) error = nil, want error")
	}
}

// 验证 NewCatalog 会注册 system 和 knowledge 工具集，作为领域层工具的统一入口。
func TestNewCatalogRegistersToolSets(t *testing.T) {
	ctx := context.Background()

	catalog, err := NewCatalog(&svc.ServiceContext{})
	if err != nil {
		t.Fatalf("NewCatalog() error = %v, want nil", err)
	}
	sets, err := catalog.ListSets(ctx)
	if err != nil {
		t.Fatalf("Catalog.ListSets() error = %v, want nil", err)
	}

	gotNames := make([]string, 0, len(sets))
	for _, set := range sets {
		gotNames = append(gotNames, set.Name)
	}
	sort.Strings(gotNames)
	wantNames := []string{ToolSetKnowledge, ToolSetSystem}
	sort.Strings(wantNames)
	if !reflect.DeepEqual(gotNames, wantNames) {
		t.Fatalf("Catalog.ListSets() names = %#v, want %#v", gotNames, wantNames)
	}
}

// 验证 NewCatalog 展开的注册表可以按名称调用 current_time 工具。
func TestNewCatalogBuildRegistryInvokesCurrentTime(t *testing.T) {
	ctx := context.Background()
	fixed := mustParseTime(t, "2026-07-05T10:11:12Z")
	catalog, err := NewCatalog(&svc.ServiceContext{}, builtin.WithClock(func() time.Time {
		return fixed
	}))
	if err != nil {
		t.Fatalf("NewCatalog() error = %v, want nil", err)
	}

	registry, err := catalog.BuildRegistry(ctx, coretool.Selection{ToolNames: []string{ToolCurrentTime}})
	if err != nil {
		t.Fatalf("Catalog.BuildRegistry(current_time) error = %v, want nil", err)
	}
	output, err := coretool.NewRunner(registry).Invoke(ctx, ToolCurrentTime, nil)
	if err != nil {
		t.Fatalf("Runner.Invoke(current_time) error = %v, want nil", err)
	}
	if output.Text != "2026-07-05T10:11:12Z" {
		t.Fatalf("Runner.Invoke(current_time).Text = %q, want 2026-07-05T10:11:12Z", output.Text)
	}
}

// 验证 NewCatalog 展开的注册表可以按名称调用 knowledge_search 工具。
func TestNewCatalogBuildRegistryInvokesKnowledgeSearch(t *testing.T) {
	catalog, err := NewCatalog(&svc.ServiceContext{})
	if err != nil {
		t.Fatalf("NewCatalog() error = %v, want nil", err)
	}
	registry, err := catalog.BuildRegistry(context.Background(), coretool.Selection{ToolNames: []string{ToolKnowledgeSearch}})
	if err != nil {
		t.Fatalf("Catalog.BuildRegistry(knowledge_search) error = %v, want nil", err)
	}
	tool, ok := registry.Lookup(ToolKnowledgeSearch)
	if !ok {
		t.Fatalf("Registry.Lookup(knowledge_search) ok = false, want true")
	}
	descriptor, err := tool.Describe(context.Background())
	if err != nil {
		t.Fatalf("Tool.Describe(knowledge_search) error = %v, want nil", err)
	}
	if descriptor.Name != ToolKnowledgeSearch {
		t.Fatalf("Tool.Describe(knowledge_search).Name = %q, want %q", descriptor.Name, ToolKnowledgeSearch)
	}
}

func mustParseTime(t *testing.T, value string) time.Time {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		t.Fatalf("time.Parse(%q) error = %v, want nil", value, err)
	}
	return parsed
}
