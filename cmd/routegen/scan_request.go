package main

import (
	"go/ast"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
)

func scanRequestDTOs(root string) (map[string]RequestDTO, error) {
	requestDir := filepath.Join(root, "internal", "transport", "http", "request")
	structs := map[string]requestStruct{}
	if err := scanGoFiles(requestDir, func(path string, file *ast.File) {
		for _, decl := range file.Decls {
			genDecl, ok := decl.(*ast.GenDecl)
			if !ok {
				continue
			}
			for _, spec := range genDecl.Specs {
				typeSpec, ok := spec.(*ast.TypeSpec)
				if !ok {
					continue
				}
				structType, ok := typeSpec.Type.(*ast.StructType)
				if !ok {
					continue
				}
				structs[typeSpec.Name.Name] = requestStruct{Fields: requestFields(structType)}
			}
		}
	}); err != nil {
		return nil, err
	}

	out := map[string]RequestDTO{}
	for name := range structs {
		out[name] = RequestDTO{
			HasJSONBody:      requestStructHasJSONBody(name, structs, map[string]bool{}),
			HasMultipartBody: requestStructHasMultipartBody(name, structs, map[string]bool{}),
			HasDirectURI:     requestStructHasDirectURI(name, structs),
			IsURIOnly:        requestStructIsURIOnly(name, structs, map[string]bool{}),
			EmbeddedURIOnly:  requestStructEmbeddedURIOnlyTypes(name, structs),
		}
	}
	return out, nil
}

func requestFields(structType *ast.StructType) []requestField {
	if structType == nil || structType.Fields == nil {
		return nil
	}
	fields := make([]requestField, 0, len(structType.Fields.List))
	for _, field := range structType.Fields.List {
		item := requestField{}
		if field.Tag != nil {
			item.JSONTag = tagValue(field.Tag.Value, "json")
			item.URITag = tagValue(field.Tag.Value, "uri")
			item.FormTag = tagValue(field.Tag.Value, "form")
		}
		if len(field.Names) == 0 {
			item.EmbeddedType = requestEmbeddedTypeName(field.Type)
		}
		item.HasMultipartFile = tagName(item.FormTag) != "" && requestExprIsMultipartFileHeader(field.Type)
		fields = append(fields, item)
	}
	return fields
}

func requestStructHasJSONBody(name string, structs map[string]requestStruct, visiting map[string]bool) bool {
	if visiting[name] {
		return false
	}
	info, ok := structs[name]
	if !ok {
		return false
	}
	visiting[name] = true
	defer delete(visiting, name)

	for _, field := range info.Fields {
		if tagName(field.JSONTag) != "" && tagName(field.URITag) == "" {
			return true
		}
		if field.EmbeddedType != "" && requestStructHasJSONBody(field.EmbeddedType, structs, visiting) {
			return true
		}
	}
	return false
}

func requestStructHasMultipartBody(name string, structs map[string]requestStruct, visiting map[string]bool) bool {
	if visiting[name] {
		return false
	}
	info, ok := structs[name]
	if !ok {
		return false
	}
	visiting[name] = true
	defer delete(visiting, name)

	for _, field := range info.Fields {
		if field.HasMultipartFile {
			return true
		}
		if field.EmbeddedType != "" && requestStructHasMultipartBody(field.EmbeddedType, structs, visiting) {
			return true
		}
	}
	return false
}

func requestStructHasDirectURI(name string, structs map[string]requestStruct) bool {
	info, ok := structs[name]
	if !ok {
		return false
	}
	for _, field := range info.Fields {
		if tagName(field.URITag) != "" {
			return true
		}
	}
	return false
}

func requestStructEmbeddedURIOnlyTypes(name string, structs map[string]requestStruct) []string {
	info, ok := structs[name]
	if !ok {
		return nil
	}
	var out []string
	for _, field := range info.Fields {
		if field.EmbeddedType == "" {
			continue
		}
		if requestStructIsURIOnly(field.EmbeddedType, structs, map[string]bool{}) {
			out = append(out, field.EmbeddedType)
		}
	}
	return out
}

func requestStructIsURIOnly(name string, structs map[string]requestStruct, visiting map[string]bool) bool {
	if visiting[name] {
		return false
	}
	info, ok := structs[name]
	if !ok {
		return false
	}
	visiting[name] = true
	defer delete(visiting, name)

	hasURI := false
	for _, field := range info.Fields {
		if tagName(field.JSONTag) != "" || tagName(field.FormTag) != "" || field.HasMultipartFile {
			return false
		}
		if tagName(field.URITag) != "" {
			hasURI = true
			continue
		}
		if field.EmbeddedType != "" && requestStructIsURIOnly(field.EmbeddedType, structs, visiting) {
			hasURI = true
			continue
		}
		return false
	}
	return hasURI
}

func requestEmbeddedTypeName(expr ast.Expr) string {
	switch item := expr.(type) {
	case *ast.Ident:
		return item.Name
	case *ast.StarExpr:
		return requestEmbeddedTypeName(item.X)
	case *ast.SelectorExpr:
		return item.Sel.Name
	default:
		return ""
	}
}

func requestExprIsMultipartFileHeader(expr ast.Expr) bool {
	switch item := expr.(type) {
	case *ast.SelectorExpr:
		return item.Sel.Name == "FileHeader"
	case *ast.StarExpr:
		return requestExprIsMultipartFileHeader(item.X)
	case *ast.ArrayType:
		return requestExprIsMultipartFileHeader(item.Elt)
	default:
		return false
	}
}

func tagValue(raw string, key string) string {
	if raw == "" {
		return ""
	}
	unquoted, err := strconv.Unquote(raw)
	if err != nil {
		return ""
	}
	return reflect.StructTag(unquoted).Get(key)
}

func tagName(tag string) string {
	if tag == "" {
		return ""
	}
	name := strings.Split(tag, ",")[0]
	if name == "-" {
		return ""
	}
	return name
}
