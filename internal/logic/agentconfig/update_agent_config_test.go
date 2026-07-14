package agentconfig

import (
	"context"
	"testing"

	"github.com/boxify/api-go/internal/models"
	"github.com/boxify/api-go/internal/repository"
	"github.com/boxify/api-go/internal/svc"
	"github.com/boxify/api-go/internal/transport/http/request"
	"github.com/google/uuid"
)

// TestUpdateAgentConfigRejectsInvalidContextRatios 验证上下文目标比例不小于触发比例时不会写数据库。
func TestUpdateAgentConfigRejectsInvalidContextRatios(t *testing.T) {
	repo := newAgentConfigTestRepository()
	logic := NewUpdateAgentConfigLogic(context.Background(), &svc.ServiceContext{AgentConfigRepo: repo})
	target := 0.9

	if _, err := logic.UpdateAgentConfig(repo.row.UserID, &request.UpdateAgentConfigRequest{ContextTargetRatio: &target}); err == nil {
		t.Fatal("UpdateAgentConfig(invalid ratios) error = nil, want validation error")
	}
	if repo.updates != 0 {
		t.Fatalf("UpdateFields calls = %d, want 0 after validation error", repo.updates)
	}
}

// TestUpdateAgentConfigPersistsContextColumns 验证合法上下文配置通过局部字段更新并返回最新值。
func TestUpdateAgentConfigPersistsContextColumns(t *testing.T) {
	repo := newAgentConfigTestRepository()
	logic := NewUpdateAgentConfigLogic(context.Background(), &svc.ServiceContext{AgentConfigRepo: repo})
	window, recent := 65536, 12000

	result, err := logic.UpdateAgentConfig(repo.row.UserID, &request.UpdateAgentConfigRequest{
		ContextWindowTokens:     &window,
		ContextKeepRecentTokens: &recent,
	})
	if err != nil {
		t.Fatalf("UpdateAgentConfig(context columns) error = %v, want nil", err)
	}
	if repo.updates != 1 || result.ContextWindowTokens != window || result.ContextKeepRecentTokens != recent {
		t.Fatalf("UpdateAgentConfig result=%#v updates=%d, want persisted context columns", result, repo.updates)
	}
}

type agentConfigTestRepository struct {
	row     *models.AgentConfig
	updates int
}

func newAgentConfigTestRepository() *agentConfigTestRepository {
	return &agentConfigTestRepository{row: &models.AgentConfig{
		ID: uuid.New(), UserID: uuid.New(), ContextEnabled: true,
		ContextWindowTokens: 32768, ContextOutputReserveTokens: 4096, ContextSafetyMarginTokens: 512,
		ContextTriggerRatio: 0.8, ContextTargetRatio: 0.6, ContextKeepRecentTokens: 8192, ContextSummaryMaxTokens: 1024,
	}}
}

func (r *agentConfigTestRepository) Create(_ context.Context, _ uuid.UUID, row *models.AgentConfig) (*models.AgentConfig, error) {
	return row, nil
}

func (r *agentConfigTestRepository) List(_ context.Context, _ uuid.UUID) ([]*models.AgentConfig, error) {
	return []*models.AgentConfig{r.row}, nil
}

func (r *agentConfigTestRepository) FindByID(_ context.Context, _ uuid.UUID, _ uuid.UUID) (*models.AgentConfig, error) {
	return r.row, nil
}

func (r *agentConfigTestRepository) FindByUserID(_ context.Context, _ uuid.UUID) (*models.AgentConfig, error) {
	return r.row, nil
}

func (r *agentConfigTestRepository) Update(_ context.Context, _ uuid.UUID, row *models.AgentConfig) (*models.AgentConfig, error) {
	r.row = row
	return row, nil
}

func (r *agentConfigTestRepository) UpdateFields(_ context.Context, _ uuid.UUID, _ uuid.UUID, patch *models.AgentConfig, fields *repository.AgentConfigUpdateFields) (*models.AgentConfig, error) {
	r.updates++
	for _, column := range fields.Columns() {
		switch column {
		case "context_window_tokens":
			r.row.ContextWindowTokens = patch.ContextWindowTokens
		case "context_keep_recent_tokens":
			r.row.ContextKeepRecentTokens = patch.ContextKeepRecentTokens
		}
	}
	return r.row, nil
}

func (r *agentConfigTestRepository) Delete(_ context.Context, _ uuid.UUID, _ uuid.UUID) error {
	return nil
}
