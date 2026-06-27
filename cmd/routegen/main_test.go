package main

import (
	"bytes"
	"go/ast"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseDirective(t *testing.T) {
	directive, ok := parseDirective("// routegen: auth user_id input=request.CreateBookRequest output=response.BookResponse")
	if !ok {
		t.Fatal("parseDirective ok = false, want true")
	}
	if !directive.Auth || !directive.UserID {
		t.Fatalf("directive auth/user_id = %v/%v, want true/true", directive.Auth, directive.UserID)
	}
	if directive.Input != "request.CreateBookRequest" {
		t.Fatalf("input = %q", directive.Input)
	}
	if directive.Output != "response.BookResponse" {
		t.Fatalf("output = %q", directive.Output)
	}
}

func TestParseDirectiveWithoutOutputKeepsOutputEmpty(t *testing.T) {
	directive, ok := parseDirective("// routegen: auth user_id")
	if !ok {
		t.Fatal("parseDirective ok = false, want true")
	}
	if directive.Output != "" {
		t.Fatalf("output = %q, want empty", directive.Output)
	}
}

func TestParseDirectiveGroupSupportsAtRoutegenProtocol(t *testing.T) {
	group := commentGroup(
		"// 创建图书。",
		"// 保存图书的基础信息。",
		"// @routegen",
		"// @auth",
		"// @userID",
		"// @input *request.CreateBookRequest",
		"// @output response.ListResponse[*response.BookResponse]",
	)

	directive, comments, ok := parseDirectiveGroup(group)
	if !ok {
		t.Fatal("parseDirectiveGroup ok = false, want true")
	}
	if !directive.Auth || !directive.UserID {
		t.Fatalf("directive auth/userID = %v/%v, want true/true", directive.Auth, directive.UserID)
	}
	if directive.Input != "*request.CreateBookRequest" {
		t.Fatalf("input = %q", directive.Input)
	}
	if directive.Output != "response.ListResponse[*response.BookResponse]" {
		t.Fatalf("output = %q", directive.Output)
	}
	wantComments := []string{"创建图书。", "保存图书的基础信息。"}
	if strings.Join(comments, "\n") != strings.Join(wantComments, "\n") {
		t.Fatalf("comments = %#v, want %#v", comments, wantComments)
	}
}

func TestParseDirectiveGroupSupportsNewAnnotationProtocol(t *testing.T) {
	group := commentGroup(
		"// 普通注释会被 description 覆盖。",
		"// @auth(user_id)",
		"// @description 重命名会话",
		"// 修改当前用户拥有的会话标题。",
		"// 标题不能为空。",
		"// @summary 重命名会话",
		"// @input request.RenameConversationRequest",
		"// @response ConversationResponse",
	)

	directive, comments, ok := parseDirectiveGroup(group)
	if !ok {
		t.Fatal("parseDirectiveGroup ok = false, want true")
	}
	if !directive.Auth || !directive.UserID {
		t.Fatalf("directive auth/userID = %v/%v, want true/true", directive.Auth, directive.UserID)
	}
	if directive.Input != "request.RenameConversationRequest" {
		t.Fatalf("input = %q", directive.Input)
	}
	if directive.Output != "response.ConversationResponse" {
		t.Fatalf("output = %q, want response.ConversationResponse", directive.Output)
	}
	if directive.Summary != "重命名会话" {
		t.Fatalf("summary = %q", directive.Summary)
	}
	wantDescription := []string{"重命名会话", "修改当前用户拥有的会话标题。", "标题不能为空。"}
	if strings.Join(comments, "\n") != strings.Join(wantDescription, "\n") {
		t.Fatalf("comments = %#v, want %#v", comments, wantDescription)
	}
}

func TestParseDirectiveGroupSupportsAuthWithoutUserIDAndResponseNormalization(t *testing.T) {
	group := commentGroup(
		"// @auth",
		"// @input request.ListConversationRequest",
		"// @response ListResponse[*response.ConversationResponse]",
	)

	directive, _, ok := parseDirectiveGroup(group)
	if !ok {
		t.Fatal("parseDirectiveGroup ok = false, want true")
	}
	if !directive.Auth || directive.UserID {
		t.Fatalf("directive auth/userID = %v/%v, want true/false", directive.Auth, directive.UserID)
	}
	if directive.Output != "response.ListResponse[*response.ConversationResponse]" {
		t.Fatalf("output = %q", directive.Output)
	}
}

func TestScanRoutesParsesRouteDirectiveAndHandlerType(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "internal/transport/http/routes/book.go", `package routes

import (
	"github.com/boxify/api-go/internal/transport/http/handler"
	"github.com/gin-gonic/gin"
)

func RegisterBookRoutes(api *gin.RouterGroup, book handler.BookHandler, authMiddleware gin.HandlerFunc) {
	bookRoutes := api.Group("/books", authMiddleware)
	// routegen: auth user_id input=request.CreateBookRequest output=response.BookResponse
	bookRoutes.POST("", book.Create)
	// routegen: auth user_id output=[]*response.BookResponse
	bookRoutes.GET("", book.List)
}
`)

	routes, err := scanRoutes(root)
	if err != nil {
		t.Fatalf("scanRoutes error = %v", err)
	}
	if len(routes) != 2 {
		t.Fatalf("len(routes) = %d, want 2", len(routes))
	}
	create := routes[0]
	if create.HTTPMethod != "POST" || create.HandlerType != "BookHandler" || create.HandlerMethod != "Create" {
		t.Fatalf("create route = %+v", create)
	}
	if create.Domain != "book" || create.Directive.Input != "request.CreateBookRequest" || create.Directive.Output != "response.BookResponse" {
		t.Fatalf("create route directive/domain = %+v", create)
	}
	list := routes[1]
	if list.HTTPMethod != "GET" || list.HandlerMethod != "List" || list.Directive.Output != "[]*response.BookResponse" {
		t.Fatalf("list route = %+v", list)
	}
}

