/**
 * @Time   : 2026/6/30 22:24
 * @Author : chenyangzhao542@gmail.com
 * @File   : tag.go
 **/

package models

import (
	"time"

	"github.com/google/uuid"
)

// Tag ORM 模型 + 文档/图片关联表。

type Tag struct {
	ID        uuid.UUID `gorm:"column:id;type:uuid;primaryKey"`
	UserID    uuid.UUID `gorm:"column:user_id;type:uuid;not null;index;uniqueIndex:uq_tag_user_name"`
	Name      string    `gorm:"column:name;size:64;not null;uniqueIndex:uq_tag_user_name""`
	Color     string    `gorm:"column:color;size:16;not null;default:'#155EEF'"`
	CreatedAt time.Time `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt time.Time `gorm:"column:updated_at;autoUpdateTime"`

	User      User       `gorm:"foreignKey:UserID;references:ID;constraint:OnDelete:CASCADE"`
	Documents []Document `gorm:"many2many:document_tags;constraint:OnDelete:CASCADE;"`
	Images    []Image    `gorm:"many2many:image_tags;constraint:OnDelete:CASCADE;"`
}

func (Tag) TableName() string {
	return "tags"
}
