package mapper

import (
	"github.com/boxify/api-go/internal/models"
	"github.com/boxify/api-go/internal/transport/http/response"
)

func ImageToResponse(row *models.Image, tags []string, url string) *response.ImageResponse {
	if row == nil {
		return nil
	}
	if tags == nil {
		tags = imageTagNames(row.Tags)
	}
	objects := []string(row.Objects)
	if objects == nil {
		objects = []string{}
	}
	return &response.ImageResponse{
		ID:          row.ID,
		KBID:        row.KBID,
		FileName:    row.FileName,
		FileExt:     row.FileExt,
		FileSize:    row.FileSize,
		Url:         url,
		Description: derefString(row.Description),
		Objects:     objects,
		Scene:       row.Scene,
		Tags:        tags,
		Status:      row.Status,
		Progress:    row.Progress,
		ErrorMsg:    row.ErrorMsg,
		CreatedAt:   row.CreatedAt,
		UpdatedAt:   row.UpdatedAt,
	}
}

func imageTagNames(rows []models.Tag) []string {
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.Name)
	}
	return out
}

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
