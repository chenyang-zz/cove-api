package mapper_test

import (
	"testing"

	domainskills "github.com/boxify/api-go/internal/domain/skills"
	"github.com/boxify/api-go/internal/mapper"
	"github.com/boxify/api-go/internal/models"
	"github.com/boxify/api-go/internal/transport/http/request"
	"github.com/google/uuid"
)

// TestSkillConfigFromRequestMapsTypedConfig 验证 mapper 会把请求 DTO 转换为模型层技能配置。
func TestSkillConfigFromRequestMapsTypedConfig(t *testing.T) {
	got := mapper.SkillConfigFromRequest(&request.SkillConfig{
		QuickPrompt: []string{"问题"},
		FewShots:    []request.FewShot{{Input: "输入", Output: "输出"}},
	})
	if len(got.QuickPrompt) != 1 || got.QuickPrompt[0] != "问题" ||
		len(got.FewShots) != 1 || got.FewShots[0].Input != "输入" || got.FewShots[0].Output != "输出" {
		t.Fatalf("SkillConfigFromRequest = %+v, want typed model config", got)
	}
}

// TestSkillToResponseReturnsTypedConfig 验证技能响应直接映射模型层结构化配置。
func TestSkillToResponseReturnsTypedConfig(t *testing.T) {
	row := &models.Skill{
		ID:   uuid.New(),
		Name: "技能",
		Config: models.SkillConfig{
			QuickPrompt: []string{"问题"},
			FewShots:    []models.SkillFewShot{{Input: "输入", Output: "输出"}},
		},
	}
	got := mapper.SkillToResponse(row)
	var requestConfig *request.SkillConfig = got.Config
	if got.Config == nil || len(got.Config.QuickPrompt) != 1 || got.Config.QuickPrompt[0] != "问题" ||
		len(got.Config.FewShots) != 1 || got.Config.FewShots[0].Output != "输出" {
		t.Fatalf("SkillToResponse Config = %+v, want typed response config", got.Config)
	}
	if requestConfig.FewShots[0].Input != "输入" {
		t.Fatalf("SkillToResponse request config = %+v, want converted transport config", requestConfig)
	}
}

// TestSkillConfigFromDomainReturnsModelCopy 验证领域层配置会转换为模型层配置并隔离切片副本。
func TestSkillConfigFromDomainReturnsModelCopy(t *testing.T) {
	input := domainskills.Config{
		QuickPrompt: []string{"问题"},
		FewShots:    []domainskills.FewShot{{Input: "输入", Output: "输出"}},
	}
	got := mapper.SkillConfigFromDomain(input)
	input.QuickPrompt[0] = "已修改"
	input.FewShots[0].Input = "已修改"

	if len(got.QuickPrompt) != 1 || got.QuickPrompt[0] != "问题" ||
		len(got.FewShots) != 1 || got.FewShots[0].Input != "输入" || got.FewShots[0].Output != "输出" {
		t.Fatalf("SkillConfigFromDomain = %+v, want independent model copy", got)
	}
}

// TestSkillConfigResponseFromDomainMapsConfig 验证领域层配置会直接转换为传输层配置。
func TestSkillConfigResponseFromDomainMapsConfig(t *testing.T) {
	input := domainskills.Config{
		QuickPrompt: []string{"问题"},
		FewShots:    []domainskills.FewShot{{Input: "输入", Output: "输出"}},
	}
	got := mapper.SkillConfigResponseFromDomain(input)
	input.QuickPrompt[0] = "已修改"
	input.FewShots[0].Output = "已修改"

	if got == nil || len(got.QuickPrompt) != 1 || got.QuickPrompt[0] != "问题" ||
		len(got.FewShots) != 1 || got.FewShots[0].Input != "输入" || got.FewShots[0].Output != "输出" {
		t.Fatalf("SkillConfigResponseFromDomain = %+v, want independent response config", got)
	}
}

