package middleware_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/boxify/api-go/internal/transport/http/middleware"
	"github.com/boxify/api-go/internal/util"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type fixedTokenVerifier struct {
	userID uuid.UUID
}

func (v fixedTokenVerifier) VerifyAccessToken(ctx context.Context, token string) (uuid.UUID, error) {
	return v.userID, nil
}

func TestAuthStoresUserIDInRequestContext(t *testing.T) {
	gin.SetMode(gin.TestMode)
	userID := uuid.New()
	router := gin.New()
	router.Use(middleware.Auth(fixedTokenVerifier{userID: userID}))
	router.GET("/ctx-user", func(c *gin.Context) {
		got, err := util.UserIDFromContext(c.Request.Context())
		if err != nil {
			c.Status(http.StatusInternalServerError)
			return
		}
		c.String(http.StatusOK, got.String())
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ctx-user", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
	if got := w.Body.String(); got != userID.String() {
		t.Fatalf("body = %q, want %q", got, userID.String())
	}
}
