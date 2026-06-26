package auth_test

import (
	"context"
	"testing"
	"time"

	"github.com/boxify/api-go/internal/infrastructure/security"
	"github.com/boxify/api-go/internal/logic/auth"
	"github.com/boxify/api-go/internal/models"
	"github.com/boxify/api-go/internal/repository"
	"github.com/boxify/api-go/internal/svc"
	"github.com/boxify/api-go/internal/transport/http/request"
	httpresponse "github.com/boxify/api-go/internal/transport/http/response"
	"github.com/boxify/api-go/internal/util"
	"github.com/boxify/api-go/internal/xerr"
	"github.com/google/uuid"
)

type fakeUserRepository struct {
	byID    map[uuid.UUID]*models.User
	byLogin map[string]*models.User
}

func newFakeUserRepository() *fakeUserRepository {
	return &fakeUserRepository{
		byID:    map[uuid.UUID]*models.User{},
		byLogin: map[string]*models.User{},
	}
}

func (r *fakeUserRepository) Create(ctx context.Context, user *models.User) (*models.User, error) {
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

func (r *fakeUserRepository) Update(ctx context.Context, user *models.User) (*models.User, error) {
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

func (r *fakeUserRepository) FindByLogin(ctx context.Context, login string) (*models.User, error) {
	user, ok := r.byLogin[login]
	if !ok {
		return nil, xerr.NotFound("用户不存在")
	}
	return user, nil
}

func (r *fakeUserRepository) FindByID(ctx context.Context, id uuid.UUID) (*models.User, error) {
	user, ok := r.byID[id]
	if !ok {
		return nil, xerr.NotFound("用户不存在")
	}
	return user, nil
}

type fakeRefreshTokenRepository struct {
	byHash map[string]*models.RefreshToken
}

func newFakeRefreshTokenRepository() *fakeRefreshTokenRepository {
	return &fakeRefreshTokenRepository{byHash: map[string]*models.RefreshToken{}}
}

func (r *fakeRefreshTokenRepository) Create(ctx context.Context, token *models.RefreshToken) (*models.RefreshToken, error) {
	if token.ID == uuid.Nil {
		token.ID = uuid.New()
	}
	r.byHash[token.TokenHash] = token
	return token, nil
}

func (r *fakeRefreshTokenRepository) FindByHash(ctx context.Context, hash string) (*models.RefreshToken, error) {
	token, ok := r.byHash[hash]
	if !ok {
		return nil, xerr.InvalidToken()
	}
	return token, nil
}

func (r *fakeRefreshTokenRepository) Revoke(ctx context.Context, id uuid.UUID, revokedAt time.Time) error {
	for hash, token := range r.byHash {
		if token.ID == id {
			token.RevokedAt = &revokedAt
			r.byHash[hash] = token
			return nil
		}
	}
	return xerr.InvalidToken()
}

func TestRegisterLogicRegistersUserAndReturnsRefreshToken(t *testing.T) {
	users := newFakeUserRepository()
	refreshTokens := newFakeRefreshTokenRepository()
	svcCtx := &svc.ServiceContext{
		UserRepo:         users,
		RefreshTokenRepo: refreshTokens,
		TokenIssuer:      security.NewTokenIssuer("test-secret", time.Hour),
	}

	out, err := auth.NewRegisterLogic(t.Context(), svcCtx).Register(&request.RegisterRequest{
		Username: "  Alice  ",
		Email:    ptr("  ALICE@example.COM  "),
		Password: "secret123",
	})
	if err != nil {
		t.Fatalf("Register error = %v", err)
	}
	var _ *httpresponse.AuthResponse = out
	if out.UserID == uuid.Nil {
		t.Fatal("user id is nil")
	}
	if out.Username != "alice" {
		t.Fatalf("username = %q, want alice", out.Username)
	}
	if out.Email == nil || *out.Email != "alice@example.com" {
		t.Fatalf("email = %v, want alice@example.com", out.Email)
	}
	if out.AccessToken == "" || out.RefreshToken == "" {
		t.Fatalf("tokens must be returned: access=%q refresh=%q", out.AccessToken, out.RefreshToken)
	}
	created := users.byID[out.UserID]
	if created.PasswordHash == "secret123" || !security.CheckPassword(created.PasswordHash, "secret123") {
		t.Fatal("password was not hashed correctly")
	}
}

func TestLoginLogicSupportsUsernameAndEmail(t *testing.T) {
	users := newFakeUserRepository()
	refreshTokens := newFakeRefreshTokenRepository()
	svcCtx := &svc.ServiceContext{
		UserRepo:         users,
		RefreshTokenRepo: refreshTokens,
		TokenIssuer:      security.NewTokenIssuer("test-secret", time.Hour),
	}
	registered, err := auth.NewRegisterLogic(t.Context(), svcCtx).Register(&request.RegisterRequest{
		Username: "alice",
		Email:    ptr("alice@example.com"),
		Password: "secret123",
	})
	if err != nil {
		t.Fatalf("Register error = %v", err)
	}

	for _, login := range []string{"alice", "alice@example.com"} {
		out, err := auth.NewLoginLogic(t.Context(), svcCtx).Login(&request.LoginRequest{
			Login:    login,
			Password: "secret123",
		})
		if err != nil {
			t.Fatalf("Login(%q) error = %v", login, err)
		}
		if out.UserID != registered.UserID || out.AccessToken == "" || out.RefreshToken == "" {
			t.Fatalf("Login(%q) output = %+v, want registered user with tokens", login, out)
		}
	}
}

func TestRefreshLogicRotatesTokenAndRejectsReuse(t *testing.T) {
	users := newFakeUserRepository()
	refreshTokens := newFakeRefreshTokenRepository()
	svcCtx := &svc.ServiceContext{
		UserRepo:         users,
		RefreshTokenRepo: refreshTokens,
		TokenIssuer:      security.NewTokenIssuer("test-secret", time.Hour),
	}
	registered, err := auth.NewRegisterLogic(t.Context(), svcCtx).Register(&request.RegisterRequest{
		Username: "alice",
		Password: "secret123",
	})
	if err != nil {
		t.Fatalf("Register error = %v", err)
	}

	refreshed, err := auth.NewRefreshLogic(t.Context(), svcCtx).Refresh(&request.RefreshRequest{RefreshToken: registered.RefreshToken})
	if err != nil {
		t.Fatalf("Refresh error = %v", err)
	}
	if refreshed.UserID != registered.UserID || refreshed.AccessToken == "" || refreshed.RefreshToken == "" {
		t.Fatalf("Refresh output = %+v, want same user with new tokens", refreshed)
	}
	if refreshed.RefreshToken == registered.RefreshToken {
		t.Fatal("refresh token was not rotated")
	}

	_, err = auth.NewRefreshLogic(t.Context(), svcCtx).Refresh(&request.RefreshRequest{RefreshToken: registered.RefreshToken})
	if xerr.From(err).Kind != xerr.KindUnauthorized {
		t.Fatalf("Refresh reused token error = %v, want unauthorized", err)
	}
}

func TestMeLogicReadsUserIDFromContext(t *testing.T) {
	users := newFakeUserRepository()
	svcCtx := &svc.ServiceContext{
		UserRepo: users,
	}
	user := &models.User{
		ID:           uuid.New(),
		Username:     "alice",
		PasswordHash: "hash",
	}
	users.byID[user.ID] = user
	users.byLogin[user.Username] = user

	out, err := auth.NewMeLogic(util.WithUserID(t.Context(), user.ID), svcCtx).Me()
	if err != nil {
		t.Fatalf("Me error = %v", err)
	}
	if out.ID != user.ID || out.Username != user.Username {
		t.Fatalf("Me = %+v, want user %s", out, user.ID)
	}
}

func TestMeLogicRequiresUserIDInContext(t *testing.T) {
	out, err := auth.NewMeLogic(t.Context(), &svc.ServiceContext{}).Me()
	if err == nil {
		t.Fatalf("Me error = nil, out = %+v", out)
	}
	if xerr.From(err).Kind != xerr.KindUnauthorized {
		t.Fatalf("Me error = %v, want unauthorized", err)
	}
}

func ptr(value string) *string {
	return &value
}

var _ repository.UserRepository = (*fakeUserRepository)(nil)
var _ repository.RefreshTokenRepository = (*fakeRefreshTokenRepository)(nil)