// TestBuiltinSkillToResponseMapsTemplate 验证内置技能模板会转换为传输层响应。
func TestBuiltinSkillToResponseMapsTemplate(t *testing.T) {
	id := uuid.New()
	template := domainskills.Template{
		ID:          id,
		Key:         "demo",
		Name:        "内置技能",
		Description: "说明",
		Icon:        "icon",
		Prompt:      "提示词",
		ToolKeys:    []string{"tool"},
		Enabled:     true,
		Config: domainskills.Config{
			QuickPrompt: []string{"问题"},
			FewShots:    []domainskills.FewShot{{Input: "输入", Output: "输出"}},
		},
	}
	got := mapper.BuiltinSkillToResponse(template)

	if got.ID != id || got.Key != "demo" || got.Name != "内置技能" || !got.IsBuiltin || !got.Enabled {
		t.Fatalf("BuiltinSkillToResponse = %#v, want builtin template fields", got)
	}
	if len(got.ToolKeys) != 1 || got.ToolKeys[0] != "tool" {
		t.Fatalf("BuiltinSkillToResponse ToolKeys = %#v, want tool", got.ToolKeys)
	}
	if got.Config == nil || len(got.Config.QuickPrompt) != 1 || got.Config.QuickPrompt[0] != "问题" ||
		len(got.Config.FewShots) != 1 || got.Config.FewShots[0].Output != "输出" {
		t.Fatalf("BuiltinSkillToResponse Config = %#v, want converted config", got.Config)
	}
}

// TestBuiltinSkillToModelMapsTemplate 验证内置技能模板会转换为用户可持久化的模型记录。
func TestBuiltinSkillToModelMapsTemplate(t *testing.T) {
	userID := uuid.New()
	skillID := uuid.New()
	got := mapper.BuiltinSkillToModel(domainskills.Template{
		ID:          uuid.New(),
		Key:         "demo",
		Name:        "内置技能",
		Description: "说明",
		Icon:        "icon",
		Prompt:      "提示词",
		ToolKeys:    []string{"tool"},
		Enabled:     true,
		Config: domainskills.Config{
			QuickPrompt: []string{"问题"},
			FewShots:    []domainskills.FewShot{{Input: "输入", Output: "输出"}},
		},
	}, userID, skillID)

	if got.ID != skillID || got.UserID != userID || got.Name != "内置技能" || !got.IsBuiltin || !got.Enabled {
		t.Fatalf("BuiltinSkillToModel = %#v, want persisted user skill fields", got)
	}
	if len(got.ToolKeys) != 1 || got.ToolKeys[0] != "tool" {
		t.Fatalf("BuiltinSkillToModel ToolKeys = %#v, want tool", got.ToolKeys)
	}
	if len(got.Config.QuickPrompt) != 1 || got.Config.QuickPrompt[0] != "问题" ||
		len(got.Config.FewShots) != 1 || got.Config.FewShots[0].Input != "输入" {
		t.Fatalf("BuiltinSkillToModel Config = %#v, want converted config", got.Config)
	}
}

// TestSkillFromCreateRequestMapsDefaults 验证创建请求会转换为模型记录并应用默认值。
func TestSkillFromCreateRequestMapsDefaults(t *testing.T) {
	userID := uuid.New()
	skillID := uuid.New()
	kbID := uuid.New()
	got := mapper.SkillFromCreateRequest(&request.CreateSkillRequest{
		Name:        " 技能 ",
		Description: " 说明 ",
		Icon:        " ",
		Prompt:      " 提示词 ",
		ToolKeys:    []string{" search ", ""},
		Config:      &request.SkillConfig{QuickPrompt: []string{"问题"}},
	}, userID, skillID, &kbID, "default")

	if got.ID != skillID || got.UserID != userID || got.Name != "技能" || got.Description != "说明" {
		t.Fatalf("SkillFromCreateRequest = %#v, want trimmed model fields", got)
	}
	if got.Icon != "default" || got.Prompt != "提示词" || !got.Enabled || got.IsBuiltin {
		t.Fatalf("SkillFromCreateRequest defaults = %#v, want default icon enabled non-builtin", got)
	}
	if got.KBID == nil || *got.KBID != kbID || len(got.ToolKeys) != 1 || got.ToolKeys[0] != "search" {
		t.Fatalf("SkillFromCreateRequest associations = %#v, want kb and normalized tools", got)
	}
}
