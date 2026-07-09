package skill

import (
	"context"
	"errors"
	"slices"
	"testing"

	corellm "github.com/boxify/api-go/internal/core/llm"
	"github.com/boxify/api-go/internal/core/prompt"
	"github.com/boxify/api-go/internal/domain/types"
	"github.com/boxify/api-go/internal/infrastructure/security"
	"github.com/boxify/api-go/internal/models"
	appprompts "github.com/boxify/api-go/internal/prompts"
	"github.com/boxify/api-go/internal/prompts/promptsgen"
	"github.com/boxify/api-go/internal/repository"
	"github.com/boxify/api-go/internal/svc"
	"github.com/boxify/api-go/internal/transport/http/request"
	"github.com/boxify/api-go/internal/xerr"
	"github.com/google/uuid"
)

// TestCreateSkillPersistsNormalizedFields 验证创建技能会规范化字段并持久化到仓储。
func TestCreateSkillPersistsNormalizedFields(t *testing.T) {
	ctx := context.Background()
	userID := uuid.New()
	kbID := uuid.New()
	skillRepo := newFakeSkillRepository()
	kbRepo := newFakeSkillKnowledgeBaseRepository(&models.KnowledgeBase{ID: kbID, UserID: userID, Name: "知识库"})

	out, err := NewCreateSkillLogic(ctx, &svc.ServiceContext{SkillRepo: skillRepo, KnowledgeBaseRepo: kbRepo}).CreateSkill(userID, &request.CreateSkillRequest{
		Name:        "  写作助手  ",
		Description: "  desc  ",
		Prompt:      "  prompt  ",
		ToolKeys:    []string{" search ", "", "time"},
		KBID:        stringPtr(kbID.String()),
		Enabled:     boolPtr(false),
		Config: &request.SkillConfig{
			QuickPrompt: []string{"  快速问题  ", ""},
			FewShots:    []request.FewShot{{Input: " in ", Output: " out "}},
		},
	})
	if err != nil {
		t.Fatalf("CreateSkill error = %v", err)
	}
	if skillRepo.created == nil {
		t.Fatalf("CreateSkill did not persist skill")
	}
	if skillRepo.created.ID == uuid.Nil {
		t.Fatalf("created ID = nil, want generated uuid")
	}
	if skillRepo.created.Name != "写作助手" || skillRepo.created.Description != "desc" || skillRepo.created.Icon != "🧩" || skillRepo.created.Prompt != "prompt" {
		t.Fatalf("created skill fields = %#v", skillRepo.created)
	}
	if !slices.Equal([]string(skillRepo.created.ToolKeys), []string{"search", "time"}) {
		t.Fatalf("ToolKeys = %#v, want [search time]", skillRepo.created.ToolKeys)
	}
	if skillRepo.created.KBID == nil || *skillRepo.created.KBID != kbID {
		t.Fatalf("KBID = %v, want %s", skillRepo.created.KBID, kbID)
	}
	if skillRepo.created.Enabled {
		t.Fatalf("Enabled = true, want false from input")
	}
	if out.ID != skillRepo.created.ID || out.KBID == nil || *out.KBID != kbID {
		t.Fatalf("response = %#v, want persisted id and kb_id", out)
	}
	if out.Config == nil || len(out.Config.QuickPrompt) != 1 || out.Config.QuickPrompt[0] != "快速问题" {
		t.Fatalf("response Config = %#v, want normalized quick prompt", out.Config)
	}
}

// TestCreateSkillRejectsForeignKnowledgeBase 验证创建技能时会校验知识库归属。
func TestCreateSkillRejectsForeignKnowledgeBase(t *testing.T) {
	ctx := context.Background()
	userID := uuid.New()
	kbID := uuid.New()
	kbRepo := newFakeSkillKnowledgeBaseRepository(&models.KnowledgeBase{ID: kbID, UserID: uuid.New(), Name: "其他用户知识库"})

	_, err := NewCreateSkillLogic(ctx, &svc.ServiceContext{SkillRepo: newFakeSkillRepository(), KnowledgeBaseRepo: kbRepo}).CreateSkill(userID, &request.CreateSkillRequest{
		Name: "技能",
		KBID: stringPtr(kbID.String()),
	})
	if xerr.From(err).Kind != xerr.KindNotFound {
		t.Fatalf("CreateSkill foreign kb error kind = %v, want not_found", xerr.From(err).Kind)
	}
}

