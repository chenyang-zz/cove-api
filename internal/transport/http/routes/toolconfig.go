/*
 * @Time   : 2026-07-10 12:07:41
 * @Author : chenyang
 * @File   : toolconfig.go
 */

package routes

import (
	"github.com/boxify/api-go/internal/transport/http/handler"
	"github.com/gin-gonic/gin"
)

func RegisterToolConfigRoutes(api *gin.RouterGroup, toolConfig handler.ToolConfigHandler, authMiddleware gin.HandlerFunc) {
	toolConfigRoutes := api.Group("/tool-config", authMiddleware)

	// @auth(user_id)
	// @description 查询工具配置列表
	// @output response.ToolConfigListResponse
	toolConfigRoutes.GET("/", toolConfig.ListToolConfigs)
	toolConfigRoutes.GET("/list", toolConfig.ListToolConfigs)

	// @auth(user_id)
	// @description 开启/关闭工具
	// @input request.ToggleToolRequest
	toolConfigRoutes.POST("/:tool_key/toggle", toolConfig.ToggleTool)
}