func TestScanRoutesFindsHandlerAfterMiddlewareArgs(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "internal/transport/http/routes/book.go", `package routes

import (
	"github.com/boxify/api-go/internal/transport/http/handler"
	"github.com/gin-gonic/gin"
)

func RegisterBookRoutes(api *gin.RouterGroup, book handler.BookHandler, authMiddleware gin.HandlerFunc) {
	bookRoutes := api.Group("/books")
	// routegen: auth user_id input=request.UpdateBookRequest output=response.BookResponse
	bookRoutes.PUT("/:id", authMiddleware, book.Update)
}
`)

	routes, err := scanRoutes(root)
	if err != nil {
		t.Fatalf("scanRoutes error = %v", err)
	}
	if len(routes) != 1 {
		t.Fatalf("len(routes) = %d, want 1", len(routes))
	}
	if routes[0].HandlerType != "BookHandler" || routes[0].HandlerMethod != "Update" {
		t.Fatalf("route = %+v, want BookHandler.Update", routes[0])
	}
	if routes[0].Path != "/:id" {
		t.Fatalf("route path = %q, want /:id", routes[0].Path)
	}
}

func TestScanRoutesParsesCustomLogicComment(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "internal/transport/http/routes/book.go", `package routes

import (
	"github.com/boxify/api-go/internal/transport/http/handler"
	"github.com/gin-gonic/gin"
)

func RegisterBookRoutes(api *gin.RouterGroup, book handler.BookHandler) {
	bookRoutes := api.Group("/books")
	// 创建图书。
	// 保存图书的基础信息。
	// routegen: input=request.CreateBookRequest output=response.BookResponse
	bookRoutes.POST("", book.Create)
}
`)

	routes, err := scanRoutes(root)
	if err != nil {
		t.Fatalf("scanRoutes error = %v", err)
	}
	if len(routes) != 1 {
		t.Fatalf("len(routes) = %d, want 1", len(routes))
	}
	if got := strings.Join(routes[0].CommentLines, "\n"); got != "创建图书。\n保存图书的基础信息。" {
		t.Fatalf("comment lines = %q", got)
	}
}

func TestScanRoutesParsesAtRoutegenProtocolAndMultilineComment(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "internal/transport/http/routes/book.go", `package routes

import (
	"github.com/boxify/api-go/internal/transport/http/handler"
	"github.com/gin-gonic/gin"
)

func RegisterBookRoutes(api *gin.RouterGroup, book handler.BookHandler) {
	bookRoutes := api.Group("/books")
	// 创建图书。
	// 保存图书的基础信息。
	// @routegen
	// @auth
	// @userID
	// @input request.CreateBookRequest
	// @output response.BookResponse
	bookRoutes.POST("", book.Create)
}
`)

	routes, err := scanRoutes(root)
	if err != nil {
		t.Fatalf("scanRoutes error = %v", err)
	}
	if len(routes) != 1 {
		t.Fatalf("len(routes) = %d, want 1", len(routes))
	}
	route := routes[0]
	if !route.Directive.Auth || !route.Directive.UserID {
		t.Fatalf("directive auth/userID = %v/%v, want true/true", route.Directive.Auth, route.Directive.UserID)
	}
	if route.Directive.Input != "request.CreateBookRequest" || route.Directive.Output != "response.BookResponse" {
		t.Fatalf("directive = %+v", route.Directive)
	}
	if got := strings.Join(route.CommentLines, "\n"); got != "创建图书。\n保存图书的基础信息。" {
		t.Fatalf("comment lines = %q", got)
	}
}

func TestGenerateCreatesMissingHandlerAndLogic(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "internal/transport/http/routes/book.go", `package routes

import (
	"github.com/boxify/api-go/internal/transport/http/handler"
	"github.com/gin-gonic/gin"
)

func RegisterBookRoutes(api *gin.RouterGroup, book handler.BookHandler, authMiddleware gin.HandlerFunc) {
	bookRoutes := api.Group("/books", authMiddleware)
	// routegen: auth user_id input=request.CreateBookRequest output=response.BookResponse
	bookRoutes.POST("", book.Create)
}
`)

	report, err := Generate(root)
	if err != nil {
		t.Fatalf("Generate error = %v", err)
	}

	handlerFile := readFile(t, root, "internal/transport/http/handler/book.go")
	for _, want := range []string{
		"type BookHandler struct",
		"func NewBookHandler(svcCtx *svc.ServiceContext) BookHandler",
		"func (h BookHandler) Create(c *gin.Context)",
		"var body request.CreateBookRequest",
		"c.ShouldBindJSON(&body)",
		"userID, err := util.UserIDFromContext(c.Request.Context())",
		"booklogic.NewCreateLogic(c.Request.Context(), h.svc).Create(userID, &body)",
		"response.OK(c, out)",
	} {
		if !strings.Contains(handlerFile, want) {
			t.Fatalf("handler file missing %q:\n%s", want, handlerFile)
		}
	}

	logicFile := readFile(t, root, "internal/logic/book/create.go")
	if strings.Contains(logicFile, generatedHeader) {
		t.Fatalf("logic file contains generated header:\n%s", logicFile)
	}
	for _, want := range []string{
		"// CreateLogic contains the create use case.",
		"type CreateLogic struct",
		"// NewCreateLogic creates a CreateLogic.",
		`xlog.Component("logic.book.create")`,
		"// Create handles the create use case.",
		"func (l *CreateLogic) Create(userID uuid.UUID, input *request.CreateBookRequest) (*response.BookResponse, error)",
		"return nil, nil",
	} {
		if !strings.Contains(logicFile, want) {
			t.Fatalf("logic file missing %q:\n%s", want, logicFile)
		}
	}
	assertReportContains(t, report, FileAdded, "internal/transport/http/handler/book.go")
	assertReportContains(t, report, FileAdded, "internal/logic/book/create.go")
}