// TestListSkillsMapsRows 验证技能列表会从仓储读取并映射响应。
func TestListSkillsMapsRows(t *testing.T) {
	ctx := context.Background()
	userID := uuid.New()
	rowID := uuid.New()
	repo := newFakeSkillRepository(&models.Skill{
		ID:       rowID,
		UserID:   userID,
		Name:     "技能",
		ToolKeys: models.StringList{"time"},
		Config:   models.SkillConfig{QuickPrompt: []string{"hello"}},
		Enabled:  true,
	})

	out, err := NewListSkillsLogic(ctx, &svc.ServiceContext{SkillRepo: repo}).ListSkills(userID)
	if err != nil {
		t.Fatalf("ListSkills error = %v", err)
	}
	if len(out.List) != 1 || out.List[0].ID != rowID || out.List[0].KBID != nil {
		t.Fatalf("ListSkills response = %#v, want one skill with nil kb_id", out)
	}
	if out.List[0].Config == nil || len(out.List[0].Config.QuickPrompt) != 1 {
		t.Fatalf("ListSkills Config = %#v, want converted config", out.List[0].Config)
	}
}

// TestSkillIDFromInputUsesStandardUUIDParsing 验证技能 ID helper 与其他 logic 一样处理 nil、非法值和合法 UUID。
func TestSkillIDFromInputUsesStandardUUIDParsing(t *testing.T) {
	if _, err := skillIDFromInput(nil); xerr.From(err).Kind != xerr.KindBadRequest {
		t.Fatalf("skillIDFromInput(nil) kind = %v, want bad_request", xerr.From(err).Kind)
	}
	if _, err := skillIDFromInput(&request.UriSkillIDRequest{ID: " not-a-uuid "}); xerr.From(err).Kind != xerr.KindBadRequest {
		t.Fatalf("skillIDFromInput(invalid) kind = %v, want bad_request", xerr.From(err).Kind)
	}
	want := uuid.New()
	got, err := skillIDFromInput(&request.UriSkillIDRequest{ID: want.String()})
	if err != nil || got != want {
		t.Fatalf("skillIDFromInput(valid) = %s, %v, want %s, nil", got, err, want)
	}
}

// TestUpdateSkillSendsOnlyChangedFields 验证更新技能只提交传入字段并支持清空知识库绑定。
func TestUpdateSkillSendsOnlyChangedFields(t *testing.T) {
	ctx := context.Background()
	userID := uuid.New()
	skillID := uuid.New()
	kbID := uuid.New()
	repo := newFakeSkillRepository(&models.Skill{ID: skillID, UserID: userID, Name: "旧名称", Icon: "🧩", KBID: &kbID, Enabled: true})

	out, err := NewUpdateSkillLogic(ctx, &svc.ServiceContext{SkillRepo: repo, KnowledgeBaseRepo: newFakeSkillKnowledgeBaseRepository()}).UpdateSkill(userID, &request.UpdateSkillRequest{
		UriSkillIDRequest: request.UriSkillIDRequest{ID: skillID.String()},
		Name:              stringPtr("  新名称  "),
		KBID:              stringPtr(""),
		Enabled:           boolPtr(false),
	})
	if err != nil {
		t.Fatalf("UpdateSkill error = %v", err)
	}
	if !slices.Equal(repo.updatedColumns, []string{"name", "kb_id", "enabled"}) {
		t.Fatalf("updated columns = %#v, want [name kb_id enabled]", repo.updatedColumns)
	}
	if repo.updatedPatch.Name != "新名称" || repo.updatedPatch.KBID != nil || repo.updatedPatch.Enabled {
		t.Fatalf("updated patch = %#v, want trimmed name, nil kb_id, disabled", repo.updatedPatch)
	}
	if out.Name != "新名称" || out.KBID != nil || out.Enabled {
		t.Fatalf("UpdateSkill response = %#v, want updated fields", out)
	}
}

// TestUpdateSkillRejectsEmptyPatch 验证空更新参数会返回 bad request。
func TestUpdateSkillRejectsEmptyPatch(t *testing.T) {
	_, err := NewUpdateSkillLogic(context.Background(), &svc.ServiceContext{SkillRepo: newFakeSkillRepository()}).UpdateSkill(uuid.New(), &request.UpdateSkillRequest{
		UriSkillIDRequest: request.UriSkillIDRequest{ID: uuid.New().String()},
	})
	if xerr.From(err).Kind != xerr.KindBadRequest {
		t.Fatalf("UpdateSkill empty patch kind = %v, want bad_request", xerr.From(err).Kind)
	}
}

