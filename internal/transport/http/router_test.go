package http_test

import (
	"bufio"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/boxify/api-go/internal/domain"
	"github.com/boxify/api-go/internal/infrastructure/security"
	"github.com/boxify/api-go/internal/models"
	"github.com/boxify/api-go/internal/svc"
	httptransport "github.com/boxify/api-go/internal/transport/http"
	"github.com/boxify/api-go/internal/xerr"
	"github.com/google/uuid"
)

func newTestRouter(t *testing.T, enableDebugPanicRoute ...bool) http.Handler {
	t.Helper()
	cipher, err := security.NewSecretCipher("0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatalf("new cipher: %v", err)
	}
	svcCtx := &svc.ServiceContext{
		UserRepo:         newTestUserRepository(),
		RefreshTokenRepo: newTestRefreshTokenRepository(),
		ModelConfigRepo:  &testModelConfigRepository{},
		SecretCipher:     cipher,
		TokenIssuer:      security.NewTokenIssuer("test-secret", time.Hour),
	}
	deps := httptransport.Dependencies{
		Svc: svcCtx,
	}
	if len(enableDebugPanicRoute) > 0 {
		deps.EnableDebugPanicRoute = enableDebugPanicRoute[0]
	}
	return httptransport.NewRouter(deps)
}

type testModelConfigRepository struct {
	rows []*models.ModelConfig
}

func (r *testModelConfigRepository) Create(ctx context.Context, row *models.ModelConfig) (*models.ModelConfig, error) {
	r.rows = append(r.rows, row)
	return row, nil
}

func (r *testModelConfigRepository) List(ctx context.Context, userID uuid.UUID, modelType *domain.ModelType) ([]*models.ModelConfig, error) {
	out := make([]*models.ModelConfig, 0, len(r.rows))
	for _, row := range r.rows {
		if row.UserID == userID && (modelType == nil || row.Type == string(*modelType)) {
			out = append(out, row)
		}
	}
	return out, nil
}

type testUserRepository struct {
	byID    map[uuid.UUID]*models.User
	byLogin map[string]*models.User
}

func newTestUserRepository() *testUserRepository {
	return &testUserRepository{
		byID:    map[uuid.UUID]*models.User{},
		byLogin: map[string]*models.User{},
	}
}

func (r *testUserRepository) Create(ctx context.Context, user *models.User) (*models.User, error) {
	if user.ID == uuid.Nil {
		user.ID = uuid.New()
	}
	if _, ok := r.byLogin[user.Username]; ok {
		return nil, xerr.UserExists()
	}
	if user.Email != nil {
		if _, ok := r.byLogin[*user.Email]; ok {
			return nil, xerr.UserExists()
		}
	}
	r.byID[user.ID] = user
	r.byLogin[user.Username] = user
	if user.Email != nil {
		r.byLogin[*user.Email] = user
	}
	return user, nil
}

func (r *testUserRepository) Update(ctx context.Context, user *models.User) (*models.User, error) {
	if _, ok := r.byID[user.ID]; !ok {
		return nil, xerr.NotFound("用户不存在")
	}
	r.byID[user.ID] = user
	r.byLogin[user.Username] = user
	if user.Email != nil {
		r.byLogin[*user.Email] = user
	}
	return user, nil
}

func (r *testUserRepository) FindByLogin(ctx context.Context, login string) (*models.User, error) {
	user, ok := r.byLogin[login]
	if !ok {
		return nil, xerr.NotFound("用户不存在")
	}
	return user, nil
}

func (r *testUserRepository) FindByID(ctx context.Context, id uuid.UUID) (*models.User, error) {
	user, ok := r.byID[id]
	if !ok {
		return nil, xerr.NotFound("用户不存在")
	}
	return user, nil
}

type testRefreshTokenRepository struct {
	byHash map[string]*models.RefreshToken
}

func newTestRefreshTokenRepository() *testRefreshTokenRepository {
	return &testRefreshTokenRepository{byHash: map[string]*models.RefreshToken{}}
}

func (r *testRefreshTokenRepository) Create(ctx context.Context, token *models.RefreshToken) (*models.RefreshToken, error) {
	if token.ID == uuid.Nil {
		token.ID = uuid.New()
	}
	r.byHash[token.TokenHash] = token
	return token, nil
}

func (r *testRefreshTokenRepository) FindByHash(ctx context.Context, hash string) (*models.RefreshToken, error) {
	token, ok := r.byHash[hash]
	if !ok {
		return nil, xerr.InvalidToken()
	}
	return token, nil
}

func (r *testRefreshTokenRepository) Revoke(ctx context.Context, id uuid.UUID, revokedAt time.Time) error {
	for hash, token := range r.byHash {
		if token.ID == id {
			token.RevokedAt = &revokedAt
			r.byHash[hash] = token
			return nil
		}
	}
	return xerr.InvalidToken()
}

func TestRouterHealthUsesUnifiedResponse(t *testing.T) {
	router := newTestRouter(t)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if got := strings.TrimSpace(w.Body.String()); got != `{"code":0,"message":"ok","data":{"status":"ok"}}` {
		t.Fatalf("body = %s", got)
	}
}

func TestRouterRequiresExplicitDependencies(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("NewRouter did not panic for missing dependencies")
		}
	}()
	_ = httptransport.NewRouter(httptransport.Dependencies{})
}

func TestProtectedRouteRequiresBearerToken(t *testing.T) {
	router := newTestRouter(t)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
	if !strings.Contains(w.Body.String(), `"code":40100`) {
		t.Fatalf("body = %s, want auth error code", w.Body.String())
	}
}

func TestChatStreamSetsSSEHeadersAndEvents(t *testing.T) {
	router := newTestRouter(t)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/chat/stream", strings.NewReader(`{"message":"hello"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer dev-token")
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", w.Code, http.StatusOK, w.Body.String())
	}
	if got := w.Header().Get("Content-Type"); !strings.HasPrefix(got, "text/event-stream") {
		t.Fatalf("content-type = %q, want text/event-stream", got)
	}

	scanner := bufio.NewScanner(strings.NewReader(w.Body.String()))
	events := map[string]bool{}
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "event: ") {
			events[strings.TrimPrefix(line, "event: ")] = true
		}
	}
	for _, name := range []string{"meta", "token", "done"} {
		if !events[name] {
			t.Fatalf("missing SSE event %q in body:\n%s", name, w.Body.String())
		}
	}
}
