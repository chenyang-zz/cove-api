package main

import (
	"go/ast"
	"go/token"
	"strings"
)

func parseDirective(line string) (Directive, bool) {
	text := strings.TrimSpace(line)
	text = strings.TrimPrefix(text, "//")
	text = strings.TrimSpace(text)
	if !strings.HasPrefix(text, "routegen:") {
		return Directive{}, false
	}
	text = strings.TrimSpace(strings.TrimPrefix(text, "routegen:"))
	var directive Directive
	for _, field := range strings.Fields(text) {
		switch {
		case field == "auth":
			directive.Auth = true
		case field == "user_id":
			directive.UserID = true
		case field == "sse":
			directive.SSE = true
		case strings.HasPrefix(field, "input="):
			directive.Input = strings.TrimPrefix(field, "input=")
		case strings.HasPrefix(field, "output="):
			directive.Output = strings.TrimPrefix(field, "output=")
		}
	}
	return directive, true
}

func parseAtDirective(line string) (key string, value string, ok bool) {
	text := cleanCommentText(line)
	if !strings.HasPrefix(text, "@") {
		return "", "", false
	}
	text = strings.TrimSpace(strings.TrimPrefix(text, "@"))
	if text == "" {
		return "", "", false
	}
	parts := strings.Fields(text)
	if len(parts) == 0 {
		return "", "", false
	}
	key = parts[0]
	value = strings.TrimSpace(strings.TrimPrefix(text, key))
	if idx := strings.Index(key, "("); idx > 0 && strings.HasSuffix(key, ")") {
		value = strings.TrimSpace(strings.TrimSuffix(key[idx+1:], ")"))
		key = key[:idx]
	}
	return key, value, true
}

func parseDirectiveGroup(group *ast.CommentGroup) (Directive, []string, bool) {
	if group == nil {
		return Directive{}, nil, false
	}
	commentLines := logicCommentLinesFromGroup(group)

	for i := len(group.List) - 1; i >= 0; i-- {
		if directive, ok := parseDirective(group.List[i].Text); ok {
			return directive, commentLines, true
		}
	}

	var directive Directive
	enabled := false
	for i := 0; i < len(group.List); i++ {
		item := group.List[i]
		key, value, ok := parseAtDirective(item.Text)
		if !ok {
			continue
		}
		switch key {
		case "routegen":
			enabled = true
		case "auth":
			enabled = true
			directive.Auth = true
			if value == "user_id" || value == "userID" {
				directive.UserID = true
			}
		case "userID", "user_id":
			enabled = true
			directive.UserID = true
		case "sse":
			enabled = true
			directive.SSE = true
		case "description":
			enabled = true
			directive.Description = append([]string{value}, descriptionContinuationLines(group.List[i+1:])...)
		case "input":
			enabled = true
			directive.Input = value
		case "output":
			enabled = true
			directive.Output = value
		case "response":
			enabled = true
			directive.Output = normalizeResponseType(value)
		case "summary":
			enabled = true
			directive.Summary = value
		}
	}
	if !enabled {
		return Directive{}, nil, false
	}
	if len(directive.Description) > 0 {
		commentLines = nonEmptyLines(directive.Description)
	}
	return directive, commentLines, true
}

func descriptionContinuationLines(items []*ast.Comment) []string {
	var lines []string
	for _, item := range items {
		text := cleanCommentText(item.Text)
		if text == "" {
			continue
		}
		if strings.HasPrefix(text, "@") {
			break
		}
		if _, ok := parseDirective(item.Text); ok {
			break
		}
		lines = append(lines, text)
	}
	return lines
}

func normalizeResponseType(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return value
	}
	for _, prefix := range []string{"[]*", "[]", "*"} {
		if strings.HasPrefix(value, prefix) {
			return prefix + normalizeResponseType(strings.TrimPrefix(value, prefix))
		}
	}
	if idx := strings.Index(value, "["); idx > 0 {
		base := value[:idx]
		return normalizeResponseType(base) + value[idx:]
	}
	switch {
	case strings.HasPrefix(value, "map["):
		return value
	case strings.Contains(value, "."):
		return value
	default:
		return "response." + value
	}
}

func directiveForCall(fset *token.FileSet, file *ast.File, call ast.Node) (Directive, []string, bool) {
	callLine := fset.Position(call.Pos()).Line
	for _, group := range file.Comments {
		if fset.Position(group.End()).Line != callLine-1 {
			continue
		}
		if directive, commentLines, ok := parseDirectiveGroup(group); ok {
			return directive, commentLines, true
		}
	}
	return Directive{}, nil, false
}

func logicCommentLinesFromGroup(group *ast.CommentGroup) []string {
	if group == nil {
		return nil
	}
	var lines []string
	for _, item := range group.List {
		text := cleanCommentText(item.Text)
		if text == "" {
			continue
		}
		if _, ok := parseDirective(item.Text); ok {
			continue
		}
		if isRoutegenAtDirective(item.Text) {
			continue
		}
		lines = append(lines, text)
	}
	return lines
}

func isRoutegenAtDirective(line string) bool {
	key, _, ok := parseAtDirective(line)
	if !ok {
		return false
	}
	switch key {
	case "routegen", "auth", "userID", "user_id", "sse", "description", "summary", "input", "output", "response":
		return true
	default:
		return strings.HasPrefix(key, "routegen.")
	}
}

func cleanCommentText(text string) string {
	text = strings.TrimSpace(text)
	text = strings.TrimPrefix(text, "//")
	text = strings.TrimSpace(text)
	return text
}
