package handler

import (
	modelconfiglogic "github.com/boxify/api-go/internal/logic/modelconfig"
	"github.com/boxify/api-go/internal/svc"
	"github.com/boxify/api-go/internal/transport/http/request"
	"github.com/boxify/api-go/internal/transport/http/response"
	"github.com/boxify/api-go/internal/util"
	"github.com/boxify/api-go/internal/xerr"
	"github.com/gin-gonic/gin"
)

type ModelConfigHandler struct {
	svc *svc.ServiceContext
}

func NewModelConfigHandler(svcCtx *svc.ServiceContext) ModelConfigHandler {
	return ModelConfigHandler{svc: svcCtx}
}

func (h ModelConfigHandler) ListModels(c *gin.Context) {
	var query request.ListModelsRequest
	if err := c.ShouldBindQuery(&query); err != nil {
		response.FromError(c, xerr.Validation(err))
		return
	}
	userID, err := util.UserIDFromContext(c.Request.Context())
	if err != nil {
		response.FromError(c, err)
		return
	}
	out, err := modelconfiglogic.NewListModelsLogic(c.Request.Context(), h.svc).ListModels(userID, &query)
	if err != nil {
		response.FromError(c, err)
		return
	}
	response.OK(c, out)
}

func (h ModelConfigHandler) CreateModel(c *gin.Context) {
	var body request.CreateModelRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		response.FromError(c, xerr.Validation(err))
		return
	}
	userID, err := util.UserIDFromContext(c.Request.Context())
	if err != nil {
		response.FromError(c, err)
		return
	}
	out, err := modelconfiglogic.NewCreateModelLogic(c.Request.Context(), h.svc).CreateModel(userID, &body)
	if err != nil {
		response.FromError(c, err)
		return
	}
	response.OK(c, out)
}