func TestGenerateUsesCustomLogicMethodComment(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "internal/transport/http/routes/book.go", `package routes

import (
	"github.com/boxify/api-go/internal/transport/http/handler"
	"github.com/gin-gonic/gin"
)

func RegisterBookRoutes(api *gin.RouterGroup, book handler.BookHandler) {
	bookRoutes := api.Group("/books")
	// 创建图书。
	// 保存图书的基础信息。
	// routegen: input=request.CreateBookRequest output=response.BookResponse
	bookRoutes.POST("", book.Create)
}
`)

	if _, err := Generate(root); err != nil {
		t.Fatalf("Generate error = %v", err)
	}

	logicFile := readFile(t, root, "internal/logic/book/create.go")
	for _, want := range []string{
		"// CreateLogic contains the create use case.",
		"// NewCreateLogic creates a CreateLogic.",
		"// Create 创建图书。\n// 保存图书的基础信息。",
	} {
		if !strings.Contains(logicFile, want) {
			t.Fatalf("logic file missing %q:\n%s", want, logicFile)
		}
	}
}

func TestGenerateUsesAtRoutegenMultilineLogicMethodComment(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "internal/transport/http/routes/book.go", `package routes

import (
	"github.com/boxify/api-go/internal/transport/http/handler"
	"github.com/gin-gonic/gin"
)

func RegisterBookRoutes(api *gin.RouterGroup, book handler.BookHandler) {
	bookRoutes := api.Group("/books")
	// 创建图书。
	// 保存图书的基础信息。
	// @routegen
	// @input request.CreateBookRequest
	// @output response.BookResponse
	bookRoutes.POST("", book.Create)
}
`)

	if _, err := Generate(root); err != nil {
		t.Fatalf("Generate error = %v", err)
	}

	logicFile := readFile(t, root, "internal/logic/book/create.go")
	if !strings.Contains(logicFile, "// Create 创建图书。\n// 保存图书的基础信息。") {
		t.Fatalf("logic file missing multiline custom comment:\n%s", logicFile)
	}
}

func TestGenerateUsesMultilineDescriptionMethodComment(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "internal/transport/http/routes/conversation.go", `package routes

import (
	"github.com/boxify/api-go/internal/transport/http/handler"
	"github.com/gin-gonic/gin"
)

func RegisterConversationRoutes(api *gin.RouterGroup, conversation handler.ConversationHandler, authMiddleware gin.HandlerFunc) {
	conversationRoutes := api.Group("/conversation", authMiddleware)
	// 普通注释不会进入 GoDoc。
	// @auth(user_id)
	// @description 重命名会话
	// 修改当前用户拥有的会话标题。
	// 标题不能为空。
	// @summary 会话标题更新
	// @input request.RenameConversationRequest
	// @response ConversationResponse
	conversationRoutes.PATCH("/:conversation_id", conversation.RenameConversation)
}
`)

	if _, err := Generate(root); err != nil {
		t.Fatalf("Generate error = %v", err)
	}

	logicFile := readFile(t, root, "internal/logic/conversation/rename_conversation.go")
	for _, want := range []string{
		"// RenameConversation 重命名会话\n// 修改当前用户拥有的会话标题。\n// 标题不能为空。",
		"func (l *RenameConversationLogic) RenameConversation(userID uuid.UUID, input *request.RenameConversationRequest) (*response.ConversationResponse, error)",
	} {
		if !strings.Contains(logicFile, want) {
			t.Fatalf("logic file missing %q:\n%s", want, logicFile)
		}
	}
	if strings.Contains(logicFile, "普通注释不会进入 GoDoc") {
		t.Fatalf("description should override normal comments:\n%s", logicFile)
	}
	if strings.Contains(logicFile, "会话标题更新") {
		t.Fatalf("summary should not be used as logic GoDoc:\n%s", logicFile)
	}
}

func TestGenerateDoesNotDuplicateMethodNameInDescription(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "internal/transport/http/routes/conversation.go", `package routes

import (
	"github.com/boxify/api-go/internal/transport/http/handler"
	"github.com/gin-gonic/gin"
)

func RegisterConversationRoutes(api *gin.RouterGroup, conversation handler.ConversationHandler) {
	conversationRoutes := api.Group("/conversation")
	// @description RenameConversation 重命名会话
	// @input request.RenameConversationRequest
	// @response ConversationResponse
	conversationRoutes.PATCH("/:conversation_id", conversation.RenameConversation)
}
`)

	if _, err := Generate(root); err != nil {
		t.Fatalf("Generate error = %v", err)
	}

	logicFile := readFile(t, root, "internal/logic/conversation/rename_conversation.go")
	if !strings.Contains(logicFile, "// RenameConversation 重命名会话") {
		t.Fatalf("logic file missing description comment:\n%s", logicFile)
	}
	if strings.Contains(logicFile, "// RenameConversation RenameConversation") {
		t.Fatalf("logic file duplicated method name:\n%s", logicFile)
	}
}

func TestGenerateDoesNotDuplicateMethodNameInCustomLogicComment(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "internal/transport/http/routes/book.go", `package routes

import (
	"github.com/boxify/api-go/internal/transport/http/handler"
	"github.com/gin-gonic/gin"
)

func RegisterBookRoutes(api *gin.RouterGroup, book handler.BookHandler) {
	bookRoutes := api.Group("/books")
	// Create 创建图书。
	// routegen: input=request.CreateBookRequest output=response.BookResponse
	bookRoutes.POST("", book.Create)
}
`)

	if _, err := Generate(root); err != nil {
		t.Fatalf("Generate error = %v", err)
	}

	logicFile := readFile(t, root, "internal/logic/book/create.go")
	if !strings.Contains(logicFile, "// Create 创建图书。") {
		t.Fatalf("logic file missing custom comment:\n%s", logicFile)
	}
	if strings.Contains(logicFile, "// Create Create") {
		t.Fatalf("logic file duplicated method name:\n%s", logicFile)
	}
}

