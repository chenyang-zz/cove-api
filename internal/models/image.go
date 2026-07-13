/**
 * @Time   : 2026/6/30 21:34
 * @Author : chenyangzhao542@gmail.com
 * @File   : image.go
 **/

package models

import (
	"time"

	"github.com/google/uuid"
)

// Image ORM 模型 —— PostgreSQL images 表（图片元数据）。
//
// 原始图片存对象存储（file_key），多模态模型生成的描述向量进 ES，可被搜索

type Image struct {
	ID          uuid.UUID  `gorm:"column:id;type:uuid;primaryKey"`
	UserID      uuid.UUID  `gorm:"column:user_id;type:uuid;not null;index"`
	KBID        *uuid.UUID `gorm:"column:kb_id;type:uuid;index"` // 所属知识库（多知识库分类）。删库时整库资料一并删除，故 CASCADE
	FileName    string     `gorm:"column:file_name;size:512;not null"`
	FileExt     string     `gorm:"column:file_ext;size:16;not null"`
	FileSize    int64      `gorm:"column:file_size;not null"`
	FileKey     string     `gorm:"column:file_key;size:512;not null"`
	Description *string     `gorm:"column:description;type:text"`                           // AI 详细描述
	OCRText     *string     `gorm:"column:ocr_text;type:text"`                              // 图中文字
	Objects     JSONStrings `gorm:"column:objects;type:jsonb"`                              // 物体列表（字符串数组）
	Scene       *string     `gorm:"column:scene;size:256"`                                  // 场景
	Status      string      `gorm:"column:status;size:16;not null;default:'pending';index"` // pending | processing | done | failed
	Progress    float64     `gorm:"column:progress;not null;default:0"`                     // 解析进度 0~1
	ErrorMsg    *string     `gorm:"column:error_msg;type:text"`
	CreatedAt   time.Time   `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt   time.Time   `gorm:"column:updated_at;autoUpdateTime"`

	User          User          `gorm:"foreignKey:UserID;references:ID;constraint:OnDelete:CASCADE"`
	KnowledgeBase KnowledgeBase `gorm:"foreignKey:KBID;references:ID;constraint:OnDelete:CASCADE"`
	Tags          []Tag         `gorm:"many2many:image_tags;constraint:OnDelete:CASCADE;"`
}

func (Image) TableName() string {
	return "images"
}
