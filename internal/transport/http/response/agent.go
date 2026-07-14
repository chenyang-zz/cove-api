/**
 * @Time   : 2026/6/28 15:19
 * @Author : chenyangzhao542@gmail.com
 * @File   : agent.go
 **/

package response

import (
	"time"

	"github.com/boxify/api-go/internal/models"
	"github.com/google/uuid"
)

type AgentConfigResponse struct {
	SystemPrompt       string  `json:"system_prompt"`
	Temperature        float64 `json:"temperature"`
	EnableKnowledge    bool    `json:"enable_knowledge"`
	EnableMemory       bool    `json:"enable_memory"`
	EnableWebSearch    bool    `json:"enable_web_search"`
	EnableActiveRecall bool    `json:"enable_active_recall"`
	EnableCrossSession bool    `json:"enalbe_cross_session"`
	ShowAvatar         bool    `json:"show_avatar"`
	HumanMode          bool    `json:"human_mode"`
}

type AgentPersonaResponse struct {
	ID          uuid.UUID `json:"id"`
	Name        string    `json:"name"`
	AvatarKey   string    `json:"avatar_key"`
	AvatarUrl   string    `json:"avatar_url"`
	Identity    string    `json:"identity"`
	Soul        string    `json:"soul"`
	Temperature float64   `json:"temperature"`
	IsActive    bool      `json:"is_active"`
}

type AgentTaskResponse struct {
	ID                   uuid.UUID          `json:"id"`
	Name                 string             `json:"name"`
	Instruction          string             `json:"instruction"`
	KBIDs                []string           `json:"kbi_ds"`
	TriggerType          models.TriggerType `json:"trigger_type"`
	TriggerTime          string             `json:"trigger_time"`
	TriggerWeekday       int                `json:"trigger_weekday"`
	TriggerIntervalHours int                `json:"trigger_interval_hours"`
	Enabled              bool               `json:"enabled"`
	NotifyEnabled        bool               `json:"notify_enabled"`
	LastRunAt            *time.Time         `json:"last_run_at"`
	LastStatus           models.TaskRunType `json:"last_status"`
	NextRunAt            *time.Time         `json:"next_run_at"`
	CreatedAt            time.Time          `json:"created_at"`
	UpdatedAt            time.Time          `json:"updated_at"`
}

type OptimizePromptResponse struct {
	Optimized string `json:"optimized"`
}
