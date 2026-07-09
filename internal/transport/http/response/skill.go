/**
 * @Time   : 2026/7/9 18:34
 * @Author : chenyangzhao542@gmail.com
 * @File   : skill.go
 **/

package response

import (
	"github.com/boxify/api-go/internal/transport/http/request"
	"github.com/google/uuid"
)

type SkillResponse struct {
	ID          uuid.UUID            `json:"id"`
	Key         string               `json:"key,omitempty"`
	Name        string               `json:"name"`
	Description string               `json:"description"`
	Icon        string               `json:"icon"`
	Prompt      string               `json:"prompt"`
	ToolKeys    []string             `json:"tool_keys"`
	KBID        *uuid.UUID           `json:"kb_id"`
	Enabled     bool                 `json:"enabled"`
	Config      *request.SkillConfig `json:"config"`
	IsBuiltin   bool                 `json:"is_builtin"`
}
