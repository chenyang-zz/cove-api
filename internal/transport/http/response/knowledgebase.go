/**
 * @Time   : 2026/6/30 17:58
 * @Author : chenyangzhao542@gmail.com
 * @File   : knowledgebase.go
 **/

package response

import (
	"time"

	"github.com/google/uuid"
)

type KnowledgeBaseResponse struct {
	ID          uuid.UUID `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Icon        string    `json:"icon"`
	Color       string    `json:"color"`
	IsDefault   bool      `json:"is_default"`
	ChatEnabled bool      `json:"chat_enabled"`
	DocCount    int64     `json:"doc_count"`
	ImageCount  int64     `json:"image_count"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}
