package http_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/boxify/api-go/internal/infrastructure/security"
	"github.com/google/uuid"
)

func TestCreateModelConfigValidationErrorsIncludeFieldDetails(t *testing.T) {
	router := newTestRouter(t)
	token, err := security.NewTokenIssuer("test-secret", time.Hour).IssueAccessToken(uuid.New())
	if err != nil {
		t.Fatalf("issue access token: %v", err)
	}

	tests := []struct {
		name      string
		body      string
		wantField string
		wantTag   string
	}{
		{
			name:      "missing api key",
			body:      `{"type":"chat","provider":"deepseek","name":"DeepSeek Chat","model_name":"deepseek-chat","base_url":"https://api.deepseek.com"}`,
			wantField: "api_key",
			wantTag:   "required",
		},
		{
			name:      "invalid provider",
			body:      `{"type":"chat","provider":"bad-provider","name":"DeepSeek Chat","model_name":"deepseek-chat","base_url":"https://api.deepseek.com","api_key":"sk-secret"}`,
			wantField: "provider",
			wantTag:   "oneof",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/model-configs/create", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer "+token)
			router.ServeHTTP(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
			}
			var got struct {
				Code    int    `json:"code"`
				Message string `json:"message"`
				Errors  []struct {
					Field   string `json:"field"`
					Tag     string `json:"tag"`
					Param   string `json:"param"`
					Message string `json:"message"`
				} `json:"errors"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
				t.Fatalf("Unmarshal response: %v", err)
			}
			if got.Code != 40001 || got.Message != "请求参数错误" {
				t.Fatalf("response = %+v, want validation envelope", got)
			}
			if !hasValidationError(got.Errors, tt.wantField, tt.wantTag) {
				t.Fatalf("errors = %+v, want %s/%s", got.Errors, tt.wantField, tt.wantTag)
			}
			if strings.Contains(rec.Body.String(), "Key:") || strings.Contains(rec.Body.String(), "Error:Field validation") {
				t.Fatalf("response leaked raw validator error: %s", rec.Body.String())
			}
		})
	}
}

func hasValidationError(errors []struct {
	Field   string `json:"field"`
	Tag     string `json:"tag"`
	Param   string `json:"param"`
	Message string `json:"message"`
}, field, tag string) bool {
	for _, item := range errors {
		if item.Field == field && item.Tag == tag && item.Message != "" {
			return true
		}
	}
	return false
}