// TestDeleteSkillDeletesByID 验证删除技能会解析 ID 并调用仓储删除。
func TestDeleteSkillDeletesByID(t *testing.T) {
	ctx := context.Background()
	userID := uuid.New()
	skillID := uuid.New()
	repo := newFakeSkillRepository(&models.Skill{ID: skillID, UserID: userID, Name: "技能"})

	if err := NewDeleteSkillLogic(ctx, &svc.ServiceContext{SkillRepo: repo}).DeleteSkill(userID, &request.UriSkillIDRequest{ID: skillID.String()}); err != nil {
		t.Fatalf("DeleteSkill error = %v", err)
	}
	if repo.deletedID != skillID {
		t.Fatalf("deletedID = %s, want %s", repo.deletedID, skillID)
	}
}

// TestOptimizeSkillPromptUsesDefaultChatModel 验证提示词优化会使用默认对话模型。
func TestOptimizeSkillPromptUsesDefaultChatModel(t *testing.T) {
	ctx := context.Background()
	userID := uuid.New()
	cipher := newSkillTestCipher(t)
	encrypted, err := cipher.Encrypt("sk-secret")
	if err != nil {
		t.Fatalf("Encrypt error = %v", err)
	}
	factory := &fakeSkillLLMFactory{client: &fakeSkillLLMClient{invokeText: "优化后提示词"}}
	manager := corellm.NewManager()
	manager.Register("fake", factory)
	modelRepo := &fakeSkillModelConfigRepository{rows: []*models.ModelConfig{
		{ID: uuid.New(), UserID: userID, Type: string(types.ChatModelType), Provider: "fake", ModelName: "chat-a", APIKeyEncrypted: encrypted},
		{ID: uuid.New(), UserID: userID, Type: string(types.ChatModelType), Provider: "fake", ModelName: "chat-b", APIKeyEncrypted: encrypted, IsDefault: true},
	}}
	promptManager := prompt.NewManager()
	if err := appprompts.Register(promptManager); err != nil {
		t.Fatalf("Register prompts error = %v, want nil", err)
	}

	out, err := NewOptimizeSkillPromptLogic(ctx, &svc.ServiceContext{
		ModelConfigRepo: modelRepo,
		SecretCipher:    cipher,
		LLMManager:      manager,
		PromptManager:   promptManager,
		PromptClient:    promptsgen.NewClient(promptManager),
	}).OptimizeSkillPrompt(userID, &request.OptimizeSkillPromptRequest{Prompt: "原始提示词"})
	if err != nil {
		t.Fatalf("OptimizeSkillPrompt error = %v", err)
	}
	if out.Optimized != "优化后提示词" {
		t.Fatalf("Optimized = %q, want 优化后提示词", out.Optimized)
	}
	if factory.got.Model != "chat-b" || factory.got.APIKey != "sk-secret" {
		t.Fatalf("factory config = %#v, want default model and decrypted key", factory.got)
	}
}

// TestOptimizeSkillPromptRejectsMissingModel 验证未配置对话模型时会返回 bad request。
func TestOptimizeSkillPromptRejectsMissingModel(t *testing.T) {
	_, err := NewOptimizeSkillPromptLogic(context.Background(), &svc.ServiceContext{
		ModelConfigRepo: &fakeSkillModelConfigRepository{},
		SecretCipher:    newSkillTestCipher(t),
		LLMManager:      corellm.NewManager(),
	}).OptimizeSkillPrompt(uuid.New(), &request.OptimizeSkillPromptRequest{Prompt: "prompt"})
	if xerr.From(err).Kind != xerr.KindBadRequest {
		t.Fatalf("OptimizeSkillPrompt missing model kind = %v, want bad_request", xerr.From(err).Kind)
	}
}

