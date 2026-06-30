/**
 * @Time   : 2026/6/30 21:43
 * @Author : chenyangzhao542@gmail.com
 * @File   : document.go
 **/

package response

import (
	"time"

	"github.com/google/uuid"
)

type DocumentResponse struct {
	ID         uuid.UUID  `json:"id"`
	KBID       *uuid.UUID `json:"kb_id"`
	FileName   string     `json:"file_name"`
	FileExt    string     `json:"file_ext"`
	FileSize   int64      `json:"file_size"`
	SourceType string     `json:"source_type"`
	SourceUrl  *string    `json:"source_url"`
	Status     string     `json:"status"`
	Progress   float64    `json:"progress"`
	ChunkNum   int64      `json:"chunk_num"`
	ErrorMsg   *string    `json:"error_msg"`
	Tags       []string   `json:"tags"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

type PreviewDocumentResponse struct {
	ID         uuid.UUID `json:"id"`
	FileName   string    `json:"file_name"`
	FileExt    string    `json:"file_ext"`
	IsMarkdown bool      `json:"is_markdown"`
	SourceUrl  *string   `json:"source_url"`
	Content    string    `json:"content"`
	Truncated  bool      `json:"truncated"`
}

type DocumentStatusResponse struct {
	Status   string  `json:"status"`
	Progress float64 `json:"progress"`
	ErrorMsg *string `json:"error_msg"`
}

type SearchDocumentResponse struct {
	ChunkID    uuid.UUID  `json:"chunk_id"`
	Content    string     `json:"content"`
	DocName    string     `json:"doc_name"`
	SourceID   uuid.UUID  `json:"source_id"`
	SourceType string     `json:"source_type"`
	KBID       *uuid.UUID `json:"kb_id"`
	Score      float64    `json:"score"`
}
