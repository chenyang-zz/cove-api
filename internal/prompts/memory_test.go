package prompts

import (
	"strings"
	"testing"

	"github.com/boxify/api-go/internal/core/memory"
	coreprompt "github.com/boxify/api-go/internal/core/prompt"
	"github.com/boxify/api-go/internal/prompts/promptsgen"
)

// TestMemoryPrompterConvertsDomainInput 验证 Memory adapter 会把领域数值和实体数据转换为生成参数并完成渲染。
func TestMemoryPrompterConvertsDomainInput(t *testing.T) {
	manager := coreprompt.NewManager()
	if err := Register(manager); err != nil {
		t.Fatalf("Register error = %v, want nil", err)
	}
	prompter := NewMemoryPrompter(promptsgen.NewClient(manager))

	text, err := prompter.DedupEntity(&memory.DedupPromptInput{
		EntityA: memory.DedupEntityPromptInput{Name: "Cove", Aliases: []string{"cove"}},
		EntityB: memory.DedupEntityPromptInput{Name: "COVE"},
		Context: memory.DedupPromptContext{
			NameTextSim:  0.9,
			NameEmbedSim: 0.8,
			NameContains: true,
		},
	})
	if err != nil {
		t.Fatalf("MemoryPrompter.DedupEntity error = %v, want nil", err)
	}
	for _, want := range []string{"Cove", "cove", "0.9", "0.8", "true"} {
		if !strings.Contains(text, want) {
			t.Fatalf("MemoryPrompter.DedupEntity output missing %q:\n%s", want, text)
		}
	}
}
