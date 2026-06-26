package models

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

type StringList []string

func (l StringList) Value() (driver.Value, error) {
	if l == nil {
		return "[]", nil
	}
	data, err := json.Marshal([]string(l))
	if err != nil {
		return nil, err
	}
	return string(data), nil
}

func (l *StringList) Scan(value any) error {
	if l == nil {
		return fmt.Errorf("StringList scan target is nil")
	}
	switch v := value.(type) {
	case nil:
		*l = StringList{}
		return nil
	case []byte:
		return json.Unmarshal(v, l)
	case string:
		return json.Unmarshal([]byte(v), l)
	default:
		return fmt.Errorf("unsupported StringList scan type %T", value)
	}
}

type ModelConfig struct {
	ID              uuid.UUID  `gorm:"column:id;type:uuid;primaryKey"`
	UserID          uuid.UUID  `gorm:"column:user_id;type:uuid;not null;index"`
	User            User       `gorm:"foreignKey:UserID;references:ID;constraint:OnDelete:CASCADE"`
	Type            string     `gorm:"column:type;size:32;not null;index"`
	Provider        string     `gorm:"column:provider;size:32;not null"`
	Name            string     `gorm:"column:name;uniqueIndex;size:128;not null"`
	ModelName       string     `gorm:"column:model_name;size:128;not null"`
	APIKeyEncrypted string     `gorm:"column:api_key_encrypted;size:512;not null"`
	BaseURL         string     `gorm:"column:base_url;size:255;not null"`
	Capability      StringList `gorm:"column:capability;type:jsonb;not null;default:'[]'"`
	IsDefault       bool       `gorm:"column:is_default;not null;default:false"`
	CreatedAt       time.Time  `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt       time.Time  `gorm:"column:updated_at;autoUpdateTime"`
}

func (ModelConfig) TableName() string {
	return "model_configs"
}
