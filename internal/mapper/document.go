package mapper

import (
	"github.com/boxify/api-go/internal/models"
	"github.com/boxify/api-go/internal/transport/http/response"
)

func DocumentToResponse(row *models.Document, tags []string) *response.DocumentResponse {
	if row == nil {
		return nil
	}
	if tags == nil {
		tags = documentTagNames(row.Tags)
	}
	return &response.DocumentResponse{
		ID:         row.ID,
		KBID:       row.KBID,
		FileName:   row.FileName,
		FileExt:    row.FileExt,
		FileSize:   row.FileSize,
		SourceType: row.SourceType,
		SourceUrl:  row.SourceUrl,
		Status:     row.Status,
		Progress:   row.Progress,
		ChunkNum:   row.ChunkNum,
		ErrorMsg:   row.ErrorMsg,
		Tags:       tags,
		CreatedAt:  row.CreatedAt,
		UpdatedAt:  row.UpdatedAt,
	}
}

func documentTagNames(rows []models.Tag) []string {
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.Name)
	}
	return out
}
