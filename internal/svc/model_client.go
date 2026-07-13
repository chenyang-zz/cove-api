package svc

import (
	"context"

	corellm "github.com/boxify/api-go/internal/core/llm"
	"github.com/boxify/api-go/internal/domain/types"
	"github.com/boxify/api-go/internal/xerr"
	"github.com/google/uuid"
)

// EmbeddingClient 返回当前用户默认向量模型客户端。
//
// 缺少模型配置时返回 bad request；依赖未初始化或 API Key 解密失败时返回 internal 错误。
func EmbeddingClient(ctx context.Context, svcCtx *ServiceContext, userID uuid.UUID) (corellm.Client, error) {
	if svcCtx == nil || svcCtx.ModelConfigRepo == nil || svcCtx.SecretCipher == nil || svcCtx.LLMManager == nil {
		return nil, xerr.Internal("向量模型依赖未初始化", nil)
	}
	modelType := types.EmbeddingModelType
	configs, err := svcCtx.ModelConfigRepo.List(ctx, userID, &modelType)
	if err != nil {
		return nil, err
	}
	if len(configs) == 0 {
		return nil, xerr.BadRequest("未配置向量模型，请先在模型配置中添加")
	}
	defaultConfig := configs[0]
	for _, config := range configs {
		if config.IsDefault {
			defaultConfig = config
			break
		}
	}
	apiKey, err := svcCtx.SecretCipher.Decrypt(defaultConfig.APIKeyEncrypted)
	if err != nil {
		return nil, xerr.Internal("模型 API Key 解密失败", err)
	}
	return svcCtx.LLMManager.NewClient(corellm.ModelConfig{
		Provider:       defaultConfig.Provider,
		Model:          defaultConfig.ModelName,
		APIKey:         apiKey,
		BaseURL:        defaultConfig.BaseURL,
		EmbeddingModel: defaultConfig.ModelName,
	})
}

// ChatClient 返回当前用户默认聊天模型客户端。
//
// 缺少模型配置时返回 bad request；依赖未初始化或 API Key 解密失败时返回 internal 错误。
func ChatClient(ctx context.Context, svcCtx *ServiceContext, userID uuid.UUID) (corellm.Client, error) {
	if svcCtx == nil || svcCtx.ModelConfigRepo == nil || svcCtx.SecretCipher == nil || svcCtx.LLMManager == nil {
		return nil, xerr.Internal("聊天模型依赖未初始化", nil)
	}
	modelType := types.ChatModelType
	configs, err := svcCtx.ModelConfigRepo.List(ctx, userID, &modelType)
	if err != nil {
		return nil, err
	}
	if len(configs) == 0 {
		return nil, xerr.BadRequest("未配置聊天模型，请先在模型配置中添加")
	}
	defaultConfig := configs[0]
	for _, config := range configs {
		if config.IsDefault {
			defaultConfig = config
			break
		}
	}
	apiKey, err := svcCtx.SecretCipher.Decrypt(defaultConfig.APIKeyEncrypted)
	if err != nil {
		return nil, xerr.Internal("模型 API Key 解密失败", err)
	}
	return svcCtx.LLMManager.NewClient(corellm.ModelConfig{
		Provider: defaultConfig.Provider,
		Model:    defaultConfig.ModelName,
		APIKey:   apiKey,
		BaseURL:  defaultConfig.BaseURL,
	})
}

// MultimodalClient 返回当前用户默认多模态模型客户端。
//
// 缺少模型配置时返回 bad request；依赖未初始化或 API Key 解密失败时返回 internal 错误。
// 返回的客户端通常还应实现 corellm.VisionClient。
func MultimodalClient(ctx context.Context, svcCtx *ServiceContext, userID uuid.UUID) (corellm.Client, error) {
	if svcCtx == nil || svcCtx.ModelConfigRepo == nil || svcCtx.SecretCipher == nil || svcCtx.LLMManager == nil {
		return nil, xerr.Internal("多模态模型依赖未初始化", nil)
	}
	modelType := types.Multimodal
	configs, err := svcCtx.ModelConfigRepo.List(ctx, userID, &modelType)
	if err != nil {
		return nil, err
	}
	if len(configs) == 0 {
		return nil, xerr.BadRequest("未配置多模态模型，请先在模型配置中添加")
	}
	defaultConfig := configs[0]
	for _, config := range configs {
		if config.IsDefault {
			defaultConfig = config
			break
		}
	}
	apiKey, err := svcCtx.SecretCipher.Decrypt(defaultConfig.APIKeyEncrypted)
	if err != nil {
		return nil, xerr.Internal("模型 API Key 解密失败", err)
	}
	return svcCtx.LLMManager.NewClient(corellm.ModelConfig{
		Provider: defaultConfig.Provider,
		Model:    defaultConfig.ModelName,
		APIKey:   apiKey,
		BaseURL:  defaultConfig.BaseURL,
	})
}
