package handler

import (
	documentlogic "github.com/boxify/api-go/internal/logic/document"
	"github.com/boxify/api-go/internal/svc"
	"github.com/boxify/api-go/internal/transport/http/request"
	"github.com/boxify/api-go/internal/transport/http/response"
	"github.com/boxify/api-go/internal/util"
	"github.com/boxify/api-go/internal/xerr"
	"github.com/gin-gonic/gin"
)

type DocumentHandler struct {
	svc *svc.ServiceContext
}

func NewDocumentHandler(svcCtx *svc.ServiceContext) DocumentHandler {
	return DocumentHandler{svc: svcCtx}
}

func (h DocumentHandler) UploadDocument(c *gin.Context) {
	var body request.UploadDocumentRequest
	if err := c.ShouldBind(&body); err != nil {
		response.FromError(c, xerr.Validation(err))
		return
	}
	userID, err := util.UserIDFromContext(c.Request.Context())
	if err != nil {
		response.FromError(c, err)
		return
	}
	out, err := documentlogic.NewUploadDocumentLogic(c.Request.Context(), h.svc).UploadDocument(userID, &body)
	if err != nil {
		response.FromError(c, err)
		return
	}
	response.OK(c, out)
}

func (h DocumentHandler) ImportDocumentFromUrl(c *gin.Context) {
	var body request.URLImportRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		response.FromError(c, xerr.Validation(err))
		return
	}
	userID, err := util.UserIDFromContext(c.Request.Context())
	if err != nil {
		response.FromError(c, err)
		return
	}
	out, err := documentlogic.NewImportDocumentFromUrlLogic(c.Request.Context(), h.svc).ImportDocumentFromUrl(userID, &body)
	if err != nil {
		response.FromError(c, err)
		return
	}
	response.OK(c, out)
}

func (h DocumentHandler) ListDocuments(c *gin.Context) {
	var query request.ListDocumentsRequest
	if err := c.ShouldBindQuery(&query); err != nil {
		response.FromError(c, xerr.Validation(err))
		return
	}
	userID, err := util.UserIDFromContext(c.Request.Context())
	if err != nil {
		response.FromError(c, err)
		return
	}
	out, err := documentlogic.NewListDocumentsLogic(c.Request.Context(), h.svc).ListDocuments(userID, &query)
	if err != nil {
		response.FromError(c, err)
		return
	}
	response.OK(c, out)
}

func (h DocumentHandler) GetDocument(c *gin.Context) {
	var query request.UriDocumentIDRequest
	if err := c.ShouldBindUri(&query); err != nil {
		response.FromError(c, xerr.Validation(err))
		return
	}
	if err := c.ShouldBindQuery(&query); err != nil {
		response.FromError(c, xerr.Validation(err))
		return
	}
	userID, err := util.UserIDFromContext(c.Request.Context())
	if err != nil {
		response.FromError(c, err)
		return
	}
	out, err := documentlogic.NewGetDocumentLogic(c.Request.Context(), h.svc).GetDocument(userID, &query)
	if err != nil {
		response.FromError(c, err)
		return
	}
	response.OK(c, out)
}

func (h DocumentHandler) PreviewDocumentContent(c *gin.Context) {
	var query request.UriDocumentIDRequest
	if err := c.ShouldBindUri(&query); err != nil {
		response.FromError(c, xerr.Validation(err))
		return
	}
	if err := c.ShouldBindQuery(&query); err != nil {
		response.FromError(c, xerr.Validation(err))
		return
	}
	userID, err := util.UserIDFromContext(c.Request.Context())
	if err != nil {
		response.FromError(c, err)
		return
	}
	out, err := documentlogic.NewPreviewDocumentContentLogic(c.Request.Context(), h.svc).PreviewDocumentContent(userID, &query)
	if err != nil {
		response.FromError(c, err)
		return
	}
	response.OK(c, out)
}

func (h DocumentHandler) GetDocumentStatus(c *gin.Context) {
	var query request.UriDocumentIDRequest
	if err := c.ShouldBindUri(&query); err != nil {
		response.FromError(c, xerr.Validation(err))
		return
	}
	if err := c.ShouldBindQuery(&query); err != nil {
		response.FromError(c, xerr.Validation(err))
		return
	}
	userID, err := util.UserIDFromContext(c.Request.Context())
	if err != nil {
		response.FromError(c, err)
		return
	}
	out, err := documentlogic.NewGetDocumentStatusLogic(c.Request.Context(), h.svc).GetDocumentStatus(userID, &query)
	if err != nil {
		response.FromError(c, err)
		return
	}
	response.OK(c, out)
}

func (h DocumentHandler) ReParseDocument(c *gin.Context) {
	var body request.UriDocumentIDRequest
	if err := c.ShouldBindUri(&body); err != nil {
		response.FromError(c, xerr.Validation(err))
		return
	}
	userID, err := util.UserIDFromContext(c.Request.Context())
	if err != nil {
		response.FromError(c, err)
		return
	}
	out, err := documentlogic.NewReParseDocumentLogic(c.Request.Context(), h.svc).ReParseDocument(userID, &body)
	if err != nil {
		response.FromError(c, err)
		return
	}
	response.OK(c, out)
}

func (h DocumentHandler) DeleteDocument(c *gin.Context) {
	var body request.UriDocumentIDRequest
	if err := c.ShouldBindUri(&body); err != nil {
		response.FromError(c, xerr.Validation(err))
		return
	}
	userID, err := util.UserIDFromContext(c.Request.Context())
	if err != nil {
		response.FromError(c, err)
		return
	}
	if err := documentlogic.NewDeleteDocumentLogic(c.Request.Context(), h.svc).DeleteDocument(userID, &body); err != nil {
		response.FromError(c, err)
		return
	}
	response.OK(c, nil)
}

func (h DocumentHandler) SearchDocuments(c *gin.Context) {
	var body request.SearchDocumentsRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		response.FromError(c, xerr.Validation(err))
		return
	}
	userID, err := util.UserIDFromContext(c.Request.Context())
	if err != nil {
		response.FromError(c, err)
		return
	}
	out, err := documentlogic.NewSearchDocumentsLogic(c.Request.Context(), h.svc).SearchDocuments(userID, &body)
	if err != nil {
		response.FromError(c, err)
		return
	}
	response.OK(c, out)
}

func (h DocumentHandler) MoveDocument(c *gin.Context) {
	var query request.MoveDocumentRequest
	if err := c.ShouldBindUri(&query.UriDocumentIDRequest); err != nil {
		response.FromError(c, xerr.Validation(err))
		return
	}
	if err := c.ShouldBindJSON(&query); err != nil {
		response.FromError(c, xerr.Validation(err))
		return
	}
	userID, err := util.UserIDFromContext(c.Request.Context())
	if err != nil {
		response.FromError(c, err)
		return
	}
	out, err := documentlogic.NewMoveDocumentLogic(c.Request.Context(), h.svc).MoveDocument(userID, &query)
	if err != nil {
		response.FromError(c, err)
		return
	}
	response.OK(c, out)
}
