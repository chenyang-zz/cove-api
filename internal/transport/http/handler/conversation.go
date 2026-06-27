package handler

import (
	conversationlogic "github.com/boxify/api-go/internal/logic/conversation"
	"github.com/boxify/api-go/internal/svc"
	"github.com/boxify/api-go/internal/transport/http/request"
	"github.com/boxify/api-go/internal/transport/http/response"
	"github.com/boxify/api-go/internal/util"
	"github.com/boxify/api-go/internal/xerr"
	"github.com/gin-gonic/gin"
)

type ConversationHandler struct {
	svc *svc.ServiceContext
}

func NewConversationHandler(svcCtx *svc.ServiceContext) ConversationHandler {
	return ConversationHandler{svc: svcCtx}
}

func (h ConversationHandler) CreateConversation(c *gin.Context) {
	var body request.CreateConversationRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		response.FromError(c, xerr.Validation(err))
		return
	}
	userID, err := util.UserIDFromContext(c.Request.Context())
	if err != nil {
		response.FromError(c, err)
		return
	}
	out, err := conversationlogic.NewCreateConversationLogic(c.Request.Context(), h.svc).CreateConversation(userID, &body)
	if err != nil {
		response.FromError(c, err)
		return
	}
	response.OK(c, out)
}

func (h ConversationHandler) ListConversations(c *gin.Context) {
	userID, err := util.UserIDFromContext(c.Request.Context())
	if err != nil {
		response.FromError(c, err)
		return
	}
	out, err := conversationlogic.NewListConversationsLogic(c.Request.Context(), h.svc).ListConversations(userID)
	if err != nil {
		response.FromError(c, err)
		return
	}
	response.OK(c, out)
}

func (h ConversationHandler) RenameConversation(c *gin.Context) {
	var body request.RenameConversationRequest
	if err := c.ShouldBindUri(&body.UriConversationIDRequest); err != nil {
		response.FromError(c, xerr.Validation(err))
		return
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		response.FromError(c, xerr.Validation(err))
		return
	}
	userID, err := util.UserIDFromContext(c.Request.Context())
	if err != nil {
		response.FromError(c, err)
		return
	}
	out, err := conversationlogic.NewRenameConversationLogic(c.Request.Context(), h.svc).RenameConversation(userID, &body)
	if err != nil {
		response.FromError(c, err)
		return
	}
	response.OK(c, out)
}
