package skill

import (
	"errors"
	"fmt"
	"io/fs"
	"strings"

	"gopkg.in/yaml.v3"
)

var knownMetadataKeys = map[string]struct{}{
	"name":         {},
	"description":  {},
	"version":      {},
	"icon":         {},
	"tags":         {},
	"tools":        {},
	"requirements": {},
}

var knownRequirementKeys = map[string]struct{}{
	"env":      {},
	"binaries": {},
	"os":       {},
}

// Parse 解析 Markdown skill 文档。
//
// 文档必须以 YAML front matter 开头，且 front matter 至少包含非空 name 和
// description。正文会作为 Markdown 原文保留，core/skill 不解释其中的业务语义。
func Parse(raw string) (*Document, error) {
	header, body, err := splitFrontMatter(raw)
	if err != nil {
		return nil, err
	}
	metadata, err := parseMetadata(header)
	if err != nil {
		return nil, err
	}
	return &Document{
		Metadata: metadata,
		Body:     strings.TrimSpace(body),
		Raw:      raw,
	}, nil
}

// Read 从 fsys 中读取并解析 Markdown skill 文档。
func Read(fsys fs.FS, name string) (*Document, error) {
	if fsys == nil {
		return nil, errors.New("skill document fs is nil")
	}
	data, err := fs.ReadFile(fsys, name)
	if err != nil {
		return nil, fmt.Errorf("read skill document %s failed: %w", name, err)
	}
	document, err := Parse(string(data))
	if err != nil {
		return nil, fmt.Errorf("parse skill document %s failed: %w", name, err)
	}
	return document, nil
}

// splitFrontMatter 分离 Markdown skill 文档的 YAML front matter 和正文。
func splitFrontMatter(raw string) (string, string, error) {
	normalized := strings.ReplaceAll(raw, "\r\n", "\n")
	if !strings.HasPrefix(normalized, "---\n") {
		return "", "", errors.New("skill document front matter is missing")
	}

	lines := strings.SplitAfter(normalized, "\n")
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) != "---" {
			continue
		}
		header := strings.Join(lines[1:i], "")
		body := strings.Join(lines[i+1:], "")
		return header, body, nil
	}
	return "", "", errors.New("skill document front matter is not closed")
}

// parseMetadata 解析 YAML front matter，返回 Metadata。
func parseMetadata(header string) (Metadata, error) {
	raw := map[string]any{}
	if err := yaml.Unmarshal([]byte(header), &raw); err != nil {
		return Metadata{}, fmt.Errorf("parse skill metadata yaml: %w", err)
	}
	metadata := Metadata{}
	var err error
	if metadata.Name, err = requiredString(raw, "name"); err != nil {
		return Metadata{}, err
	}
	if metadata.Description, err = requiredString(raw, "description"); err != nil {
		return Metadata{}, err
	}
	if metadata.Version, err = optionalString(raw, "version"); err != nil {
		return Metadata{}, err
	}
	if metadata.Icon, err = optionalString(raw, "icon"); err != nil {
		return Metadata{}, err
	}
	if metadata.Tags, err = optionalStringList(raw, "tags"); err != nil {
		return Metadata{}, err
	}
	if metadata.Tools, err = optionalStringList(raw, "tools"); err != nil {
		return Metadata{}, err
	}
	if metadata.Requirements, err = parseRequirements(raw["requirements"]); err != nil {
		return Metadata{}, err
	}
	metadata.Annotations = annotations(raw, knownMetadataKeys)
	return metadata, nil
}

// parseRequirements 解析 requirements 字段，返回 Requirements。
func parseRequirements(raw any) (Requirements, error) {
	if raw == nil {
		return Requirements{}, nil
	}
	values, ok := raw.(map[string]any)
	if !ok {
		return Requirements{}, errors.New("requirements must be an object")
	}
	requirements := Requirements{}
	var err error
	if requirements.Env, err = optionalStringList(values, "env"); err != nil {
		return Requirements{}, err
	}
	if requirements.Binaries, err = optionalStringList(values, "binaries"); err != nil {
		return Requirements{}, err
	}
	if requirements.OS, err = optionalStringList(values, "os"); err != nil {
		return Requirements{}, err
	}
	requirements.Annotations = annotations(values, knownRequirementKeys)
	return requirements, nil
}

func requiredString(values map[string]any, key string) (string, error) {
	value, err := optionalString(values, key)
	if err != nil {
		return "", err
	}
	if value == "" {
		return "", fmt.Errorf("%s is required", key)
	}
	return value, nil
}

func optionalString(values map[string]any, key string) (string, error) {
	raw, ok := values[key]
	if !ok || raw == nil {
		return "", nil
	}
	value, ok := raw.(string)
	if !ok {
		return "", fmt.Errorf("%s must be a string", key)
	}
	return strings.TrimSpace(value), nil
}

func optionalStringList(values map[string]any, key string) ([]string, error) {
	raw, ok := values[key]
	if !ok || raw == nil {
		return nil, nil
	}
	items, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("%s must be a list", key)
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		value, ok := item.(string)
		if !ok {
			return nil, fmt.Errorf("%s items must be strings", key)
		}
		value = strings.TrimSpace(value)
		if value != "" {
			out = append(out, value)
		}
	}
	return out, nil
}

func annotations(values map[string]any, known map[string]struct{}) map[string]any {
	out := map[string]any{}
	for key, value := range values {
		if _, ok := known[key]; ok {
			continue
		}
		out[key] = value
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
