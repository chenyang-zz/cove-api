package chat

import (
	"strings"
	"testing"
)

// 验证 composePersonaPrompt 按 Soul → Identity 组装并带标准标题。
func TestComposePersonaPromptOrdersSoulThenIdentity(t *testing.T) {
	got := composePersonaPrompt("温暖、简洁", "你是小盒")
	want := "# Soul\n温暖、简洁\n\n# Identity\n你是小盒"
	if got != want {
		t.Fatalf("composePersonaPrompt = %q, want %q", got, want)
	}
}

// 验证仅一段非空时只输出对应章节。
func TestComposePersonaPromptOmitsEmptySections(t *testing.T) {
	if got := composePersonaPrompt("only soul", ""); got != "# Soul\nonly soul" {
		t.Fatalf("soul only = %q", got)
	}
	if got := composePersonaPrompt("", "only identity"); got != "# Identity\nonly identity" {
		t.Fatalf("identity only = %q", got)
	}
	if got := composePersonaPrompt("  ", "\n"); got != "" {
		t.Fatalf("all empty = %q, want empty", got)
	}
}

// 验证正文以一级标题开头时剥离，避免重复 # Soul / # Identity。
func TestComposePersonaPromptStripsLeadingH1(t *testing.T) {
	got := composePersonaPrompt("# Soul\n温暖、简洁", "# Identity\n你是小盒")
	want := "# Soul\n温暖、简洁\n\n# Identity\n你是小盒"
	if got != want {
		t.Fatalf("composePersonaPrompt strip H1 = %q, want %q", got, want)
	}
	// ## 不是一级标题，保留
	got = composePersonaPrompt("## Soul\n内文", "")
	if !strings.HasPrefix(got, "# Soul\n## Soul\n内文") {
		t.Fatalf("composePersonaPrompt keep ## = %q", got)
	}
}

// 验证 buildChatSystemPrompt：有人格时不注入 Cove intro，无人格时注入。
func TestBuildChatSystemPromptPersonaVsCoveIntro(t *testing.T) {
	withPersona := buildChatSystemPrompt("soul body", "id body", "extra")
	if strings.Contains(withPersona, "Cove") {
		t.Fatalf("with persona should not inject Cove intro: %q", withPersona)
	}
	if !strings.Contains(withPersona, "# Soul\nsoul body") || !strings.Contains(withPersona, "# Identity\nid body") {
		t.Fatalf("with persona missing sections: %q", withPersona)
	}
	if !strings.Contains(withPersona, "extra") {
		t.Fatalf("with persona missing agent config prompt: %q", withPersona)
	}

	noPersona := buildChatSystemPrompt("", "", "cfg")
	if !strings.Contains(noPersona, coveAssistantIntro) || !strings.Contains(noPersona, "cfg") {
		t.Fatalf("no persona = %q, want Cove intro + cfg", noPersona)
	}

	emptyPersonaFields := buildChatSystemPrompt("  ", "", "")
	if emptyPersonaFields != coveAssistantIntro {
		t.Fatalf("empty persona fields = %q, want only Cove intro", emptyPersonaFields)
	}
}
