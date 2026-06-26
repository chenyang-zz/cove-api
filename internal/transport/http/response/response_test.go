package response

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/boxify/api-go/internal/xerr"
	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"github.com/go-playground/validator/v10"
)

func TestFromErrorIncludesValidationFieldErrors(t *testing.T) {
	type createRequest struct {
		APIKey   string `json:"api_key" binding:"required"`
		Provider string `json:"provider" binding:"oneof=openai deepseek"`
	}
	registerValidationTagNames()
	err := binding.Validator.ValidateStruct(createRequest{Provider: "invalid"})

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	FromError(c, xerr.Validation(err))

	var got Envelope
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("Unmarshal response: %v", err)
	}
	if got.Code != 40001 || got.Message != "请求参数错误" {
		t.Fatalf("envelope = %+v, want validation envelope", got)
	}
	if len(got.Errors) != 2 {
		t.Fatalf("errors len = %d, want 2: %+v", len(got.Errors), got.Errors)
	}
	assertFieldError(t, got.Errors, "api_key", "required", "")
	assertFieldError(t, got.Errors, "provider", "oneof", "openai deepseek")
	if bytes.Contains(w.Body.Bytes(), []byte("Key:")) {
		t.Fatalf("response leaked raw validator error: %s", w.Body.String())
	}
}

func TestFromErrorIncludesJSONBindingErrors(t *testing.T) {
	for _, tt := range []struct {
		name      string
		err       error
		wantField string
		wantTag   string
	}{
		{
			name:    "syntax",
			err:     &json.SyntaxError{Offset: 3},
			wantTag: "json",
		},
		{
			name:      "type",
			err:       &json.UnmarshalTypeError{Field: "api_key", Value: "number", Type: nil},
			wantField: "api_key",
			wantTag:   "type",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			FromError(c, xerr.Validation(tt.err))

			var got Envelope
			if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
				t.Fatalf("Unmarshal response: %v", err)
			}
			assertFieldError(t, got.Errors, tt.wantField, tt.wantTag, "")
		})
	}
}

func TestFromErrorDoesNotIncludeErrorsForNonValidationError(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	FromError(c, xerr.Unauthorized("请先登录"))

	var got Envelope
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("Unmarshal response: %v", err)
	}
	if len(got.Errors) != 0 {
		t.Fatalf("errors = %+v, want empty", got.Errors)
	}
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func assertFieldError(t *testing.T, errors []FieldError, field, tag, param string) {
	t.Helper()
	for _, item := range errors {
		if item.Field == field && item.Tag == tag {
			if param != "" && item.Param != param {
				t.Fatalf("field error %+v param, want %q", item, param)
			}
			if item.Message == "" {
				t.Fatalf("field error %+v missing message", item)
			}
			return
		}
	}
	t.Fatalf("field error %s/%s not found in %+v", field, tag, errors)
}

func registerValidationTagNames() {
	if v, ok := binding.Validator.Engine().(*validator.Validate); ok {
		v.RegisterTagNameFunc(jsonTagName)
	}
}