// TestBuiltinSkillEndpointsReturnNotImplemented 验证内置技能相关逻辑暂未实现。
func TestBuiltinSkillEndpointsReturnNotImplemented(t *testing.T) {
	ctx := context.Background()
	userID := uuid.New()
	skillID := uuid.New()
	if _, err := NewListBuiltinSkillsLogic(ctx, &svc.ServiceContext{}).ListBuiltinSkills(userID); xerr.From(err).Kind != xerr.KindBadRequest {
		t.Fatalf("ListBuiltinSkills kind = %v, want bad_request", xerr.From(err).Kind)
	}
	if _, err := NewCopyBuiltinSkillLogic(ctx, &svc.ServiceContext{}).CopyBuiltinSkill(userID, &request.UriSkillIDRequest{ID: skillID.String()}); xerr.From(err).Kind != xerr.KindBadRequest {
		t.Fatalf("CopyBuiltinSkill kind = %v, want bad_request", xerr.From(err).Kind)
	}
}

func stringPtr(v string) *string {
	return &v
}

func boolPtr(v bool) *bool {
	return &v
}

func newSkillTestCipher(t *testing.T) *security.SecretCipher {
	t.Helper()
	cipher, err := security.NewSecretCipher("0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatalf("NewSecretCipher error = %v", err)
	}
	return cipher
}

type fakeSkillRepository struct {
	rows           []*models.Skill
	created        *models.Skill
	updatedPatch   *models.Skill
	updatedColumns []string
	deletedID      uuid.UUID
}

func newFakeSkillRepository(rows ...*models.Skill) *fakeSkillRepository {
	return &fakeSkillRepository{rows: rows}
}

func (r *fakeSkillRepository) Create(ctx context.Context, userID uuid.UUID, row *models.Skill) (*models.Skill, error) {
	row.UserID = userID
	r.created = row
	r.rows = append(r.rows, row)
	return row, nil
}

func (r *fakeSkillRepository) List(ctx context.Context, userID uuid.UUID) ([]*models.Skill, error) {
	out := make([]*models.Skill, 0, len(r.rows))
	for _, row := range r.rows {
		if row.UserID == userID {
			out = append(out, row)
		}
	}
	return out, nil
}

func (r *fakeSkillRepository) FindByID(ctx context.Context, userID uuid.UUID, skillID uuid.UUID) (*models.Skill, error) {
	for _, row := range r.rows {
		if row.ID == skillID && row.UserID == userID {
			return row, nil
		}
	}
	return nil, xerr.NotFound("技能不存在")
}

func (r *fakeSkillRepository) Update(ctx context.Context, userID uuid.UUID, row *models.Skill) (*models.Skill, error) {
	return row, nil
}

func (r *fakeSkillRepository) UpdateFields(ctx context.Context, userID uuid.UUID, skillID uuid.UUID, patch *models.Skill, fields *repository.SkillUpdateFields) (*models.Skill, error) {
	existing, err := r.FindByID(ctx, userID, skillID)
	if err != nil {
		return nil, err
	}
	r.updatedPatch = patch
	r.updatedColumns = fields.Columns()
	for _, column := range r.updatedColumns {
		switch column {
		case "name":
			existing.Name = patch.Name
		case "description":
			existing.Description = patch.Description
		case "icon":
			existing.Icon = patch.Icon
		case "prompt":
			existing.Prompt = patch.Prompt
		case "tool_keys":
			existing.ToolKeys = patch.ToolKeys
		case "kb_id":
			existing.KBID = patch.KBID
		case "config":
			existing.Config = patch.Config
		case "enabled":
			existing.Enabled = patch.Enabled
		}
	}
	return existing, nil
}

func (r *fakeSkillRepository) Delete(ctx context.Context, userID uuid.UUID, skillID uuid.UUID) error {
	r.deletedID = skillID
	for i, row := range r.rows {
		if row.ID == skillID && row.UserID == userID {
			r.rows = append(r.rows[:i], r.rows[i+1:]...)
			return nil
		}
	}
	return xerr.NotFound("技能不存在")
}

type fakeSkillKnowledgeBaseRepository struct {
	rows []*models.KnowledgeBase
}

func newFakeSkillKnowledgeBaseRepository(rows ...*models.KnowledgeBase) *fakeSkillKnowledgeBaseRepository {
	return &fakeSkillKnowledgeBaseRepository{rows: rows}
}

func (r *fakeSkillKnowledgeBaseRepository) Create(ctx context.Context, userID uuid.UUID, row *models.KnowledgeBase) (*models.KnowledgeBase, error) {
	row.UserID = userID
	r.rows = append(r.rows, row)
	return row, nil
}

