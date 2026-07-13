/*
 * @Time   : 2026-07-12 20:24:16
 * @Author : chenyang
 * @File   : image.go
 */

package response

import (
	"time"

	"github.com/google/uuid"
)

type ImageResponse struct {
	ID          uuid.UUID  `json:"id"`
	KBID        *uuid.UUID `json:"kb_id"`
	FileName    string     `json:"file_name"`
	FileExt     string     `json:"file_ext"`
	FileSize    int64      `json:"file_size"`
	Url         string     `json:"url"`
	Description string     `json:"description"`
	Objects     []string   `json:"objects"`
	Scene       *string    `json:"scene"`
	Tags        []string   `json:"tags"`
	Status      string     `json:"status"`
	Progress    float64    `json:"progress"`
	ErrorMsg    *string    `json:"error_msg"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

type SearchImageResponse struct {
	ChunkID    uuid.UUID  `json:"chunk_id"`
	Content    string     `json:"content"`
	ImageName  string     `json:"image_name"`
	SourceID   uuid.UUID  `json:"source_id"`
	SourceType string     `json:"source_type"`
	KBID       *uuid.UUID `json:"kb_id"`
	Score      float64    `json:"score"`
}
