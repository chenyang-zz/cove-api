package models

import (
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func ensureUUID(id *uuid.UUID) {
	if *id == uuid.Nil {
		*id = uuid.New()
	}
}

func (c *Conversation) BeforeCreate(tx *gorm.DB) error {
	ensureUUID(&c.ID)
	return nil
}

func (u *User) BeforeCreate(tx *gorm.DB) error {
	ensureUUID(&u.ID)
	return nil
}

func (r *RefreshToken) BeforeCreate(tx *gorm.DB) error {
	ensureUUID(&r.ID)
	return nil
}

func (m *ModelConfig) BeforeCreate(tx *gorm.DB) error {
	ensureUUID(&m.ID)
	return nil
}

func (m *Message) BeforeCreate(tx *gorm.DB) error {
	ensureUUID(&m.ID)
	return nil
}

func (a *AgentConfig) BeforeCreate(tx *gorm.DB) error {
	ensureUUID(&a.ID)
	return nil
}

func (a *AgentPersona) BeforeCreate(tx *gorm.DB) error {
	ensureUUID(&a.ID)
	return nil
}

func (a *AgentTask) BeforeCreate(tx *gorm.DB) error {
	ensureUUID(&a.ID)
	return nil
}

func (k *KnowledgeBase) BeforeCreate(tx *gorm.DB) error {
	ensureUUID(&k.ID)
	return nil
}

func (s *Skill) BeforeCreate(tx *gorm.DB) error {
	ensureUUID(&s.ID)
	return nil
}

func (m *MCPServer) BeforeCreate(tx *gorm.DB) error {
	ensureUUID(&m.ID)
	return nil
}

func (d *Document) BeforeCreate(tx *gorm.DB) error {
	ensureUUID(&d.ID)
	return nil
}

func (i *Image) BeforeCreate(tx *gorm.DB) error {
	ensureUUID(&i.ID)
	return nil
}

func (t *Tag) BeforeCreate(tx *gorm.DB) error {
	ensureUUID(&t.ID)
	return nil
}
