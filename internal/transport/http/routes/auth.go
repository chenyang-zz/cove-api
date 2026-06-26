package routes

import (
	"github.com/boxify/api-go/internal/transport/http/handler"
	"github.com/gin-gonic/gin"
)

func RegisterAuthRoutes(api *gin.RouterGroup, auth handler.AuthHandler, authMiddleware gin.HandlerFunc) {
	authRoutes := api.Group("/auth")
	authRoutes.POST("/register", auth.Register)
	authRoutes.POST("/login", auth.Login)
	authRoutes.POST("/refresh", auth.Refresh)

	authRoutes.GET("/me", authMiddleware, auth.Me)
	// routegen: auth user_id input=request.ProfileRequest output=response.UserResponse
	authRoutes.PUT("/profile", authMiddleware, auth.Profile)
	// routegen: auth user_id input=request.PasswordRequest
	authRoutes.POST("/password", authMiddleware, auth.Password)
}
