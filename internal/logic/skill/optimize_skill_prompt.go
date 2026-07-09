package skill

import (
	"context"
	"log/slog"
	"strings"

	corellm "github.com/boxify/api-go/internal/core/llm"
	"github.com/boxify/api-go/internal/observability/xlog"
	"github.com/boxify/api-go/internal/prompts/promptsgen"
	"github.com/boxify/api-go/internal/svc"
	"github.com/boxify/api-go/internal/transport/http/request"
	"github.com/boxify/api-go/internal/transport/http/response"
	"github.com/boxify/api-go/internal/xerr"
	"github.com/google/uuid"
)

// OptimizeSkillPromptLogic contains the optimizeSkillPrompt use case.
type OptimizeSkillPromptLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	log    *slog.Logger
}

// NewOptimizeSkillPromptLogic creates a OptimizeSkillPromptLogic.
func NewOptimizeSkillPromptLogic(ctx context.Context, svcCtx *svc.ServiceContext) *OptimizeSkillPromptLogic {
	return &OptimizeSkillPromptLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		log:    xlog.Component("logic.skill.optimizeskillprompt"),
	}
}

// OptimizeSkillPrompt 优化提示词
func (l *OptimizeSkillPromptLogic) OptimizeSkillPrompt(userID uuid.UUID, input *request.OptimizeSkillPromptRequest) (*response.OptimizePromptResponse, error) {
	rawPrompt := strings.TrimSpace(input.Prompt)
	client, err := svc.ChatClient(l.ctx, l.svcCtx, userID)
	if err != nil {
		return nil, err
	}
	promptText, err := l.svcCtx.PromptClient.SkillOptimizePrompt(&promptsgen.SkillOptimizePromptParams{RawPrompt: rawPrompt})
	if err != nil {
		return nil, err
	}
	result, err := client.Invoke(l.ctx, []*corellm.Message{
		corellm.UserMessage(promptText),
	}, corellm.WithTemperature(0.4))
	if err != nil {
		return nil, xerr.Wrapf(err, "调用模型失败, err: %v", err)
	}
	l.log.InfoContext(l.ctx, "优化技能提示词", slog.String("user_id", userID.String()))
	return &response.OptimizePromptResponse{Optimized: result}, nil
}
