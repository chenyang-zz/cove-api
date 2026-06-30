package mcpserver

import (
	"github.com/boxify/api-go/internal/infrastructure/security"
	"github.com/boxify/api-go/internal/models"
	"github.com/boxify/api-go/internal/transport/http/request"
	"github.com/boxify/api-go/internal/xerr"
	"github.com/google/uuid"
)

func mcpServerIDFromInput(input *request.UriMCPServerIDRequest) (uuid.UUID, error) {
	if input == nil {
		return uuid.Nil, xerr.BadRequest("MCP服务 ID 无效")
	}
	id, err := uuid.Parse(input.ID)
	if err != nil {
		return uuid.Nil, xerr.BadRequest("MCP服务 ID 无效")
	}
	return id, nil
}

func encryptMCPAuthConfig(cipher interface {
	Encrypt(string) (string, error)
}, input *request.MCPAuthConfig) (models.MCPAuthConfig, error) {
	out := models.MCPAuthConfig{}
	if input == nil {
		return out, nil
	}
	if input.Token != "" {
		encrypted, err := cipher.Encrypt(input.Token)
		if err != nil {
			return nil, xerr.Internal("MCP认证配置加密失败", err)
		}
		out["token"] = encrypted
	}
	if input.Header != "" {
		out["header"] = input.Header
	}
	if input.Placement != "" {
		out["placement"] = input.Placement
	}
	if input.QueryParam != "" {
		out["query_param"] = input.QueryParam
	}
	if input.Key != "" {
		encrypted, err := cipher.Encrypt(input.Key)
		if err != nil {
			return nil, xerr.Internal("MCP认证配置加密失败", err)
		}
		out["key"] = encrypted
	}
	return out, nil
}

func decryptMCPAuthConfig(cipher interface {
	Decrypt(string) (string, error)
}, input models.MCPAuthConfig) (models.MCPAuthConfig, error) {
	out := models.MCPAuthConfig{}
	if input == nil {
		return out, nil
	}
	if token := input["token"]; token != "" {
		plain, err := cipher.Decrypt(token)
		if err != nil {
			return nil, xerr.Internal("MCP认证配置解密失败", err)
		}
		out["token"] = plain
	}
	if header := input["header"]; header != "" {
		out["header"] = header
	}
	if placement := input["placement"]; placement != "" {
		out["placement"] = placement
	}
	if queryParam := input["query_param"]; queryParam != "" {
		out["query_param"] = queryParam
	}
	if key := input["key"]; key != "" {
		plain, err := cipher.Decrypt(key)
		if err != nil {
			return nil, xerr.Internal("MCP认证配置解密失败", err)
		}
		out["key"] = plain
	}
	return out, nil
}

func maskMCPAuthConfig(authType string, authConfig models.MCPAuthConfig) string {
	if authType == string(request.Bearer) {
		return security.MaskSecret(authConfig["token"])
	}
	if authType == string(request.ApiKey) {
		return security.MaskSecret(authConfig["key"])
	}
	return ""
}
