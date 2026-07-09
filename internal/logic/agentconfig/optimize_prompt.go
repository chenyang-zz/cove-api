package agentconfig

import (
	"context"
	"log/slog"

	"github.com/boxify/api-go/internal/core/llm"
	"github.com/boxify/api-go/internal/domain/types"
	"github.com/boxify/api-go/internal/observability/xlog"
	"github.com/boxify/api-go/internal/prompts/promptsgen"
	"github.com/boxify/api-go/internal/svc"
	"github.com/boxify/api-go/internal/transport/http/request"
	"github.com/boxify/api-go/internal/transport/http/response"
	"github.com/boxify/api-go/internal/xerr"
	"github.com/google/uuid"
)

// OptimizePromptLogic contains the optimizePrompt use case.
type OptimizePromptLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	log    *slog.Logger
}

// NewOptimizePromptLogic creates a OptimizePromptLogic.
func NewOptimizePromptLogic(ctx context.Context, svcCtx *svc.ServiceContext) *OptimizePromptLogic {
	return &OptimizePromptLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		log:    xlog.Component("logic.agentconfig.optimizeprompt"),
	}
}

// OptimizePrompt 优化提示词
func (l *OptimizePromptLogic) OptimizePrompt(userID uuid.UUID, input *request.OptimizePromptRequest) (*response.OptimizePromptResponse, error) {
	// 获取chat 模型配置
	modelType := types.ChatModelType
	configs, err := l.svcCtx.ModelConfigRepo.List(l.ctx, userID, &modelType)
	if err != nil {
		return nil, err
	}
	if len(configs) == 0 {
		return nil, xerr.BadRequest("未配置对话模型，请先在模型配置中添加")
	}

	// 获取默认模型
	defaultConfig := configs[0]
	for _, config := range configs {
		if config.IsDefault {
			defaultConfig = config
			break
		}
	}

	// 暂时只适配openai格式的模型
	apiKey, err := l.svcCtx.SecretCipher.Decrypt(defaultConfig.APIKeyEncrypted)
	if err != nil {
		return nil, err
	}
	client, err := l.svcCtx.LLMManager.NewClient(llm.ModelConfig{
		Provider: defaultConfig.Provider,
		Model:    defaultConfig.ModelName,
		APIKey:   apiKey,
		BaseURL:  defaultConfig.BaseURL,
	})
	if err != nil {
		return nil, err
	}

	promptText, err := l.svcCtx.PromptClient.AgentOptimizePrompt(&promptsgen.AgentOptimizePromptParams{
		RawPrompt: input.SystemPrompt,
	})
	if err != nil {
		return nil, err
	}
	result, err := client.Invoke(l.ctx, []*llm.Message{
		llm.UserMessage(promptText),
	}, llm.WithTemperature(0.4))
	if err != nil {
		return nil, xerr.Wrapf(err, "调用模型失败, err: %v", err)
	}

	return &response.OptimizePromptResponse{
		Optimized: result,
	}, nil
}
