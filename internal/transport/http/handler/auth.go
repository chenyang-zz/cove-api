package handler

import (
	authlogic "github.com/boxify/api-go/internal/logic/auth"
	"github.com/boxify/api-go/internal/svc"
	"github.com/boxify/api-go/internal/transport/http/request"
	"github.com/boxify/api-go/internal/transport/http/response"
	"github.com/boxify/api-go/internal/util"
	"github.com/boxify/api-go/internal/xerr"
	"github.com/gin-gonic/gin"
)

type AuthHandler struct {
	svc *svc.ServiceContext
}

func NewAuthHandler(svcCtx *svc.ServiceContext) AuthHandler {
	return AuthHandler{svc: svcCtx}
}

func (h AuthHandler) Register(c *gin.Context) {
	var body request.RegisterRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		response.FromError(c, xerr.Validation(err))
		return
	}
	out, err := authlogic.NewRegisterLogic(c.Request.Context(), h.svc).Register(&body)
	if err != nil {
		response.FromError(c, err)
		return
	}
	response.OK(c, out)
}

func (h AuthHandler) Login(c *gin.Context) {
	var body request.LoginRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		response.FromError(c, xerr.Validation(err))
		return
	}
	out, err := authlogic.NewLoginLogic(c.Request.Context(), h.svc).Login(&body)
	if err != nil {
		response.FromError(c, err)
		return
	}
	response.OK(c, out)
}

func (h AuthHandler) Refresh(c *gin.Context) {
	var body request.RefreshRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		response.FromError(c, xerr.Validation(err))
		return
	}
	out, err := authlogic.NewRefreshLogic(c.Request.Context(), h.svc).Refresh(&body)
	if err != nil {
		response.FromError(c, err)
		return
	}
	response.OK(c, out)
}

func (h AuthHandler) Me(c *gin.Context) {
	out, err := authlogic.NewMeLogic(c.Request.Context(), h.svc).Me()
	if err != nil {
		response.FromError(c, err)
		return
	}
	response.OK(c, out)
}

func (h AuthHandler) Profile(c *gin.Context) {
	var body request.ProfileRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		response.FromError(c, xerr.Validation(err))
		return
	}
	userID, err := util.UserIDFromContext(c.Request.Context())
	if err != nil {
		response.FromError(c, err)
		return
	}
	out, err := authlogic.NewProfileLogic(c.Request.Context(), h.svc).Profile(userID, &body)
	if err != nil {
		response.FromError(c, err)
		return
	}
	response.OK(c, out)
}

func (h AuthHandler) Password(c *gin.Context) {
	var body request.PasswordRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		response.FromError(c, xerr.Validation(err))
		return
	}
	userID, err := util.UserIDFromContext(c.Request.Context())
	if err != nil {
		response.FromError(c, err)
		return
	}
	if err := authlogic.NewPasswordLogic(c.Request.Context(), h.svc).Password(userID, &body); err != nil {
		response.FromError(c, err)
		return
	}
	response.OK(c, nil)
}
