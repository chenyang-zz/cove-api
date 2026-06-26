package modelconfig

import (
	"context"
	"log/slog"

	"github.com/boxify/api-go/internal/infrastructure/security"
	"github.com/boxify/api-go/internal/models"
	"github.com/boxify/api-go/internal/observability/xlog"
	"github.com/boxify/api-go/internal/svc"
	"github.com/boxify/api-go/internal/transport/http/request"
	"github.com/boxify/api-go/internal/transport/http/response"
	"github.com/boxify/api-go/internal/xerr"
	"github.com/google/uuid"
)

type CreateModelLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	log    *slog.Logger
}

func NewCreateModelLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CreateModelLogic {
	return &CreateModelLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		log:    xlog.Component("logic.modelconfig.createmodel"),
	}
}

func (l *CreateModelLogic) CreateModel(userID uuid.UUID, input *request.CreateModelRequest) (*response.ModelResponse, error) {

	apiKeyEncrypted, err := l.svcCtx.SecretCipher.Encrypt(input.ApiKey)
	if err != nil {
		return nil, xerr.Internal("模型 API Key 加密失败", err)
	}

	model := models.ModelConfig{
		UserID:          userID,
		Type:            input.Type,
		Provider:        input.Provider,
		Name:            input.Name,
		ModelName:       input.ModelName,
		APIKeyEncrypted: apiKeyEncrypted,
		BaseURL:         input.BaseUrl,
		Capability:      models.StringList(input.Capability),
		IsDefault:       input.IsDefault,
	}

	row, err := l.svcCtx.ModelConfigRepo.Create(l.ctx, &model)
	if err != nil {
		return nil, err
	}
	return &response.ModelResponse{
		ID:           row.ID,
		Type:         row.Type,
		Provider:     row.Provider,
		Name:         row.Name,
		ModelName:    row.ModelName,
		APIKeyMasked: security.MaskSecret(input.ApiKey),
		BaseURL:      row.BaseURL,
		Capability:   []string(row.Capability),
		IsDefault:    row.IsDefault,
		CreatedAt:    row.CreatedAt,
	}, nil
}