func TestGenerateUsesQueryBindingForGETInput(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "internal/transport/http/routes/book.go", `package routes

import (
	"github.com/boxify/api-go/internal/transport/http/handler"
	"github.com/gin-gonic/gin"
)

func RegisterBookRoutes(api *gin.RouterGroup, book handler.BookHandler, authMiddleware gin.HandlerFunc) {
	bookRoutes := api.Group("/books", authMiddleware)
	// routegen: auth user_id input=request.ListBooksRequest output=response.ListBooksResponse
	bookRoutes.GET("", book.List)
}
`)

	if _, err := Generate(root); err != nil {
		t.Fatalf("Generate error = %v", err)
	}

	handlerFile := readFile(t, root, "internal/transport/http/handler/book.go")
	for _, want := range []string{
		"var query request.ListBooksRequest",
		"c.ShouldBindQuery(&query)",
		"booklogic.NewListLogic(c.Request.Context(), h.svc).List(userID, &query)",
	} {
		if !strings.Contains(handlerFile, want) {
			t.Fatalf("handler file missing %q:\n%s", want, handlerFile)
		}
	}
	if strings.Contains(handlerFile, "ShouldBindJSON(&query)") || strings.Contains(handlerFile, "var body request.ListBooksRequest") {
		t.Fatalf("GET input should use query binding only:\n%s", handlerFile)
	}
}

func TestGenerateUsesURIAndQueryBindingForGETInputWithURIParam(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "internal/transport/http/routes/book.go", `package routes

import (
	"github.com/boxify/api-go/internal/transport/http/handler"
	"github.com/gin-gonic/gin"
)

func RegisterBookRoutes(api *gin.RouterGroup, book handler.BookHandler, authMiddleware gin.HandlerFunc) {
	bookRoutes := api.Group("/books", authMiddleware)
	// routegen: auth user_id input=request.ListBookChaptersRequest output=response.ListBookChaptersResponse
	bookRoutes.GET("/:id/chapters", book.ListChapters)
}
`)

	if _, err := Generate(root); err != nil {
		t.Fatalf("Generate error = %v", err)
	}

	handlerFile := readFile(t, root, "internal/transport/http/handler/book.go")
	for _, want := range []string{
		"var query request.ListBookChaptersRequest",
		"c.ShouldBindUri(&query)",
		"c.ShouldBindQuery(&query)",
		"booklogic.NewListChaptersLogic(c.Request.Context(), h.svc).ListChapters(userID, &query)",
	} {
		if !strings.Contains(handlerFile, want) {
			t.Fatalf("handler file missing %q:\n%s", want, handlerFile)
		}
	}
	if strings.Index(handlerFile, "ShouldBindUri(&query)") > strings.Index(handlerFile, "ShouldBindQuery(&query)") {
		t.Fatalf("URI binding should be generated before query binding:\n%s", handlerFile)
	}
}

func TestGenerateUsesURIAndJSONBindingForNonGETInputWithURIParam(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "internal/transport/http/request/book.go", `package request

type UriOnlyRequest struct {
	ID string `+"`uri:\"id\" binding:\"required\"`"+`
}

type UpdateBookRequest struct {
	UriOnlyRequest
	Title string `+"`json:\"title\" binding:\"required\"`"+`
}
`)
	writeFile(t, root, "internal/transport/http/routes/book.go", `package routes

import (
	"github.com/boxify/api-go/internal/transport/http/handler"
	"github.com/gin-gonic/gin"
)

func RegisterBookRoutes(api *gin.RouterGroup, book handler.BookHandler, authMiddleware gin.HandlerFunc) {
	bookRoutes := api.Group("/books", authMiddleware)
	// routegen: auth user_id input=request.UpdateBookRequest output=response.BookResponse
	bookRoutes.PUT("/:id", book.Update)
}
`)

	if _, err := Generate(root); err != nil {
		t.Fatalf("Generate error = %v", err)
	}

	handlerFile := readFile(t, root, "internal/transport/http/handler/book.go")
	for _, want := range []string{
		"var body request.UpdateBookRequest",
		"c.ShouldBindUri(&body.UriOnlyRequest)",
		"c.ShouldBindJSON(&body)",
		"booklogic.NewUpdateLogic(c.Request.Context(), h.svc).Update(userID, &body)",
	} {
		if !strings.Contains(handlerFile, want) {
			t.Fatalf("handler file missing %q:\n%s", want, handlerFile)
		}
	}
	if strings.Contains(handlerFile, "ShouldBindUri(&body)") {
		t.Fatalf("composite DTO should bind URI-only embedded struct:\n%s", handlerFile)
	}
	if strings.Index(handlerFile, "ShouldBindUri(&body.UriOnlyRequest)") > strings.Index(handlerFile, "ShouldBindJSON(&body)") {
		t.Fatalf("URI binding should be generated before JSON binding:\n%s", handlerFile)
	}
}

