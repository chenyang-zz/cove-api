package mapper

import (
	"strings"

	domainskills "github.com/boxify/api-go/internal/domain/skills"
	"github.com/boxify/api-go/internal/models"
	"github.com/boxify/api-go/internal/transport/http/request"
	"github.com/boxify/api-go/internal/transport/http/response"
	"github.com/google/uuid"
)

// SkillConfigFromRequest 将技能配置请求转换为模型配置，并规范化文本内容。
func SkillConfigFromRequest(input *request.SkillConfig) models.SkillConfig {
	if input == nil {
		return models.SkillConfig{}
	}
	out := models.SkillConfig{
		QuickPrompt: make([]string, 0, len(input.QuickPrompt)),
		FewShots:    make([]models.SkillFewShot, 0, len(input.FewShots)),
	}
	for _, prompt := range input.QuickPrompt {
		prompt = strings.TrimSpace(prompt)
		if prompt != "" {
			out.QuickPrompt = append(out.QuickPrompt, prompt)
		}
	}
	for _, shot := range input.FewShots {
		in := strings.TrimSpace(shot.Input)
		output := strings.TrimSpace(shot.Output)
		if in == "" && output == "" {
			continue
		}
		out.FewShots = append(out.FewShots, models.SkillFewShot{Input: in, Output: output})
	}
	return out
}

// SkillToolKeysFromRequest 将请求中的工具键列表转换为模型层字符串列表。
func SkillToolKeysFromRequest(values []string) models.StringList {
	out := make(models.StringList, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		out = append(out, value)
	}
	return out
}

// SkillConfigToResponse 将模型层技能配置转换为传输层配置。
func SkillConfigToResponse(input models.SkillConfig) *request.SkillConfig {
	if len(input.QuickPrompt) == 0 && len(input.FewShots) == 0 {
		return nil
	}
	out := &request.SkillConfig{
		QuickPrompt: append([]string(nil), input.QuickPrompt...),
		FewShots:    make([]request.FewShot, 0, len(input.FewShots)),
	}
	for _, shot := range input.FewShots {
		out.FewShots = append(out.FewShots, request.FewShot{
			Input:  shot.Input,
			Output: shot.Output,
		})
	}
	return out
}

// SkillConfigFromDomain 将领域层技能配置转换为模型层配置，并返回独立副本。
func SkillConfigFromDomain(input domainskills.Config) models.SkillConfig {
	return models.SkillConfig{
		QuickPrompt: append([]string(nil), input.QuickPrompt...),
		FewShots:    skillFewShotsFromDomain(input.FewShots),
	}
}

// SkillConfigResponseFromDomain 将领域层技能配置直接转换为传输层配置。
func SkillConfigResponseFromDomain(input domainskills.Config) *request.SkillConfig {
	if len(input.QuickPrompt) == 0 && len(input.FewShots) == 0 {
		return nil
	}
	out := &request.SkillConfig{
		QuickPrompt: append([]string(nil), input.QuickPrompt...),
		FewShots:    make([]request.FewShot, 0, len(input.FewShots)),
	}
	for _, shot := range input.FewShots {
		out.FewShots = append(out.FewShots, request.FewShot{
			Input:  shot.Input,
			Output: shot.Output,
		})
	}
	return out
}

func skillFewShotsFromDomain(values []domainskills.FewShot) []models.SkillFewShot {
	if values == nil {
		return nil
	}
	out := make([]models.SkillFewShot, 0, len(values))
	for _, value := range values {
		out = append(out, models.SkillFewShot{
			Input:  value.Input,
			Output: value.Output,
		})
	}
	return out
}

// BuiltinSkillToResponse 将领域层内置技能模板转换为传输层响应。
func BuiltinSkillToResponse(input domainskills.Template) *response.SkillResponse {
	return &response.SkillResponse{
		ID:          input.ID,
		Key:         input.Key,
		Name:        input.Name,
		Description: input.Description,
		Icon:        input.Icon,
		Prompt:      input.Prompt,
		ToolKeys:    append([]string(nil), input.ToolKeys...),
		KBID:        nil,
		Enabled:     input.Enabled,
		Config:      SkillConfigResponseFromDomain(input.Config),
		IsBuiltin:   true,
	}
}

// BuiltinSkillsToListResponse 将领域层内置技能模板列表转换为列表响应。
func BuiltinSkillsToListResponse(rows []domainskills.Template) *response.ListResponse[*response.SkillResponse] {
	out := make([]*response.SkillResponse, 0, len(rows))
	for _, row := range rows {
		out = append(out, BuiltinSkillToResponse(row))
	}
	return &response.ListResponse[*response.SkillResponse]{List: out}
}

// SkillFromCreateRequest 将创建技能请求转换为模型记录，并应用默认值。
func SkillFromCreateRequest(input *request.CreateSkillRequest, userID uuid.UUID, skillID uuid.UUID, kbID *uuid.UUID, defaultIcon string) *models.Skill {
	if input == nil {
		input = &request.CreateSkillRequest{}
	}
	icon := strings.TrimSpace(input.Icon)
	if icon == "" {
		icon = defaultIcon
	}
	enabled := true
	if input.Enabled != nil {
		enabled = *input.Enabled
	}
	return &models.Skill{
		ID:          skillID,
		UserID:      userID,
		Name:        strings.TrimSpace(input.Name),
		Description: strings.TrimSpace(input.Description),
		Icon:        icon,
		Prompt:      strings.TrimSpace(input.Prompt),
		ToolKeys:    SkillToolKeysFromRequest(input.ToolKeys),
		KBID:        kbID,
		Config:      SkillConfigFromRequest(input.Config),
		Enabled:     enabled,
		IsBuiltin:   false,
		Sort:        0,
	}
}

// BuiltinSkillToModel 将领域层内置技能模板转换为用户可持久化的模型记录。
func BuiltinSkillToModel(input domainskills.Template, userID uuid.UUID, skillID uuid.UUID) *models.Skill {
	return &models.Skill{
		ID:          skillID,
		UserID:      userID,
		Name:        input.Name,
		Description: input.Description,
		Icon:        input.Icon,
		Prompt:      input.Prompt,
		ToolKeys:    models.StringList(append([]string(nil), input.ToolKeys...)),
		KBID:        nil,
		Config:      SkillConfigFromDomain(input.Config),
		Enabled:     input.Enabled,
		IsBuiltin:   true,
		Sort:        0,
	}
}

// SkillToResponse 将模型层技能记录转换为传输层响应。
func SkillToResponse(row *models.Skill) *response.SkillResponse {
	if row == nil {
		return nil
	}
	return &response.SkillResponse{
		ID:          row.ID,
		Name:        row.Name,
		Description: row.Description,
		Icon:        row.Icon,
		Prompt:      row.Prompt,
		ToolKeys:    []string(row.ToolKeys),
		KBID:        row.KBID,
		Enabled:     row.Enabled,
		Config:      SkillConfigToResponse(row.Config),
		IsBuiltin:   row.IsBuiltin,
	}
}

// SkillsToListResponse 将模型层技能记录列表转换为列表响应。
func SkillsToListResponse(rows []*models.Skill) *response.ListResponse[*response.SkillResponse] {
	out := make([]*response.SkillResponse, 0, len(rows))
	for _, row := range rows {
		if row == nil {
			continue
		}
		out = append(out, SkillToResponse(row))
	}
	return &response.ListResponse[*response.SkillResponse]{List: out}
}
