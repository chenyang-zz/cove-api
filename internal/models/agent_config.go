/**
 * @Time   : 2026/6/28 15:08
 * @Author : chenyangzhao542@gmail.com
 * @File   : agent.go
 **/

package models

import (
	"time"

	"github.com/google/uuid"
)

type AgentConfig struct {
	ID                         uuid.UUID `gorm:"column:id;type:uuid;primaryKey"`
	UserID                     uuid.UUID `gorm:"column:user_id;type:uuid;not null;index"`
	User                       User      `gorm:"foreignKey:UserID;references:ID;constraint:OnDelete:CASCADE"`
	SystemPrompt               string    `gorm:"column:system_prompt;type:text"` // 自定义系统提示词（人设/风格），问答时作为 system message 注入
	Temperature                float64   `gorm:"column:temperature;not null;default:0.7"`
	EnableKnowledge            bool      `gorm:"column:enable_knowledge;not null;default:true"` // 工具默认开关（联网搜索默认关，知识库/记忆默认开）
	EnableMemory               bool      `gorm:"column:enable_memory;not null;default:true"`
	EnableWebSearch            bool      `gorm:"column:enable_web_search;not null;default:false"`
	EnableActiveRecall         bool      `gorm:"column:enable_active_recall;not null;default:true"`  // 主动记忆：每轮提问自动召回相关记忆 + 洞察注入上下文（默认开）
	EnableCrossSession         bool      `gorm:"column:enable_cross_session;not null;default:false"` // 跨会话上下文：注入最近其他会话的摘要，让跨会话也能接着聊（默认关）
	ShowAvatar                 bool      `gorm:"column:show_avatar;not null;default:false"`          // 对话界面是否显示头像（开 → AI 人格头像 + 用户头像；关 → 两边都不显示）
	HumanMode                  bool      `gorm:"column:human_mode;not null;default:false"`           // 真人对话模式（全局）：开启后单聊/群聊都像真人微信聊天（口语短句、可多气泡），关闭恢复助手风格
	ContextEnabled             bool      `gorm:"column:context_enabled;not null;default:true"`
	ContextWindowTokens        int       `gorm:"column:context_window_tokens;not null;default:32768"`
	ContextOutputReserveTokens int       `gorm:"column:context_output_reserve_tokens;not null;default:4096"`
	ContextSafetyMarginTokens  int       `gorm:"column:context_safety_margin_tokens;not null;default:512"`
	ContextTriggerRatio        float64   `gorm:"column:context_trigger_ratio;not null;default:0.8"`
	ContextTargetRatio         float64   `gorm:"column:context_target_ratio;not null;default:0.6"`
	ContextKeepRecentTokens    int       `gorm:"column:context_keep_recent_tokens;not null;default:8192"`
	ContextSummaryMaxTokens    int64     `gorm:"column:context_summary_max_tokens;not null;default:1024"`
	CreatedAt                  time.Time `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt                  time.Time `gorm:"column:updated_at;autoUpdateTime"`
}

func (AgentConfig) TableName() string {
	return "agent_configs"
}
