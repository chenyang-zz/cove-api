/**
 * @Time   : 2026/6/30 21:43
 * @Author : chenyangzhao542@gmail.com
 * @File   : document.go
 **/

package request

import "mime/multipart"

type URLImportRequest struct {
	Url  string  `json:"url" binding:"required,url"`
	KBID *string `json:"kb_id" binding:"omitempty,uuid"` // 归属知识库
}

type UploadDocumentRequest struct {
	File *multipart.FileHeader `form:"file" binding:"required"`
	KBID *string               `form:"kb_id" binding:"omitempty,uuid"`
}

type UriDocumentIDRequest struct {
	DocumentID string `uri:"doc_id" binding:"required,uuid"`
}

type ListDocumentsRequest struct {
	PageRequest
	Tag  *string `json:"tag" form:"tag" binding:"omitempty"`          // 按标签名筛选
	KBID *string `json:"kb_id" form:"kb_id" binding:"omitempty,uuid"` // 按知识库筛选
}

type SearchDocumentsRequest struct {
	Query string   `json:"query" binding:"required,min=1"`
	TopK  int64    `json:"top_k" binding:"required,gte=1,lte=20"` // default: 5
	Tags  []string `json:"tags" binding:"omitempty"`
}

type MoveDocumentRequest struct {
	UriDocumentIDRequest
	KBID string `json:"kb_id" form:"kb_id" binding:"required,uuid"`
}
