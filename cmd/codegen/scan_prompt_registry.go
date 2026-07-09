package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"
)

func scanPromptDefinitions(path string) ([]promptDefinition, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		return nil, fmt.Errorf("parse prompt registry %s: %w", path, err)
	}
	function := findPromptRegistryFunction(file)
	if function == nil {
		return nil, fmt.Errorf("prompt registry %s does not define registeredPrompts", path)
	}

	var definitions []promptDefinition
	ast.Inspect(function.Body, func(node ast.Node) bool {
		literal, ok := node.(*ast.CompositeLit)
		if !ok {
			return true
		}
		definition, ok := promptDefinitionFromLiteral(literal)
		if ok {
			definitions = append(definitions, definition)
		}
		return true
	})
	if len(definitions) == 0 {
		return nil, fmt.Errorf("prompt registry %s contains no prompt definitions", path)
	}
	if err := validatePromptDefinitions(definitions); err != nil {
		return nil, err
	}
	return definitions, nil
}

func findPromptRegistryFunction(file *ast.File) *ast.FuncDecl {
	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if ok && function.Name.Name == "registeredPrompts" {
			return function
		}
	}
	return nil
}

func promptDefinitionFromLiteral(literal *ast.CompositeLit) (promptDefinition, bool) {
	var definition promptDefinition
	found := false
	for _, element := range literal.Elts {
		pair, ok := element.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		key, ok := pair.Key.(*ast.Ident)
		if !ok || (key.Name != "name" && key.Name != "file") {
			continue
		}
		value, ok := pair.Value.(*ast.BasicLit)
		if !ok || value.Kind != token.STRING {
			continue
		}
		text, err := strconv.Unquote(value.Value)
		if err != nil {
			continue
		}
		found = true
		if key.Name == "name" {
			definition.Name = text
		} else {
			definition.File = text
		}
	}
	return definition, found
}

func validatePromptDefinitions(definitions []promptDefinition) error {
	names := make(map[string]struct{}, len(definitions))
	files := make(map[string]struct{}, len(definitions))
	for _, definition := range definitions {
		if err := validatePromptName(definition.Name); err != nil {
			return err
		}
		if strings.TrimSpace(definition.File) == "" {
			return fmt.Errorf("prompt %s file is empty", definition.Name)
		}
		if filepath.Base(definition.File) != definition.File || filepath.Ext(definition.File) != ".tmpl" {
			return fmt.Errorf("prompt %s file %q must be a flat .tmpl filename", definition.Name, definition.File)
		}
		if _, exists := names[definition.Name]; exists {
			return fmt.Errorf("prompt name %s is registered more than once", definition.Name)
		}
		if _, exists := files[definition.File]; exists {
			return fmt.Errorf("prompt file %s is registered more than once", definition.File)
		}
		names[definition.Name] = struct{}{}
		files[definition.File] = struct{}{}
	}
	return nil
}

func validatePromptName(name string) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("prompt definition name is empty")
	}
	for _, segment := range strings.Split(name, "/") {
		if segment == "" {
			return fmt.Errorf("prompt name %q contains an empty segment", name)
		}
		for index, r := range segment {
			letter := r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z'
			digit := r >= '0' && r <= '9'
			if index == 0 && !letter {
				return fmt.Errorf("prompt name %q segment %q must start with a letter", name, segment)
			}
			if !letter && !digit && r != '_' && r != '-' {
				return fmt.Errorf("prompt name %q contains unsupported character %q", name, r)
			}
		}
	}
	return nil
}
