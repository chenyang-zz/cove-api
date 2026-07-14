/**
 * @Time   : 2026/6/28 14:47
 * @Author : chenyangzhao542@gmail.com
 * @File   : agent_persona.go
 **/

package models

import (
	"time"

	"github.com/google/uuid"
)

// DefaultPersona 新用户默认角色：亲切、会用记忆/知识库/联网的全能助手，也是写人设的范例。
// Identity 描述「是谁 / 能做什么」；Soul 描述「怎么说 / 性格语气」。
// 旧列 system_prompt 由 AutoMigrate 保留时可用 SQL 迁到 identity，本模型不再读写该列。
var DefaultPersona = AgentPersona{
	Name: "小盒",
	Identity: `
你是「小盒」，用户的专属 AI 助手。
你拥有用户的知识库、长期记忆和联网搜索能力，需要时主动调用；涉及实时信息（新闻、价格、天气等）时优先联网核实，不凭记忆编造。
你可以在「角色配置」里被修改成任意人设——这条只是默认示例。
`,
	Soul: `
性格亲切、耐心、靠谱。
回答先抓重点，再按需展开，不啰嗦。
拿不准或信息不足时如实说明，不杜撰。
语气温暖自然，像一个懂用户、记得住事的朋友。
`,
	Temperature: 0.7,
	IsActive:    true,
}

type AgentPersona struct {
	ID          uuid.UUID `gorm:"column:id;type:uuid;primaryKey"`
	UserID      uuid.UUID `gorm:"column:user_id;type:uuid;not null;index"`
	User        User      `gorm:"foreignKey:UserID;references:ID;constraint:OnDelete:CASCADE"`
	Name        string    `gorm:"column:name;size:64;not null"`
	AvatarKey   string    `gorm:"column:avatar_key;size:512"`
	Identity    string    `gorm:"column:identity;type:text"`                   // 身份设定：是谁、角色定位、能力边界
	Soul        string    `gorm:"column:soul;type:text"`                       // 性格灵魂：语气、风格、口头禅
	Temperature float64   `gorm:"column:temperature;not null;default:0.7"`     // 不使用
	IsActive    bool      `gorm:"column:is_active;not null;default:false"`     // 是否当前生效（每用户最多一条 true）
	InGroupOnly bool      `gorm:"column:in_group_only;not null;default:false"` // 仅作为角色卡组成员存在（如内置场景拉入的角色），不在「单个角色」列表单独展示
	Sort        int       `gorm:"column:sort;not null;default:0"`              // 列表排序（预留）
	CreatedAt   time.Time `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt   time.Time `gorm:"column:updated_at;autoUpdateTime"`
}

func (AgentPersona) TableName() string {
	return "agent_personas"
}