func (r *fakeSkillKnowledgeBaseRepository) List(ctx context.Context, userID uuid.UUID) ([]*models.KnowledgeBase, error) {
	return nil, nil
}

func (r *fakeSkillKnowledgeBaseRepository) FindDefault(ctx context.Context, userID uuid.UUID) (*models.KnowledgeBase, error) {
	return nil, xerr.NotFound("默认知识库不存在")
}

func (r *fakeSkillKnowledgeBaseRepository) FindByID(ctx context.Context, userID uuid.UUID, kbID uuid.UUID) (*models.KnowledgeBase, error) {
	for _, row := range r.rows {
		if row.ID == kbID && row.UserID == userID {
			return row, nil
		}
	}
	return nil, xerr.NotFound("知识库不存在")
}

func (r *fakeSkillKnowledgeBaseRepository) Update(ctx context.Context, userID uuid.UUID, row *models.KnowledgeBase) (*models.KnowledgeBase, error) {
	return row, nil
}

func (r *fakeSkillKnowledgeBaseRepository) UpdateFields(ctx context.Context, userID uuid.UUID, kbID uuid.UUID, row *models.KnowledgeBase, fields *repository.KnowledgeBaseUpdateFields) (*models.KnowledgeBase, error) {
	return row, nil
}

func (r *fakeSkillKnowledgeBaseRepository) Delete(ctx context.Context, userID uuid.UUID, kbID uuid.UUID) error {
	return nil
}

type fakeSkillModelConfigRepository struct {
	rows []*models.ModelConfig
}

func (r *fakeSkillModelConfigRepository) Create(ctx context.Context, row *models.ModelConfig) (*models.ModelConfig, error) {
	return row, nil
}

func (r *fakeSkillModelConfigRepository) Update(ctx context.Context, row *models.ModelConfig) (*models.ModelConfig, error) {
	return row, nil
}

func (r *fakeSkillModelConfigRepository) Delete(ctx context.Context, id uuid.UUID) error {
	return nil
}

func (r *fakeSkillModelConfigRepository) List(ctx context.Context, userID uuid.UUID, modelType *types.ModelType) ([]*models.ModelConfig, error) {
	out := make([]*models.ModelConfig, 0, len(r.rows))
	for _, row := range r.rows {
		if row.UserID == userID && (modelType == nil || row.Type == string(*modelType)) {
			out = append(out, row)
		}
	}
	return out, nil
}

func (r *fakeSkillModelConfigRepository) FindByID(ctx context.Context, userID uuid.UUID, configID uuid.UUID) (*models.ModelConfig, error) {
	return nil, xerr.NotFound("模型配置不存在")
}

type fakeSkillLLMFactory struct {
	client *fakeSkillLLMClient
	got    corellm.ModelConfig
}

func (f *fakeSkillLLMFactory) NewClient(cfg corellm.ModelConfig) (corellm.Client, error) {
	f.got = cfg
	return f.client, nil
}

type fakeSkillLLMClient struct {
	invokeText string
	invokeErr  error
}

func (c *fakeSkillLLMClient) Invoke(ctx context.Context, messages []*corellm.Message, opts ...corellm.ModelCallOption) (string, error) {
	if c.invokeErr != nil {
		return "", c.invokeErr
	}
	return c.invokeText, nil
}

func (c *fakeSkillLLMClient) InvokeResult(ctx context.Context, messages []*corellm.Message, opts ...corellm.ModelCallOption) (*corellm.LLMResult, error) {
	text, err := c.Invoke(ctx, messages, opts...)
	if err != nil {
		return nil, err
	}
	return &corellm.LLMResult{Text: text}, nil
}

func (c *fakeSkillLLMClient) Stream(ctx context.Context, messages []*corellm.Message, opts ...corellm.ModelCallOption) (<-chan string, error) {
	ch := make(chan string)
	close(ch)
	return ch, nil
}

func (c *fakeSkillLLMClient) Embed(ctx context.Context, texts []string, dimensions int, opts ...corellm.EmbeddingOption) ([][]float64, error) {
	return nil, errors.New("not implemented")
}

func (c *fakeSkillLLMClient) EmbedOne(ctx context.Context, text string, dimensions int) ([]float64, error) {
	return nil, errors.New("not implemented")
}
