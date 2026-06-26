package modelconfig

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/boxify/api-go/internal/domain"
	"github.com/boxify/api-go/internal/infrastructure/security"
	"github.com/boxify/api-go/internal/observability/xlog"
	"github.com/boxify/api-go/internal/svc"
	"github.com/boxify/api-go/internal/transport/http/request"
	"github.com/boxify/api-go/internal/transport/http/response"
	"github.com/google/uuid"
)

type ListModelsLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	log    *slog.Logger
}

func NewListModelsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListModelsLogic {
	return &ListModelsLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		log:    xlog.Component("logic.modelconfig.listmodels"),
	}
}

func (l *ListModelsLogic) ListModels(userID uuid.UUID, input *request.ListModelsRequest) (*response.ListModelsResponse, error) {
	fmt.Println("type", input.Type)
	var modelType *domain.ModelType
	if input.Type != "" {
		tmp := domain.ModelType(input.Type)
		modelType = &tmp
	}
	models, err := l.svcCtx.ModelConfigRepo.List(l.ctx, userID, modelType)
	if err != nil {
		return nil, err
	}

	res := make([]*response.ModelResponse, 0, len(models))
	for _, model := range models {
		decodedApiKey, err := l.svcCtx.SecretCipher.Decrypt(model.APIKeyEncrypted)
		if err != nil {
			l.log.WarnContext(l.ctx, "模型 API KEY 解析失败", slog.String("错误", err.Error()))
			continue
		}
		res = append(res, &response.ModelResponse{
			ID:           model.ID,
			Type:         model.Type,
			Provider:     model.Provider,
			Name:         model.Name,
			ModelName:    model.ModelName,
			APIKeyMasked: security.MaskSecret(decodedApiKey),
			BaseURL:      model.BaseURL,
			Capability:   model.Capability,
			IsDefault:    model.IsDefault,
			CreatedAt:    model.CreatedAt,
		})
	}

	return &response.ListModelsResponse{List: res}, nil

}
