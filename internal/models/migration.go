package models

// MigrationModels 返回需要由 GORM AutoMigrate 管理的持久化模型。
//
// 返回值每次都会创建新的切片，调用方可以安全地追加或调整切片内容，
// 不会影响后续迁移执行。
func MigrationModels() []any {
	return []any{
		&User{},
		&RefreshToken{},
		&ModelConfig{},
		&Conversation{},
		&ConversationContextState{},
		&Message{},
		&MessageFeedback{},
		&AgentConfig{},
		&AgentPersona{},
		&AgentTask{},
		&MCPServer{},
		&KnowledgeBase{},
		&Document{},
		&Image{},
		&Tag{},
		&Skill{},
		&ToolConfig{},
	}
}