func TestGenerateUsesEmbeddedURIOnlyBindingForRequiredJSONBody(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "internal/transport/http/request/conversation.go", `package request

type UriConversationIDRequest struct {
	ConversationID string `+"`uri:\"conversation_id\" binding:\"required\"`"+`
}

type RenameConversationRequest struct {
	UriConversationIDRequest
	Title string `+"`json:\"title\" binding:\"required,min=1,max=256\"`"+`
}
`)
	writeFile(t, root, "internal/transport/http/routes/conversation.go", `package routes

import (
	"github.com/boxify/api-go/internal/transport/http/handler"
	"github.com/gin-gonic/gin"
)

func RegisterConversationRoutes(api *gin.RouterGroup, conversation handler.ConversationHandler, authMiddleware gin.HandlerFunc) {
	conversationRoutes := api.Group("/conversation", authMiddleware)
	// routegen: auth user_id input=request.RenameConversationRequest output=response.ConversationResponse
	conversationRoutes.PATCH("/:conversation_id", conversation.RenameConversation)
}
`)

	if _, err := Generate(root); err != nil {
		t.Fatalf("Generate error = %v", err)
	}

	handlerFile := readFile(t, root, "internal/transport/http/handler/conversation.go")
	for _, want := range []string{
		"var body request.RenameConversationRequest",
		"c.ShouldBindUri(&body.UriConversationIDRequest)",
		"c.ShouldBindJSON(&body)",
	} {
		if !strings.Contains(handlerFile, want) {
			t.Fatalf("handler file missing %q:\n%s", want, handlerFile)
		}
	}
	if strings.Contains(handlerFile, "ShouldBindUri(&body)") {
		t.Fatalf("composite DTO should not bind complete body for URI:\n%s", handlerFile)
	}
}

func TestGenerateSkipsJSONBindingForURIOnlyDELETEInput(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "internal/transport/http/request/book.go", `package request

type UriOnlyRequest struct {
	ID string `+"`uri:\"id\" binding:\"required\"`"+`
}
`)
	writeFile(t, root, "internal/transport/http/routes/book.go", `package routes

import (
	"github.com/boxify/api-go/internal/transport/http/handler"
	"github.com/gin-gonic/gin"
)

func RegisterBookRoutes(api *gin.RouterGroup, book handler.BookHandler, authMiddleware gin.HandlerFunc) {
	bookRoutes := api.Group("/books", authMiddleware)
	// routegen: auth user_id input=request.UriOnlyRequest
	bookRoutes.DELETE("/:id", book.Delete)
}
`)

	if _, err := Generate(root); err != nil {
		t.Fatalf("Generate error = %v", err)
	}

	handlerFile := readFile(t, root, "internal/transport/http/handler/book.go")
	for _, want := range []string{
		"var body request.UriOnlyRequest",
		"c.ShouldBindUri(&body)",
		"booklogic.NewDeleteLogic(c.Request.Context(), h.svc).Delete(userID, &body)",
	} {
		if !strings.Contains(handlerFile, want) {
			t.Fatalf("handler file missing %q:\n%s", want, handlerFile)
		}
	}
	if strings.Contains(handlerFile, "ShouldBindJSON") {
		t.Fatalf("URI-only input should not bind JSON:\n%s", handlerFile)
	}
}

func TestGenerateSkipsJSONBindingForURIOnlyPOSTInput(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "internal/transport/http/request/book.go", `package request

type UriOnlyRequest struct {
	ID string `+"`uri:\"id\" binding:\"required\"`"+`
}
`)
	writeFile(t, root, "internal/transport/http/routes/book.go", `package routes

import (
	"github.com/boxify/api-go/internal/transport/http/handler"
	"github.com/gin-gonic/gin"
)

func RegisterBookRoutes(api *gin.RouterGroup, book handler.BookHandler, authMiddleware gin.HandlerFunc) {
	bookRoutes := api.Group("/books", authMiddleware)
	// routegen: auth user_id input=request.UriOnlyRequest output=response.BookResponse
	bookRoutes.POST("/:id/default", book.SetDefault)
}
`)

	if _, err := Generate(root); err != nil {
		t.Fatalf("Generate error = %v", err)
	}

	handlerFile := readFile(t, root, "internal/transport/http/handler/book.go")
	for _, want := range []string{
		"var body request.UriOnlyRequest",
		"c.ShouldBindUri(&body)",
		"booklogic.NewSetDefaultLogic(c.Request.Context(), h.svc).SetDefault(userID, &body)",
	} {
		if !strings.Contains(handlerFile, want) {
			t.Fatalf("handler file missing %q:\n%s", want, handlerFile)
		}
	}
	if strings.Contains(handlerFile, "ShouldBindJSON") {
		t.Fatalf("URI-only input should not bind JSON:\n%s", handlerFile)
	}
}

func TestGenerateUsesMultipartBindingForUploadInput(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "internal/transport/http/request/book.go", `package request

import "mime/multipart"

type UploadBookRequest struct {
	File *multipart.FileHeader `+"`form:\"file\" binding:\"required\"`"+`
	Title string `+"`form:\"title\" binding:\"required\"`"+`
}
`)
	writeFile(t, root, "internal/transport/http/routes/book.go", `package routes

import (
	"github.com/boxify/api-go/internal/transport/http/handler"
	"github.com/gin-gonic/gin"
)

func RegisterBookRoutes(api *gin.RouterGroup, book handler.BookHandler, authMiddleware gin.HandlerFunc) {
	bookRoutes := api.Group("/books", authMiddleware)
	// routegen: auth user_id input=request.UploadBookRequest output=response.BookResponse
	bookRoutes.POST("/upload", book.Upload)
}
`)

	if _, err := Generate(root); err != nil {
		t.Fatalf("Generate error = %v", err)
	}

	handlerFile := readFile(t, root, "internal/transport/http/handler/book.go")
	for _, want := range []string{
		"var body request.UploadBookRequest",
		"c.ShouldBind(&body)",
		"booklogic.NewUploadLogic(c.Request.Context(), h.svc).Upload(userID, &body)",
	} {
		if !strings.Contains(handlerFile, want) {
			t.Fatalf("handler file missing %q:\n%s", want, handlerFile)
		}
	}
	if strings.Contains(handlerFile, "ShouldBindJSON") {
		t.Fatalf("multipart input should not bind JSON:\n%s", handlerFile)
	}
}

