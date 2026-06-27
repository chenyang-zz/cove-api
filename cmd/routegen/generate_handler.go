package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func generateHandler(root, domain string, routes []Route, existing map[string]string, requestDTOs map[string]RequestDTO, report *Report) error {
	if len(routes) == 0 {
		return nil
	}
	handlerType := routes[0].HandlerType
	var snippets []string
	generatedHandlerType := false
	if _, ok := existing[handlerType]; !ok {
		snippets = append(snippets, fmt.Sprintf(`type %[1]s struct {
	svc *svc.ServiceContext
}

func New%[1]s(svcCtx *svc.ServiceContext) %[1]s {
	return %[1]s{svc: svcCtx}
}
`, handlerType))
		existing[handlerType] = handlerPath(root, domain)
		existing["New"+handlerType] = handlerPath(root, domain)
		generatedHandlerType = true
	}
	for _, route := range routes {
		key := route.HandlerType + "." + route.HandlerMethod
		if path, ok := existing[key]; ok {
			report.Add(FileSkipped, path)
			continue
		}
		snippet, err := handlerMethodSnippet(route, requestDTOs)
		if err != nil {
			return err
		}
		snippets = append(snippets, snippet)
		existing[key] = handlerPath(root, domain)
	}
	if len(snippets) == 0 {
		return nil
	}

	imports := handlerImports(domain, routes, generatedHandlerType)
	body := strings.Join(snippets, "\n")
	path := handlerPath(root, domain)
	if _, err := os.Stat(path); err == nil {
		return appendGoFile(path, imports, body, report)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	content := generatedFile("handler", imports, body, false)
	return writeGeneratedFile(path, content, report)
}

func handlerPath(root, domain string) string {
	return filepath.Join(root, "internal", "transport", "http", "handler", domain+".go")
}

func handlerImports(domain string, routes []Route, includeSvc bool) []string {
	imports := []string{
		fmt.Sprintf(`%slogic "%s/internal/logic/%s"`, domain, modulePath, domain),
		fmt.Sprintf(`"%s/internal/transport/http/response"`, modulePath),
		`"github.com/gin-gonic/gin"`,
	}
	if includeSvc {
		imports = append(imports, fmt.Sprintf(`"%s/internal/svc"`, modulePath))
	}
	needsRequest := false
	needsUtil := false
	needsXerr := false
	for _, route := range routes {
		if route.Directive.Input != "" {
			needsRequest = true
			needsXerr = true
		}
		if route.Directive.UserID {
			needsUtil = true
		}
	}
	if needsRequest {
		imports = append(imports, fmt.Sprintf(`"%s/internal/transport/http/request"`, modulePath))
	}
	if needsUtil {
		imports = append(imports, fmt.Sprintf(`"%s/internal/util"`, modulePath))
	}
	if needsXerr {
		imports = append(imports, fmt.Sprintf(`"%s/internal/xerr"`, modulePath))
	}
	sort.Strings(imports)
	return imports
}

func handlerMethodSnippet(route Route, requestDTOs map[string]RequestDTO) (string, error) {
	var b strings.Builder
	fmt.Fprintf(&b, "func (h %s) %s(c *gin.Context) {\n", route.HandlerType, route.HandlerMethod)
	if route.Directive.Input != "" {
		inputVar, bindMethod := handlerInputBinding(route)
		fmt.Fprintf(&b, "\tvar %s %s\n", inputVar, route.Directive.Input)
		if routeHasURIParam(route) {
			uriTarget, err := handlerURIBindTarget(route, inputVar, requestDTOs)
			if err != nil {
				return "", err
			}
			fmt.Fprintf(&b, "\tif err := c.ShouldBindUri(%s); err != nil {\n", uriTarget)
			b.WriteString("\t\tresponse.FromError(c, xerr.Validation(err))\n")
			b.WriteString("\t\treturn\n")
			b.WriteString("\t}\n")
		}
		if route.HTTPMethod != "GET" && routeShouldBindMultipart(route, requestDTOs) {
			bindMethod = "ShouldBind"
		}
		if route.HTTPMethod == "GET" || routeShouldBindMultipart(route, requestDTOs) || routeShouldBindJSON(route, requestDTOs) {
			fmt.Fprintf(&b, "\tif err := c.%s(&%s); err != nil {\n", bindMethod, inputVar)
			b.WriteString("\t\tresponse.FromError(c, xerr.Validation(err))\n")
			b.WriteString("\t\treturn\n")
			b.WriteString("\t}\n")
		}
	}
	if route.Directive.UserID {
		b.WriteString("\tuserID, err := util.UserIDFromContext(c.Request.Context())\n")
		b.WriteString("\tif err != nil {\n")
		b.WriteString("\t\tresponse.FromError(c, err)\n")
		b.WriteString("\t\treturn\n")
		b.WriteString("\t}\n")
	}
	callArgs := handlerCallArgs(route)
	if route.Directive.SSE {
		fmt.Fprintf(&b, "\tevents, err := %slogic.New%sLogic(c.Request.Context(), h.svc).%s(%s)\n", route.Domain, route.HandlerMethod, route.HandlerMethod, strings.Join(callArgs, ", "))
		b.WriteString("\tif err != nil {\n")
		b.WriteString("\t\tresponse.FromError(c, err)\n")
		b.WriteString("\t\treturn\n")
		b.WriteString("\t}\n")
		b.WriteString("\t_ = events\n")
		b.WriteString("\tresponse.OK(c, map[string]any{})\n")
		b.WriteString("}\n")
		return b.String(), nil
	}
	if route.Directive.Output == "" {
		fmt.Fprintf(&b, "\tif err := %slogic.New%sLogic(c.Request.Context(), h.svc).%s(%s); err != nil {\n", route.Domain, route.HandlerMethod, route.HandlerMethod, strings.Join(callArgs, ", "))
		b.WriteString("\t\tresponse.FromError(c, err)\n")
		b.WriteString("\t\treturn\n")
		b.WriteString("\t}\n")
		b.WriteString("\tresponse.OK(c, nil)\n")
		b.WriteString("}\n")
		return b.String(), nil
	}
	fmt.Fprintf(&b, "\tout, err := %slogic.New%sLogic(c.Request.Context(), h.svc).%s(%s)\n", route.Domain, route.HandlerMethod, route.HandlerMethod, strings.Join(callArgs, ", "))
	b.WriteString("\tif err != nil {\n")
	b.WriteString("\t\tresponse.FromError(c, err)\n")
	b.WriteString("\t\treturn\n")
	b.WriteString("\t}\n")
	b.WriteString("\tresponse.OK(c, out)\n")
	b.WriteString("}\n")
	return b.String(), nil
}

func handlerInputBinding(route Route) (inputVar string, bindMethod string) {
	if route.HTTPMethod == "GET" {
		return "query", "ShouldBindQuery"
	}
	return "body", "ShouldBindJSON"
}

func handlerURIBindTarget(route Route, inputVar string, requestDTOs map[string]RequestDTO) (string, error) {
	typeName := requestTypeName(route.Directive.Input)
	dto, ok := requestDTOs[typeName]
	if !ok {
		return "&" + inputVar, nil
	}
	if dto.HasDirectURI {
		return "&" + inputVar, nil
	}
	if len(dto.EmbeddedURIOnly) > 1 {
		return "", fmt.Errorf("routegen: %s.%s input %s has multiple embedded URI-only request DTOs: %s", route.HandlerType, route.HandlerMethod, route.Directive.Input, strings.Join(dto.EmbeddedURIOnly, ", "))
	}
	if len(dto.EmbeddedURIOnly) == 1 && !dto.IsURIOnly {
		return "&" + inputVar + "." + dto.EmbeddedURIOnly[0], nil
	}
	return "&" + inputVar, nil
}

func routeShouldBindJSON(route Route, requestDTOs map[string]RequestDTO) bool {
	if route.HTTPMethod == "GET" || route.Directive.Input == "" {
		return false
	}
	typeName := requestTypeName(route.Directive.Input)
	dto, ok := requestDTOs[typeName]
	if !ok {
		return true
	}
	return !dto.HasMultipartBody && dto.HasJSONBody
}

func routeShouldBindMultipart(route Route, requestDTOs map[string]RequestDTO) bool {
	if route.HTTPMethod == "GET" || route.Directive.Input == "" {
		return false
	}
	typeName := requestTypeName(route.Directive.Input)
	dto, ok := requestDTOs[typeName]
	if !ok {
		return false
	}
	return dto.HasMultipartBody
}

func requestTypeName(input string) string {
	input = strings.TrimSpace(input)
	input = strings.TrimPrefix(input, "*")
	if idx := strings.LastIndex(input, "."); idx >= 0 {
		return input[idx+1:]
	}
	return input
}

func routeHasURIParam(route Route) bool {
	return pathHasURIParam(route.Path)
}

func pathHasURIParam(path string) bool {
	for _, segment := range strings.Split(path, "/") {
		if strings.HasPrefix(segment, ":") && len(segment) > 1 {
			return true
		}
	}
	return false
}

func handlerCallArgs(route Route) []string {
	var args []string
	if route.Directive.UserID {
		args = append(args, "userID")
	}
	if route.Directive.Input != "" {
		inputVar, _ := handlerInputBinding(route)
		args = append(args, "&"+inputVar)
	}
	return args
}
