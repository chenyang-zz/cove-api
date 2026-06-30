/**
 * @Time   : 2026/6/30 21:26
 * @Author : chenyangzhao542@gmail.com
 * @File   : document.go
 **/

package models

import (
	"time"

	"github.com/google/uuid"
)

// Document ORM 模型 —— PostgreSQL documents 表（文档元数据）。
//
// 原始文件存对象存储（file_key），解析后的 chunk 向量进 ES，本表只存元数据与解析状态。

type Document struct {
	ID         uuid.UUID  `gorm:"column:id;type:uuid;primaryKey"`
	UserID     uuid.UUID  `gorm:"column:user_id;type:uuid;not null;index"`
	KBID       *uuid.UUID `gorm:"column:kb_id;type:uuid;index"` // 所属知识库（多知识库分类）。删库时整库资料一并删除，故 CASCAD
	FileName   string     `gorm:"column:file_name;size:512;not null"`
	FileExt    string     `gorm:"column:file_ext;size:16;not null"`
	FileSize   int64      `gorm:"column:file_size;not null"`
	FileKey    string     `gorm:"column:file_key;size:512;not null"`                  // 对象存储中的 key
	SourceType string     `gorm:"column:source_type;size:16;not null;default:'file'"` // file | url
	SourceUrl  *string    `gorm:"column:source_url;size:1024"`
	Status     string     `gorm:"column:status;size:16;not null;default:'pending';index"` // pending | parsing | done | failed
	Progress   float64    `gorm:"column:progress;not null;default:0"`
	ChunkNum   int64      `gorm:"column:chunk_num;not null"`
	ErrorMsg   *string    `gorm:"column:error_msg;type:text"`
	CreatedAt  time.Time  `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt  time.Time  `gorm:"column:updated_at;autoUpdateTime"`

	User          User          `gorm:"foreignKey:UserID;references:ID;constraint:OnDelete:CASCADE"`
	KnowledgeBase KnowledgeBase `gorm:"foreignKey:KBID;references:ID;constraint:OnDelete:CASCADE"`
	Tags          []Tag         `gorm:"many2many:document_tags;constraint:OnDelete:CASCADE;"`
}

func (Document) TableName() string {
	return "documents"
}