func TestGenerateUsesURIAndMultipartBindingForUploadInput(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "internal/transport/http/request/book.go", `package request

import "mime/multipart"

type UriOnlyRequest struct {
	ID string `+"`uri:\"id\" binding:\"required\"`"+`
}

type UploadBookCoverRequest struct {
	UriOnlyRequest
	Cover multipart.FileHeader `+"`form:\"cover\" binding:\"required\"`"+`
}
`)
	writeFile(t, root, "internal/transport/http/routes/book.go", `package routes

import (
	"github.com/boxify/api-go/internal/transport/http/handler"
	"github.com/gin-gonic/gin"
)

func RegisterBookRoutes(api *gin.RouterGroup, book handler.BookHandler, authMiddleware gin.HandlerFunc) {
	bookRoutes := api.Group("/books", authMiddleware)
	// routegen: auth user_id input=request.UploadBookCoverRequest output=response.BookResponse
	bookRoutes.POST("/:id/cover", book.UploadCover)
}
`)

	if _, err := Generate(root); err != nil {
		t.Fatalf("Generate error = %v", err)
	}

	handlerFile := readFile(t, root, "internal/transport/http/handler/book.go")
	for _, want := range []string{
		"var body request.UploadBookCoverRequest",
		"c.ShouldBindUri(&body.UriOnlyRequest)",
		"c.ShouldBind(&body)",
		"booklogic.NewUploadCoverLogic(c.Request.Context(), h.svc).UploadCover(userID, &body)",
	} {
		if !strings.Contains(handlerFile, want) {
			t.Fatalf("handler file missing %q:\n%s", want, handlerFile)
		}
	}
	if strings.Contains(handlerFile, "ShouldBindUri(&body)") {
		t.Fatalf("composite multipart DTO should bind URI-only embedded struct:\n%s", handlerFile)
	}
	if strings.Index(handlerFile, "ShouldBindUri(&body.UriOnlyRequest)") > strings.Index(handlerFile, "ShouldBind(&body)") {
		t.Fatalf("URI binding should be generated before multipart binding:\n%s", handlerFile)
	}
	if strings.Contains(handlerFile, "ShouldBindJSON") {
		t.Fatalf("multipart input should not bind JSON:\n%s", handlerFile)
	}
}

func TestGenerateUsesEmbeddedURIOnlyBindingForGETQueryInput(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "internal/transport/http/request/book.go", `package request

type UriProjectIDRequest struct {
	ProjectID string `+"`uri:\"project_id\" binding:\"required\"`"+`
}

type ListProjectBooksRequest struct {
	UriProjectIDRequest
	Type string `+"`form:\"type\" binding:\"omitempty\"`"+`
}
`)
	writeFile(t, root, "internal/transport/http/routes/book.go", `package routes

import (
	"github.com/boxify/api-go/internal/transport/http/handler"
	"github.com/gin-gonic/gin"
)

func RegisterBookRoutes(api *gin.RouterGroup, book handler.BookHandler, authMiddleware gin.HandlerFunc) {
	bookRoutes := api.Group("/projects", authMiddleware)
	// routegen: auth user_id input=request.ListProjectBooksRequest output=response.ListBooksResponse
	bookRoutes.GET("/:project_id/books", book.ListProjectBooks)
}
`)

	if _, err := Generate(root); err != nil {
		t.Fatalf("Generate error = %v", err)
	}

	handlerFile := readFile(t, root, "internal/transport/http/handler/book.go")
	for _, want := range []string{
		"var query request.ListProjectBooksRequest",
		"c.ShouldBindUri(&query.UriProjectIDRequest)",
		"c.ShouldBindQuery(&query)",
	} {
		if !strings.Contains(handlerFile, want) {
			t.Fatalf("handler file missing %q:\n%s", want, handlerFile)
		}
	}
	if strings.Contains(handlerFile, "ShouldBindUri(&query)") {
		t.Fatalf("composite query DTO should bind URI-only embedded struct:\n%s", handlerFile)
	}
	if strings.Index(handlerFile, "ShouldBindUri(&query.UriProjectIDRequest)") > strings.Index(handlerFile, "ShouldBindQuery(&query)") {
		t.Fatalf("URI binding should be generated before query binding:\n%s", handlerFile)
	}
}

func TestGenerateKeepsCompleteDTOURISelectorForDirectURIFields(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "internal/transport/http/request/book.go", `package request

type RenameBookRequest struct {
	ID string `+"`uri:\"id\" binding:\"required\"`"+`
	Title string `+"`json:\"title\" binding:\"required\"`"+`
}
`)
	writeFile(t, root, "internal/transport/http/routes/book.go", `package routes

import (
	"github.com/boxify/api-go/internal/transport/http/handler"
	"github.com/gin-gonic/gin"
)

func RegisterBookRoutes(api *gin.RouterGroup, book handler.BookHandler) {
	bookRoutes := api.Group("/books")
	// routegen: input=request.RenameBookRequest output=response.BookResponse
	bookRoutes.PATCH("/:id", book.Rename)
}
`)

	if _, err := Generate(root); err != nil {
		t.Fatalf("Generate error = %v", err)
	}

	handlerFile := readFile(t, root, "internal/transport/http/handler/book.go")
	if !strings.Contains(handlerFile, "c.ShouldBindUri(&body)") {
		t.Fatalf("direct URI field DTO should bind complete body:\n%s", handlerFile)
	}
}

