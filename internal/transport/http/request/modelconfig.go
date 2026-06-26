package request

type ListModelsRequest struct {
	Type string `json:"type" form:"type" binding:"omitempty,oneof=chat multimodal embedding rerank websearch asr"`
}

type CreateModelRequest struct {
	Type       string   `json:"type" form:"type" binding:"required,oneof=chat multimodal embedding rerank websearch asr"`
	Provider   string   `json:"provider" form:"provider" binding:"required,oneof=openai qwen doubao deepseek zhipu qianfan tavily"`
	Name       string   `json:"name" form:"name" binding:"required,min=1,max=128"`
	ModelName  string   `json:"model_name" form:"model_name" binding:"required,min=1,max=128"`
	ApiKey     string   `json:"api_key" form:"api_key" binding:"required,min=1"`
	BaseUrl    string   `json:"base_url" form:"base_url" binding:"required,min=1,max=255"`
	Capability []string `json:"capability" form:"capability" binding:"omitempty"`
	IsDefault  bool     `json:"is_default" form:"is_default,default=false"`
}
