package http

import (
	"time"

	"github.com/boxify/api-go/internal/observability/xlog"
	"github.com/boxify/api-go/internal/svc"
	"github.com/boxify/api-go/internal/transport/http/handler"
	"github.com/boxify/api-go/internal/transport/http/middleware"
	"github.com/boxify/api-go/internal/transport/http/response"
	"github.com/boxify/api-go/internal/transport/http/routes"
	"github.com/boxify/api-go/internal/xerr"
	"github.com/gin-gonic/gin"
)

type Dependencies struct {
	Svc                   *svc.ServiceContext
	EnableDebugPanicRoute bool
}

func NewRouter(deps Dependencies) *gin.Engine {
	gin.SetMode(gin.TestMode)
	response.RegisterValidatorTagNames()

	r := gin.New()
	r.Use(xlog.RecoveryMiddleware())
	r.Use(xlog.GinMiddleware())
	r.Use(cors())
	r.NoRoute(func(c *gin.Context) {
		response.FromError(c, xerr.NotFound("route not found"))
	})

	health := handler.HealthHandler{}
	auth := handler.NewAuthHandler(deps.Svc)
	chat := handler.NewChatHandler(deps.Svc)
	modelConfig := handler.NewModelConfigHandler(deps.Svc)
	authMiddleware := middleware.Auth(deps.Svc.TokenIssuer)

	api := r.Group("/api")
	routes.RegisterHealthRoutes(api, health)
	routes.RegisterAuthRoutes(api, auth, authMiddleware)
	routes.RegisterChatRoutes(api, chat, authMiddleware)
	routes.RegisterModelConfigRoutes(api, modelConfig, authMiddleware)
	if deps.EnableDebugPanicRoute {
		routes.RegisterDebugRoutes(api)
	}
	return r
}

func cors() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Request-ID")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		c.Header("Access-Control-Max-Age", (12 * time.Hour).String())
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	}
}