func TestGenerateFailsForMultipleEmbeddedURIOnlyRequests(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "internal/transport/http/request/book.go", `package request

type UriBookIDRequest struct {
	BookID string `+"`uri:\"book_id\" binding:\"required\"`"+`
}

type UriChapterIDRequest struct {
	ChapterID string `+"`uri:\"chapter_id\" binding:\"required\"`"+`
}

type UpdateChapterRequest struct {
	UriBookIDRequest
	UriChapterIDRequest
	Title string `+"`json:\"title\" binding:\"required\"`"+`
}
`)
	writeFile(t, root, "internal/transport/http/routes/book.go", `package routes

import (
	"github.com/boxify/api-go/internal/transport/http/handler"
	"github.com/gin-gonic/gin"
)

func RegisterBookRoutes(api *gin.RouterGroup, book handler.BookHandler) {
	bookRoutes := api.Group("/books")
	// routegen: input=request.UpdateChapterRequest output=response.BookResponse
	bookRoutes.PATCH("/:book_id/chapters/:chapter_id", book.UpdateChapter)
}
`)

	_, err := Generate(root)
	if err == nil {
		t.Fatal("Generate error = nil, want multiple embedded URI-only error")
	}
	if !strings.Contains(err.Error(), "multiple embedded URI-only") {
		t.Fatalf("Generate error = %v, want multiple embedded URI-only message", err)
	}
}

func TestGenerateUsesMultipartBindingForMultiFileUploadInput(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "internal/transport/http/request/book.go", `package request

import "mime/multipart"

type UploadBookImagesRequest struct {
	Images []*multipart.FileHeader `+"`form:\"images\" binding:\"required\"`"+`
}
`)
	writeFile(t, root, "internal/transport/http/routes/book.go", `package routes

import (
	"github.com/boxify/api-go/internal/transport/http/handler"
	"github.com/gin-gonic/gin"
)

func RegisterBookRoutes(api *gin.RouterGroup, book handler.BookHandler, authMiddleware gin.HandlerFunc) {
	bookRoutes := api.Group("/books", authMiddleware)
	// routegen: auth user_id input=request.UploadBookImagesRequest output=response.BookResponse
	bookRoutes.POST("/images", book.UploadImages)
}
`)

	if _, err := Generate(root); err != nil {
		t.Fatalf("Generate error = %v", err)
	}

	handlerFile := readFile(t, root, "internal/transport/http/handler/book.go")
	if !strings.Contains(handlerFile, "c.ShouldBind(&body)") {
		t.Fatalf("multipart input should bind multipart form:\n%s", handlerFile)
	}
	if strings.Contains(handlerFile, "ShouldBindJSON") {
		t.Fatalf("multipart input should not bind JSON:\n%s", handlerFile)
	}
}

func TestGenerateKeepsJSONBindingWhenRequestDTOCannotBeParsed(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "internal/transport/http/routes/book.go", `package routes

import (
	"github.com/boxify/api-go/internal/transport/http/handler"
	"github.com/gin-gonic/gin"
)

func RegisterBookRoutes(api *gin.RouterGroup, book handler.BookHandler, authMiddleware gin.HandlerFunc) {
	bookRoutes := api.Group("/books", authMiddleware)
	// routegen: auth user_id input=request.UnknownRequest output=response.BookResponse
	bookRoutes.POST("/:id", book.Update)
}
`)

	if _, err := Generate(root); err != nil {
		t.Fatalf("Generate error = %v", err)
	}

	handlerFile := readFile(t, root, "internal/transport/http/handler/book.go")
	if !strings.Contains(handlerFile, "c.ShouldBindJSON(&body)") {
		t.Fatalf("unknown input should keep JSON binding fallback:\n%s", handlerFile)
	}
}

func TestGenerateCreatesNoOutputLogicWithOnlyErrorReturn(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "internal/transport/http/routes/book.go", `package routes

import (
	"github.com/boxify/api-go/internal/transport/http/handler"
	"github.com/gin-gonic/gin"
)

func RegisterBookRoutes(api *gin.RouterGroup, book handler.BookHandler, authMiddleware gin.HandlerFunc) {
	bookRoutes := api.Group("/books", authMiddleware)
	// routegen: auth user_id
	bookRoutes.DELETE("/:id", book.Delete)
}
`)

	if _, err := Generate(root); err != nil {
		t.Fatalf("Generate error = %v", err)
	}

	handlerFile := readFile(t, root, "internal/transport/http/handler/book.go")
	for _, want := range []string{
		"if err := booklogic.NewDeleteLogic(c.Request.Context(), h.svc).Delete(userID); err != nil",
		"response.OK(c, nil)",
	} {
		if !strings.Contains(handlerFile, want) {
			t.Fatalf("handler file missing %q:\n%s", want, handlerFile)
		}
	}
	if strings.Contains(handlerFile, "out, err :=") {
		t.Fatalf("handler file should not declare output value:\n%s", handlerFile)
	}
	if strings.Contains(handlerFile, "ShouldBindUri") {
		t.Fatalf("handler without input should not bind uri:\n%s", handlerFile)
	}

	logicFile := readFile(t, root, "internal/logic/book/delete.go")
	for _, want := range []string{
		"func (l *DeleteLogic) Delete(userID uuid.UUID) error",
		"return nil",
	} {
		if !strings.Contains(logicFile, want) {
			t.Fatalf("logic file missing %q:\n%s", want, logicFile)
		}
	}
	if strings.Contains(logicFile, "transport/http/response") {
		t.Fatalf("logic file should not import response package:\n%s", logicFile)
	}
}

func TestGenerateSkipsExistingHandlerMethodAndLogicFile(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "internal/transport/http/routes/book.go", `package routes

import (
	"github.com/boxify/api-go/internal/transport/http/handler"
	"github.com/gin-gonic/gin"
)

func RegisterBookRoutes(api *gin.RouterGroup, book handler.BookHandler) {
	// routegen: output=response.BookResponse
	api.GET("/books/:id", book.Show)
}
`)
	existingHandler := `package handler

import "github.com/gin-gonic/gin"

type BookHandler struct{}

func (h BookHandler) Show(c *gin.Context) {}
`
	writeFile(t, root, "internal/transport/http/handler/book.go", existingHandler)
	existingLogic := `package book

type ShowLogic struct{}
`
	writeFile(t, root, "internal/logic/book/show.go", existingLogic)

	report, err := Generate(root)
	if err != nil {
		t.Fatalf("Generate error = %v", err)
	}

	if got := readFile(t, root, "internal/transport/http/handler/book.go"); got != existingHandler {
		t.Fatalf("existing handler changed:\n%s", got)
	}
	if got := readFile(t, root, "internal/logic/book/show.go"); got != existingLogic {
		t.Fatalf("existing logic changed:\n%s", got)
	}
	assertReportContains(t, report, FileSkipped, "internal/transport/http/handler/book.go")
	assertReportContains(t, report, FileSkipped, "internal/logic/book/show.go")
}

