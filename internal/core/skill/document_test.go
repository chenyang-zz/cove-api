package skill

import (
	"strings"
	"testing"
	"testing/fstest"
)

// 验证 Parse 能解析带 YAML front matter 的 Markdown skill 文档。
func TestParseSkillDocumentWithFrontMatter(t *testing.T) {
	raw := `---
name: kb_study
description: 基于知识库学习
version: 1.0.0
icon: 📚
title: 知识库学习
tags:
  - builtin
  - " "
  - knowledge
tools:
  - knowledge_search
requirements:
  env:
    - OPENAI_API_KEY
  binaries:
    - pandoc
  os:
    - darwin
  custom: value
---
# 指令

请根据知识库回答。`

	document, err := Parse(raw)
	if err != nil {
		t.Fatalf("Parse() error = %v, want nil", err)
	}
	if document.Metadata.Name != "kb_study" || document.Metadata.Description != "基于知识库学习" || document.Metadata.Version != "1.0.0" || document.Metadata.Icon != "📚" {
		t.Fatalf("Parse().Metadata = %#v, want parsed base fields", document.Metadata)
	}
	if got := strings.Join(document.Metadata.Tags, ","); got != "builtin,knowledge" {
		t.Fatalf("Parse().Metadata.Tags = %q, want builtin,knowledge", got)
	}
	if got := strings.Join(document.Metadata.Tools, ","); got != "knowledge_search" {
		t.Fatalf("Parse().Metadata.Tools = %q, want knowledge_search", got)
	}
	if document.Metadata.Annotations["title"] != "知识库学习" {
		t.Fatalf("Parse().Metadata.Annotations[title] = %#v, want 知识库学习", document.Metadata.Annotations["title"])
	}
	if document.Metadata.Requirements.Env[0] != "OPENAI_API_KEY" || document.Metadata.Requirements.Binaries[0] != "pandoc" || document.Metadata.Requirements.OS[0] != "darwin" {
		t.Fatalf("Parse().Metadata.Requirements = %#v, want parsed requirements", document.Metadata.Requirements)
	}
	if document.Metadata.Requirements.Annotations["custom"] != "value" {
		t.Fatalf("Parse().Metadata.Requirements.Annotations = %#v, want custom value", document.Metadata.Requirements.Annotations)
	}
	if document.Body != "# 指令\n\n请根据知识库回答。" || document.Raw != raw {
		t.Fatalf("Parse() body/raw = %q/%t, want markdown body and raw preserved", document.Body, document.Raw == raw)
	}
}

// 验证 Parse 会拒绝缺失 front matter、未闭合 front matter 和非法 YAML。
func TestParseSkillDocumentRejectsInvalidFrontMatter(t *testing.T) {
	cases := []struct {
		name string
		raw  string
	}{
		{name: "missing", raw: "# skill"},
		{name: "unclosed", raw: "---\nname: test\n"},
		{name: "invalid yaml", raw: "---\nname: [\n---\nbody"},
	}
	for _, tc := range cases {
		if _, err := Parse(tc.raw); err == nil {
			t.Fatalf("Parse(%s) error = nil, want error", tc.name)
		}
	}
}

// 验证 Parse 会校验必要元数据，并拒绝错误类型的字段。
func TestParseSkillDocumentValidatesMetadata(t *testing.T) {
	cases := []struct {
		name string
		raw  string
	}{
		{name: "missing name", raw: "---\ndescription: desc\n---\nbody"},
		{name: "missing description", raw: "---\nname: test\n---\nbody"},
		{name: "tags wrong type", raw: "---\nname: test\ndescription: desc\ntags: invalid\n---\nbody"},
		{name: "tool item wrong type", raw: "---\nname: test\ndescription: desc\ntools:\n  - 1\n---\nbody"},
		{name: "requirements wrong type", raw: "---\nname: test\ndescription: desc\nrequirements: invalid\n---\nbody"},
	}
	for _, tc := range cases {
		if _, err := Parse(tc.raw); err == nil {
			t.Fatalf("Parse(%s) error = nil, want error", tc.name)
		}
	}
}

// 验证 Read 会通过 fs.FS 读取并解析 skill 文档。
func TestReadSkillDocumentFromFS(t *testing.T) {
	fsys := fstest.MapFS{
		"skills/kb/SKILL.md": &fstest.MapFile{Data: []byte("---\nname: kb\ndescription: desc\n---\nbody")},
	}

	document, err := Read(fsys, "skills/kb/SKILL.md")
	if err != nil {
		t.Fatalf("Read() error = %v, want nil", err)
	}
	if document.Metadata.Name != "kb" || document.Body != "body" {
		t.Fatalf("Read() = %#v, want parsed document", document)
	}
}
