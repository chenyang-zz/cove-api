package models

import (
	"database/sql/driver"
	"reflect"
	"strings"
	"testing"
)

func TestUserTableName(t *testing.T) {
	if got := (User{}).TableName(); got != "users" {
		t.Fatalf("TableName = %q, want users", got)
	}
}

func TestRefreshTokenTableName(t *testing.T) {
	if got := (RefreshToken{}).TableName(); got != "refresh_tokens" {
		t.Fatalf("TableName = %q, want refresh_tokens", got)
	}
}

func TestModelConfigTableName(t *testing.T) {
	if got := (ModelConfig{}).TableName(); got != "model_configs" {
		t.Fatalf("TableName = %q, want model_configs", got)
	}
}

func TestUserGormTags(t *testing.T) {
	userType := reflect.TypeOf(User{})
	tests := map[string][]string{
		"ID":             {"type:uuid", "primaryKey"},
		"Username":       {"column:username", "size:64", "uniqueIndex", "not null"},
		"Nickname":       {"column:nickname", "size:64"},
		"Email":          {"column:email", "size:255", "uniqueIndex"},
		"Avatar":         {"column:avatar", "size:512"},
		"PasswordHash":   {"column:password_hash", "size:255", "not null"},
		"BriefingSeenAt": {"column:briefing_seen_at"},
		"CreatedAt":      {"column:created_at", "autoCreateTime"},
		"UpdatedAt":      {"column:updated_at", "autoUpdateTime"},
	}
	for fieldName, wantParts := range tests {
		field, ok := userType.FieldByName(fieldName)
		if !ok {
			t.Fatalf("missing field %s", fieldName)
		}
		tag := field.Tag.Get("gorm")
		for _, want := range wantParts {
			if !strings.Contains(tag, want) {
				t.Fatalf("%s gorm tag = %q, want to contain %q", fieldName, tag, want)
			}
		}
	}
}

func TestModelConfigGormTags(t *testing.T) {
	modelType := reflect.TypeOf(ModelConfig{})
	tests := map[string][]string{
		"ID":              {"column:id", "type:uuid", "primaryKey"},
		"UserID":          {"column:user_id", "type:uuid", "not null", "index"},
		"User":            {"foreignKey:UserID", "references:ID", "OnDelete:CASCADE"},
		"Type":            {"column:type", "size:32", "index", "not null"},
		"Provider":        {"column:provider", "size:32", "not null"},
		"Name":            {"column:name", "size:128", "not null"},
		"ModelName":       {"column:model_name", "size:128", "not null"},
		"APIKeyEncrypted": {"column:api_key_encrypted", "size:512", "not null"},
		"BaseURL":         {"column:base_url", "size:255", "not null"},
		"Capability":      {"column:capability", "type:jsonb"},
		"IsDefault":       {"column:is_default", "default:false"},
		"CreatedAt":       {"column:created_at", "autoCreateTime"},
		"UpdatedAt":       {"column:updated_at", "autoUpdateTime"},
	}
	for fieldName, wantParts := range tests {
		field, ok := modelType.FieldByName(fieldName)
		if !ok {
			t.Fatalf("missing field %s", fieldName)
		}
		tag := field.Tag.Get("gorm")
		for _, want := range wantParts {
			if !strings.Contains(tag, want) {
				t.Fatalf("%s gorm tag = %q, want to contain %q", fieldName, tag, want)
			}
		}
	}
}

func TestStringListScansAndValuesJSON(t *testing.T) {
	var list StringList
	if err := list.Scan([]byte(`["function_call","vision"]`)); err != nil {
		t.Fatalf("Scan error = %v", err)
	}
	if len(list) != 2 || list[0] != "function_call" || list[1] != "vision" {
		t.Fatalf("list = %#v", list)
	}
	value, err := list.Value()
	if err != nil {
		t.Fatalf("Value error = %v", err)
	}
	if _, ok := value.(driver.Value); !ok {
		t.Fatalf("value type = %T, want driver.Value", value)
	}
	if value != `["function_call","vision"]` {
		t.Fatalf("value = %v", value)
	}
}

func TestRefreshTokenGormTags(t *testing.T) {
	tokenType := reflect.TypeOf(RefreshToken{})
	tests := map[string][]string{
		"ID":        {"column:id", "type:uuid", "primaryKey"},
		"UserID":    {"column:user_id", "type:uuid", "not null", "index"},
		"User":      {"foreignKey:UserID", "references:ID", "OnDelete:CASCADE"},
		"TokenHash": {"column:token_hash", "size:128", "uniqueIndex", "not null"},
		"ExpiresAt": {"column:expires_at", "not null", "index"},
		"RevokedAt": {"column:revoked_at", "index"},
		"CreatedAt": {"column:created_at", "autoCreateTime"},
		"UpdatedAt": {"column:updated_at", "autoUpdateTime"},
	}
	for fieldName, wantParts := range tests {
		field, ok := tokenType.FieldByName(fieldName)
		if !ok {
			t.Fatalf("missing field %s", fieldName)
		}
		tag := field.Tag.Get("gorm")
		for _, want := range wantParts {
			if !strings.Contains(tag, want) {
				t.Fatalf("%s gorm tag = %q, want to contain %q", fieldName, tag, want)
			}
		}
	}
}
