/**
 * @Time   : 2026/6/28 15:43
 * @Author : chenyangzhao542@gmail.com
 * @File   : agent.go
 **/

package request

type UpdateAgentConfigRequest struct {
	SystemPrompt       *string  `json:"system_prompt" binding:"omitempty,max=4000"`
	Temperature        *float64 `json:"temperature" binding:"omitempty,gte=0.0,lte=2.0"`
	EnableKnowledge    *bool    `json:"enable_knowledge" binding:"omitempty"`
	EnableMemory       *bool    `json:"enable_memory" binding:"omitempty"`
	EnableWebSearch    *bool    `json:"enable_web_search" binding:"omitempty"`
	EnableActiveRecall *bool    `json:"enable_active_recall" binding:"omitempty"`
	EnableCrossSession *bool    `json:"enalbe_cross_session" binding:"omitempty"`
	ShowAvatar         *bool    `json:"show_avatar" binding:"omitempty"`
	HumanMode          *bool    `json:"human_mode" binding:"omitempty"`
}

type OptimizePromptRequest struct {
	SystemPrompt string `json:"system_prompt" binding:"required,min=1,max=4000"`
}

type ListAgentPersonasRequest struct {
	All bool `json:"all"`
}

type CreateAgentPersonaRequest struct {
	Name        string   `json:"name" binding:"required,min=1,max=64"`
	AvatarKey   string   `json:"avatar_key" binding:"omitempty,max=512"`
	Identity    string   `json:"identity" binding:"omitempty,max=4000"`
	Soul        string   `json:"soul" binding:"omitempty,max=4000"`
	Temperature *float64 `json:"temperature" binding:"omitempty,gte=0.0,lte=2.0"`
}

type UriAgentPersonaIDRequest struct {
	PersonaID string `uri:"persona_id" binding:"required"`
}

type UpdateAgentPersonaRequest struct {
	UriAgentPersonaIDRequest
	Name        *string  `json:"name" binding:"omitempty,min=1,max=64"`
	AvatarKey   *string  `json:"avatar_key" binding:"omitempty,max=512"`
	Identity    *string  `json:"identity" binding:"omitempty,max=4000"`
	Soul        *string  `json:"soul" binding:"omitempty,max=4000"`
	Temperature *float64 `json:"temperature" binding:"omitempty,gte=0.0,lte=2.0"`
}
