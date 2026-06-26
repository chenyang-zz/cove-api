package response

import (
	"time"

	"github.com/google/uuid"
)

type ModelResponse struct {
	ID           uuid.UUID `json:"id"`
	Type         string    `json:"type"`
	Provider     string    `json:"provider"`
	Name         string    `json:"name"`
	ModelName    string    `json:"model_name"`
	APIKeyMasked string    `json:"api_key_masked"`
	BaseURL      string    `json:"base_url"`
	Capability   []string  `json:"capability"`
	IsDefault    bool      `json:"is_default"`
	CreatedAt    time.Time `json:"created_at"`
}

type ListModelsResponse struct {
	List []*ModelResponse `json:"list"`
}
