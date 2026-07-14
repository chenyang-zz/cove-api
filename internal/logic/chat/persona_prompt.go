package chat

import (
	"strings"
)

// coveAssistantIntro 在没有有效人格信息时作为默认系统身份注入。
const coveAssistantIntro = "你是「Cove」的智能助手。你可以调用以下工具来帮助回答用户的问题。"

// composePersonaPrompt 按 Soul → Identity 组装人设系统提示片段。
// 空段省略；各段以 "# Soul" / "# Identity" 开头，段间以 "\n\n" 分隔。
// 若正文以一级 Markdown 标题开头（如 "# Soul"），会先剥离该行再包标题。
func composePersonaPrompt(soul, identity string) string {
	sections := make([]string, 0, 2)
	if body := stripLeadingATXHeading(soul); body != "" {
		sections = append(sections, "# Soul\n"+body)
	}
	if body := stripLeadingATXHeading(identity); body != "" {
		sections = append(sections, "# Identity\n"+body)
	}
	return strings.Join(sections, "\n\n")
}

// stripLeadingATXHeading 去掉正文开头的一级 Markdown 标题行（# Title，不含 ##）。
func stripLeadingATXHeading(content string) string {
	content = strings.TrimSpace(content)
	if content == "" {
		return ""
	}
	firstLine, rest, hasRest := strings.Cut(content, "\n")
	firstLine = strings.TrimRight(firstLine, "\r")
	if isATXH1(firstLine) {
		if !hasRest {
			return ""
		}
		return strings.TrimSpace(rest)
	}
	return content
}

// isATXH1 reports whether line is a level-1 ATX heading (# Title), not ## or bare #.
func isATXH1(line string) bool {
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, "#") {
		return false
	}
	if strings.HasPrefix(line, "##") {
		return false
	}
	// "# " or "#Title" both count as H1 after a single hash.
	if len(line) == 1 {
		return false
	}
	// Must be "#" followed by space or non-# content (standard ATX allows "#Title").
	return line[1] == ' ' || line[1] != '#'
}

// joinSystemPromptParts 用空行连接非空系统提示片段。
func joinSystemPromptParts(parts ...string) string {
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return strings.Join(out, "\n\n")
}

// buildChatSystemPrompt 合并人格与 AgentConfig 提示；仅在无人格信息时注入 coveAssistantIntro。
func buildChatSystemPrompt(soul, identity, agentConfigPrompt string) string {
	personaBlock := composePersonaPrompt(soul, identity)
	if personaBlock != "" {
		return joinSystemPromptParts(personaBlock, agentConfigPrompt)
	}
	return joinSystemPromptParts(coveAssistantIntro, agentConfigPrompt)
}
