package models

import (
	"time"

	"github.com/google/uuid"
)

type RefreshToken struct {
	ID        uuid.UUID  `gorm:"column:id;type:uuid;primaryKey"`
	UserID    uuid.UUID  `gorm:"column:user_id;type:uuid;not null;index"`
	User      User       `gorm:"foreignKey:UserID;references:ID;constraint:OnDelete:CASCADE"`
	TokenHash string     `gorm:"column:token_hash;size:128;uniqueIndex;not null"`
	ExpiresAt time.Time  `gorm:"column:expires_at;not null;index"`
	RevokedAt *time.Time `gorm:"column:revoked_at;index"`
	CreatedAt time.Time  `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt time.Time  `gorm:"column:updated_at;autoUpdateTime"`
}

func (RefreshToken) TableName() string {
	return "refresh_tokens"
}
