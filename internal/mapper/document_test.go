package mapper_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/boxify/api-go/internal/mapper"
	"github.com/boxify/api-go/internal/models"
)

func TestDocumentToResponseKeepsTimeFields(t *testing.T) {
	// 验证文档响应会保留 time.Time 类型的创建和更新时间，而不是提前格式化成字符串。
	createdAt := time.Date(2026, 7, 1, 9, 30, 0, 123, time.UTC)
	updatedAt := createdAt.Add(time.Hour)

	got := mapper.DocumentToResponse(&models.Document{
		FileName:  "a.txt",
		FileExt:   ".txt",
		CreatedAt: createdAt,
		UpdatedAt: updatedAt,
	}, nil)

	if reflect.TypeOf(got.CreatedAt) != reflect.TypeOf(time.Time{}) || reflect.TypeOf(got.UpdatedAt) != reflect.TypeOf(time.Time{}) {
		t.Fatalf("time field types = %T/%T, want time.Time/time.Time", got.CreatedAt, got.UpdatedAt)
	}
	if !reflect.DeepEqual(got.CreatedAt, createdAt) || !reflect.DeepEqual(got.UpdatedAt, updatedAt) {
		t.Fatalf("time fields = %v/%v, want %v/%v", got.CreatedAt, got.UpdatedAt, createdAt, updatedAt)
	}
}

func TestDocumentToResponseUsesModelTagsWhenTagsAreNotProvided(t *testing.T) {
	// 验证未显式传入标签名时，文档响应会从模型预加载的 Tags 里提取标签名称。
	got := mapper.DocumentToResponse(&models.Document{
		FileName: "a.txt",
		FileExt:  ".txt",
		Tags: []models.Tag{
			{Name: "重要"},
			{Name: "项目"},
		},
	}, nil)

	if !reflect.DeepEqual(got.Tags, []string{"重要", "项目"}) {
		t.Fatalf("tags = %#v, want model tag names", got.Tags)
	}
}