func TestGenerateAppendsMissingMethodToExistingHandlerFile(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "internal/transport/http/routes/book.go", `package routes

import (
	"github.com/boxify/api-go/internal/transport/http/handler"
	"github.com/gin-gonic/gin"
)

func RegisterBookRoutes(api *gin.RouterGroup, book handler.BookHandler) {
	// routegen: output=response.BookResponse
	api.GET("/books/:id", book.Show)
}
`)
	writeFile(t, root, "internal/transport/http/handler/book.go", `package handler

import "github.com/boxify/api-go/internal/svc"

type BookHandler struct {
	svc *svc.ServiceContext
}
`)

	report, err := Generate(root)
	if err != nil {
		t.Fatalf("Generate error = %v", err)
	}

	handlerFile := readFile(t, root, "internal/transport/http/handler/book.go")
	if !strings.Contains(handlerFile, "func (h BookHandler) Show(c *gin.Context)") {
		t.Fatalf("generated handler missing Show:\n%s", handlerFile)
	}
	assertReportContains(t, report, FileModified, "internal/transport/http/handler/book.go")
}

func TestGenerateMethodOnlyHandlerDoesNotImportSvc(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "internal/transport/http/routes/book.go", `package routes

import (
	"github.com/boxify/api-go/internal/transport/http/handler"
	"github.com/gin-gonic/gin"
)

func RegisterBookRoutes(api *gin.RouterGroup, book handler.BookHandler) {
	// routegen: input=request.UpdateBookRequest output=response.BookResponse
	api.PUT("/books/:id", book.Update)
}
`)
	writeFile(t, root, "internal/transport/http/handler/book.go", `package handler

import "github.com/boxify/api-go/internal/svc"

type BookHandler struct {
	svc *svc.ServiceContext
}
`)

	if _, err := Generate(root); err != nil {
		t.Fatalf("Generate error = %v", err)
	}

	handlerFile := readFile(t, root, "internal/transport/http/handler/book.go")
	if strings.Count(handlerFile, `internal/svc`) != 1 {
		t.Fatalf("method-only generated handler svc import count is not 1:\n%s", handlerFile)
	}
	if !strings.Contains(handlerFile, "func (h BookHandler) Update(c *gin.Context)") {
		t.Fatalf("generated handler missing Update:\n%s", handlerFile)
	}
}

func TestGenerateReportsUnchangedGeneratedFile(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "internal/transport/http/routes/book.go", `package routes

import (
	"github.com/boxify/api-go/internal/transport/http/handler"
	"github.com/gin-gonic/gin"
)

func RegisterBookRoutes(api *gin.RouterGroup, book handler.BookHandler) {
	// routegen: output=response.BookResponse
	api.GET("/books/:id", book.Show)
}
`)

	report, err := Generate(root)
	if err != nil {
		t.Fatalf("Generate error = %v", err)
	}
	if !report.Has(FileAdded, "internal/transport/http/handler/book.go") {
		t.Fatalf("first report = %+v, want added handler", report)
	}

	report, err = Generate(root)
	if err != nil {
		t.Fatalf("second Generate error = %v", err)
	}
	assertReportContains(t, report, FileSkipped, "internal/transport/http/handler/book.go")
}

func TestPrintReportUsesColorAndCanDisableColor(t *testing.T) {
	report := Report{Files: []FileChange{
		{Kind: FileAdded, Path: "internal/transport/http/handler/book.go"},
		{Kind: FileModified, Path: "internal/logic/book/create.go"},
		{Kind: FileSkipped, Path: "internal/logic/book/list.go"},
	}}

	var colored bytes.Buffer
	printReport(&colored, report, true)
	coloredOut := colored.String()
	for _, want := range []string{
		"\x1b[32m+ internal/transport/http/handler/book.go\x1b[0m",
		"\x1b[33m~ internal/logic/book/create.go\x1b[0m",
		"\x1b[90m= internal/logic/book/list.go\x1b[0m",
	} {
		if !strings.Contains(coloredOut, want) {
			t.Fatalf("colored output missing %q:\n%s", want, coloredOut)
		}
	}

	var plain bytes.Buffer
	printReport(&plain, report, false)
	plainOut := plain.String()
	if strings.Contains(plainOut, "\x1b[") {
		t.Fatalf("plain output contains ANSI code:\n%s", plainOut)
	}
	if !strings.Contains(plainOut, "+ internal/transport/http/handler/book.go") {
		t.Fatalf("plain output missing added file:\n%s", plainOut)
	}
}

func assertReportContains(t *testing.T, report Report, kind FileChangeKind, path string) {
	t.Helper()
	if !report.Has(kind, path) {
		t.Fatalf("report = %+v, want %s %s", report, kind, path)
	}
}

func writeFile(t *testing.T, root, path, content string) {
	t.Helper()
	fullPath := filepath.Join(root, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		t.Fatalf("MkdirAll error = %v", err)
	}
	if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile error = %v", err)
	}
}

func readFile(t *testing.T, root, path string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(path)))
	if err != nil {
		t.Fatalf("ReadFile error = %v", err)
	}
	return string(data)
}

func commentGroup(lines ...string) *ast.CommentGroup {
	comments := make([]*ast.Comment, 0, len(lines))
	for _, line := range lines {
		comments = append(comments, &ast.Comment{Text: line})
	}
	return &ast.CommentGroup{List: comments}
}
