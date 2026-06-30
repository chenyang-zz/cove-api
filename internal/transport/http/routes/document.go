/**
 * @Time   : 2026/6/30 21:56
 * @Author : chenyangzhao542@gmail.com
 * @File   : document.go
 **/

package routes

import (
	"github.com/boxify/api-go/internal/transport/http/handler"
	"github.com/gin-gonic/gin"
)

func RegisterDocumentRoutes(api *gin.RouterGroup, document handler.DocumentHandler, authMiddleware gin.HandlerFunc) {
	documentGroup := api.Group("/document", authMiddleware)

	// @auth(user_id)
	// @description 上传文档
	// @input request.UploadDocumentRequest
	// @output response.DocumentResponse
	documentGroup.POST("/upload", document.UploadDocument)

	// @auth(user_id)
	// @description 从url导入文档
	// @input request.URLImportRequest
	// @output response.DocumentResponse
	documentGroup.POST("/from-url", document.ImportDocumentFromUrl)

	// @auth(user_id)
	// @description 获取文档列表
	// @input request.ListDocumentsRequest
	// @output response.PageListResponse[*response.DocumentResponse]
	documentGroup.GET("/", document.ListDocuments)
	documentGroup.GET("/list", document.ListDocuments)

	// @auth(user_id)
	// @description 获取文档详情
	// @input request.UriDocumentIDRequest
	// @output response.DocumentResponse
	documentGroup.GET("/:doc_id", document.GetDocument)

	// @auth(user_id)
	// @description 预览文档原文内容
	// @input request.UriDocumentIDRequest
	// @output response.PreviewDocumentResponse
	documentGroup.GET("/:doc_id/preview", document.PreviewDocumentContent)

	// @auth(user_id)
	// @description 获取文档状态
	// @input request.UriDocumentIDRequest
	// @output response.DocumentStatusResponse
	documentGroup.GET("/:doc_id/status", document.GetDocumentStatus)

	// @auth(user_id)
	// @description 重新提交文档解析
	// @input request.UriDocumentIDRequest
	// @output response.DocumentResponse
	documentGroup.POST("/:doc_id/retry", document.ReParseDocument)

	// @auth(user_id)
	// @description 删除文档
	// @input request.UriDocumentIDRequest
	documentGroup.DELETE("/:doc_id", document.DeleteDocument)
	documentGroup.POST("/:doc_id/delete", document.DeleteDocument)

	// @auth(user_id)
	// @description 检索文档
	// @input request.SearchDocumentsRequest
	documentGroup.POST("/:doc_id/search", document.SearchDocuments)

	// @auth(user_id)
	// @description 移动文档到指定知识库
	// @input request.MoveDocumentRequest
	// @output response.DocumentResponse
	documentGroup.POST("/:doc_id/move", document.MoveDocument)
}
