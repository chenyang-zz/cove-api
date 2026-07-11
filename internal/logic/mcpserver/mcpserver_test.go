package mcpserver

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	coremcp "github.com/boxify/api-go/internal/core/mcp"
	"github.com/boxify/api-go/internal/infrastructure/security"
	"github.com/boxify/api-go/internal/models"
	"github.com/boxify/api-go/internal/repository"
	"github.com/boxify/api-go/internal/svc"
	"github.com/boxify/api-go/internal/transport/http/request"
	"github.com/boxify/api-go/internal/xerr"
	"github.com/google/uuid"
)

type fakeMCPServerRepository struct {
	rows          map[uuid.UUID]*models.MCPServer
	created       *models.MCPServer
	listUserID    uuid.UUID
	deletedID     uuid.UUID
	updated       *models.MCPServer
	partial       *models.MCPServer
	fields        []string
	updateID      uuid.UUID
	updateFieldsN int
}

func newFakeMCPServerRepository(rows ...*models.MCPServer) *fakeMCPServerRepository {
	repo := &fakeMCPServerRepository{rows: map[uuid.UUID]*models.MCPServer{}}
	for _, row := range rows {
		repo.rows[row.ID] = row
	}
	return repo
}

func (r *fakeMCPServerRepository) Create(ctx context.Context, userID uuid.UUID, row *models.MCPServer) (*models.MCPServer, error) {
	if row.ID == uuid.Nil {
		row.ID = uuid.New()
	}
	row.UserID = userID
	r.created = row
	r.rows[row.ID] = row
	return row, nil
}

func (r *fakeMCPServerRepository) List(ctx context.Context, userID uuid.UUID) ([]*models.MCPServer, error) {
	r.listUserID = userID
	out := make([]*models.MCPServer, 0, len(r.rows))
	for _, row := range r.rows {
		if row.UserID == userID {
			out = append(out, row)
		}
	}
	return out, nil
}

func (r *fakeMCPServerRepository) FindByID(ctx context.Context, userID uuid.UUID, mcpServerID uuid.UUID) (*models.MCPServer, error) {
	row, ok := r.rows[mcpServerID]
	if !ok || row.UserID != userID {
		return nil, xerr.NotFound("MCP服务不存在")
	}
	return row, nil
}

func (r *fakeMCPServerRepository) Update(ctx context.Context, userID uuid.UUID, row *models.MCPServer) (*models.MCPServer, error) {
	r.updated = row
	return row, nil
}

func (r *fakeMCPServerRepository) UpdateFields(ctx context.Context, userID uuid.UUID, mcpServerID uuid.UUID, row *models.MCPServer, fields *repository.MCPServerUpdateFields) (*models.MCPServer, error) {
	r.updateFieldsN++
	r.updateID = mcpServerID
	r.partial = row
	r.fields = fields.Columns()
	if len(r.fields) == 0 {
		return nil, xerr.BadRequest("更新字段不能为空")
	}
	existing, err := r.FindByID(ctx, userID, mcpServerID)
	if err != nil {
		return nil, err
	}
	for _, column := range r.fields {
		switch column {
		case "name":
			existing.Name = row.Name
		case "transport":
			existing.Transport = row.Transport
		case "url":
			existing.Url = row.Url
		case "auth_type":
			existing.AuthType = row.AuthType
		case "auth_config":
			existing.AuthConfig = row.AuthConfig
		case "enabled":
			existing.Enabled = row.Enabled
		case "status":
			existing.Status = row.Status
		case "last_error":
			existing.LastError = row.LastError
		case "tools_cache":
			existing.ToolsCache = row.ToolsCache
		case "synced_at":
			existing.SyncedAt = row.SyncedAt
		}
	}
	return existing, nil
}

func (r *fakeMCPServerRepository) Delete(ctx context.Context, userID uuid.UUID, mcpServerID uuid.UUID) error {
	row, ok := r.rows[mcpServerID]
	if !ok || row.UserID != userID {
		return xerr.NotFound("MCP服务不存在")
	}
	r.deletedID = mcpServerID
	delete(r.rows, mcpServerID)
	return nil
}

func (r *fakeMCPServerRepository) FindByName(ctx context.Context, userID uuid.UUID, name string) (*models.MCPServer, error) {
	for _, row := range r.rows {
		if row.UserID == userID && row.Name == name {
			return row, nil
		}
	}
	return nil, xerr.NotFound("MCP服务不存在")
}

func TestCreateMCPServerEncryptsBearerToken(t *testing.T) {
	ctx := context.Background()
	cipher := newTestCipher(t)
	userID := uuid.New()
	repo := newFakeMCPServerRepository()
	logic := NewCreateMCPServerLogic(ctx, &svc.ServiceContext{
		MCPServerRepo: repo,
		SecretCipher:  cipher,
	})

	out, err := logic.CreateMCPServer(userID, &request.CreateMCPServerRequest{
		Name:      "bearer",
		Transport: request.SSETransport,
		Url:       "https://example.com/sse",
		AuthType:  request.Bearer,
		AuthConfig: &request.MCPAuthConfig{
			Token: "plain-token",
		},
	})
	if err != nil {
		t.Fatalf("CreateMCPServer error = %v", err)
	}
	if repo.created == nil {
		t.Fatal("repository Create was not called")
	}
	if got := repo.created.AuthConfig["token"]; got == "" || got == "plain-token" {
		t.Fatalf("stored token = %q, want encrypted value", got)
	}
	plain, err := cipher.Decrypt(repo.created.AuthConfig["token"])
	if err != nil {
		t.Fatalf("Decrypt stored token error = %v", err)
	}
	if plain != "plain-token" {
		t.Fatalf("decrypted token = %q, want plain-token", plain)
	}
	if out.AuthMasked != "*******oken" {
		t.Fatalf("AuthMasked = %q, want masked plain token", out.AuthMasked)
	}
}

func TestCreateMCPServerEncryptsAPIKeyAndKeepsHeaderPlain(t *testing.T) {
	ctx := context.Background()
	cipher := newTestCipher(t)
	userID := uuid.New()
	repo := newFakeMCPServerRepository()
	logic := NewCreateMCPServerLogic(ctx, &svc.ServiceContext{
		MCPServerRepo: repo,
		SecretCipher:  cipher,
	})

	out, err := logic.CreateMCPServer(userID, &request.CreateMCPServerRequest{
		Name:      "api-key",
		Transport: request.StreamableHTTP,
		Url:       "https://example.com/mcp",
		AuthType:  request.ApiKey,
		AuthConfig: &request.MCPAuthConfig{
			Header: "X-Api-Key",
			Key:    "plain-key",
		},
	})
	if err != nil {
		t.Fatalf("CreateMCPServer error = %v", err)
	}
	if got := repo.created.AuthConfig["header"]; got != "X-Api-Key" {
		t.Fatalf("stored header = %q, want X-Api-Key", got)
	}
	if got := repo.created.AuthConfig["key"]; got == "" || got == "plain-key" {
		t.Fatalf("stored key = %q, want encrypted value", got)
	}
	plain, err := cipher.Decrypt(repo.created.AuthConfig["key"])
	if err != nil {
		t.Fatalf("Decrypt stored key error = %v", err)
	}
	if plain != "plain-key" {
		t.Fatalf("decrypted key = %q, want plain-key", plain)
	}
	if out.AuthMasked != "*****-key" {
		t.Fatalf("AuthMasked = %q, want masked plain key", out.AuthMasked)
	}
}

func TestCreateMCPServerStoresAPIKeyQueryPlacement(t *testing.T) {
	// 验证 URL query API key 配置中只有 key 加密，placement/query_param 会明文存储用于同步时拼接 URL。
	ctx := context.Background()
	cipher := newTestCipher(t)
	userID := uuid.New()
	repo := newFakeMCPServerRepository()
	logic := NewCreateMCPServerLogic(ctx, &svc.ServiceContext{
		MCPServerRepo: repo,
		SecretCipher:  cipher,
	})

	out, err := logic.CreateMCPServer(userID, &request.CreateMCPServerRequest{
		Name:      "amap",
		Transport: request.StreamableHTTP,
		Url:       "https://mcp.amap.com/mcp",
		AuthType:  request.ApiKey,
		AuthConfig: &request.MCPAuthConfig{
			Placement:  "query",
			QueryParam: "key",
			Key:        "plain-key",
		},
	})
	if err != nil {
		t.Fatalf("CreateMCPServer error = %v", err)
	}
	if repo.created.AuthConfig["placement"] != "query" || repo.created.AuthConfig["query_param"] != "key" {
		t.Fatalf("stored auth config = %#v, want query placement fields", repo.created.AuthConfig)
	}
	if repo.created.AuthConfig["key"] == "" || repo.created.AuthConfig["key"] == "plain-key" {
		t.Fatalf("stored key = %q, want encrypted value", repo.created.AuthConfig["key"])
	}
	plain, err := cipher.Decrypt(repo.created.AuthConfig["key"])
	if err != nil {
		t.Fatalf("Decrypt stored key error = %v", err)
	}
	if plain != "plain-key" {
		t.Fatalf("decrypted key = %q, want plain-key", plain)
	}
	if out.AuthMasked != "*****-key" {
		t.Fatalf("AuthMasked = %q, want masked plain key", out.AuthMasked)
	}
}

func TestGetMCPServerListUsesAuthenticatedUserAndDecryptsAuthMask(t *testing.T) {
	userID := uuid.New()
	otherUserID := uuid.New()
	cipher := newTestCipher(t)
	encryptedToken, err := cipher.Encrypt("secret-token")
	if err != nil {
		t.Fatalf("Encrypt token error = %v", err)
	}
	repo := newFakeMCPServerRepository(
		&models.MCPServer{
			ID:         uuid.New(),
			UserID:     userID,
			Name:       "mine",
			Transport:  string(request.SSETransport),
			Url:        "https://example.com/sse",
			AuthType:   string(request.Bearer),
			AuthConfig: models.MCPAuthConfig{"token": encryptedToken},
			Enabled:    true,
			Status:     "ready",
			ToolsCache: models.MCPMetas{{Name: "tool", Description: "desc"}},
		},
		&models.MCPServer{
			ID:     uuid.New(),
			UserID: otherUserID,
			Name:   "other",
		},
	)
	logic := NewGetMCPServerListLogic(context.Background(), &svc.ServiceContext{
		MCPServerRepo: repo,
		SecretCipher:  cipher,
	})

	out, err := logic.GetMCPServerList(userID)
	if err != nil {
		t.Fatalf("GetMCPServerList error = %v", err)
	}
	if repo.listUserID != userID {
		t.Fatalf("list userID = %s, want %s", repo.listUserID, userID)
	}
	if len(out.List) != 1 {
		t.Fatalf("list len = %d, want 1", len(out.List))
	}
	if out.List[0].Name != "mine" || len(out.List[0].ToolsCache) != 1 || out.List[0].ToolsCache[0].Name != "tool" {
		t.Fatalf("response list = %+v, want mapped MCP row", out.List)
	}
	if out.List[0].AuthMasked != "********oken" {
		t.Fatalf("auth_masked = %q, want masked decrypted token", out.List[0].AuthMasked)
	}
}

func TestGetMCPServerListSkipsDecryptFailures(t *testing.T) {
	userID := uuid.New()
	cipher := newTestCipher(t)
	encryptedToken, err := cipher.Encrypt("good-token")
	if err != nil {
		t.Fatalf("Encrypt token error = %v", err)
	}
	repo := newFakeMCPServerRepository(
		&models.MCPServer{
			ID:         uuid.New(),
			UserID:     userID,
			Name:       "good",
			AuthType:   string(request.Bearer),
			AuthConfig: models.MCPAuthConfig{"token": encryptedToken},
		},
		&models.MCPServer{
			ID:         uuid.New(),
			UserID:     userID,
			Name:       "bad",
			AuthType:   string(request.Bearer),
			AuthConfig: models.MCPAuthConfig{"token": "not-encrypted"},
		},
	)
	logic := NewGetMCPServerListLogic(context.Background(), &svc.ServiceContext{
		MCPServerRepo: repo,
		SecretCipher:  cipher,
	})

	out, err := logic.GetMCPServerList(userID)
	if err != nil {
		t.Fatalf("GetMCPServerList error = %v", err)
	}
	if len(out.List) != 1 || out.List[0].Name != "good" {
		t.Fatalf("list = %+v, want only decryptable MCP", out.List)
	}
}

func TestDeleteMCPServerParsesIDAndDeletesOwnedRow(t *testing.T) {
	userID := uuid.New()
	mcpID := uuid.New()
	repo := newFakeMCPServerRepository(&models.MCPServer{ID: mcpID, UserID: userID})
	logic := NewDeleteMCPServerLogic(context.Background(), &svc.ServiceContext{MCPServerRepo: repo})

	err := logic.DeleteMCPServer(userID, &request.UriMCPServerIDRequest{ID: mcpID.String()})
	if err != nil {
		t.Fatalf("DeleteMCPServer error = %v", err)
	}
	if repo.deletedID != mcpID {
		t.Fatalf("deleted ID = %s, want %s", repo.deletedID, mcpID)
	}
}

func TestDeleteMCPServerRejectsInvalidID(t *testing.T) {
	repo := newFakeMCPServerRepository()
	logic := NewDeleteMCPServerLogic(context.Background(), &svc.ServiceContext{MCPServerRepo: repo})

	err := logic.DeleteMCPServer(uuid.New(), &request.UriMCPServerIDRequest{ID: "not-a-uuid"})
	if xerr.From(err).Kind != xerr.KindBadRequest {
		t.Fatalf("DeleteMCPServer error = %v, want bad request", err)
	}
	if repo.deletedID != uuid.Nil {
		t.Fatalf("deleted ID = %s, want nil", repo.deletedID)
	}
}

func TestUpdateMCPServerPreservesFalseAndConvertsAuthConfig(t *testing.T) {
	userID := uuid.New()
	mcpID := uuid.New()
	cipher := newTestCipher(t)
	repo := newFakeMCPServerRepository(&models.MCPServer{
		ID:      mcpID,
		UserID:  userID,
		Name:    "old",
		Enabled: true,
	})
	logic := NewUpdateMCPServerLogic(context.Background(), &svc.ServiceContext{
		MCPServerRepo: repo,
		SecretCipher:  cipher,
	})
	enabled := false

	out, err := logic.UpdateMCPServer(userID, &request.UpdateMCPServerRequest{
		UriMCPServerIDRequest: request.UriMCPServerIDRequest{ID: mcpID.String()},
		AuthConfig:            &request.MCPAuthConfig{Token: "new-token"},
		Enabled:               &enabled,
	})
	if err != nil {
		t.Fatalf("UpdateMCPServer error = %v", err)
	}
	if repo.updated != nil {
		t.Fatalf("UpdateMCPServer used full update path: %+v", repo.updated)
	}
	if repo.updateID != mcpID {
		t.Fatalf("update ID = %s, want %s", repo.updateID, mcpID)
	}
	wantFields := []string{"auth_config", "enabled"}
	if !reflect.DeepEqual(repo.fields, wantFields) {
		t.Fatalf("fields = %v, want %v", repo.fields, wantFields)
	}
	if repo.partial.Enabled {
		t.Fatal("patch Enabled = true, want false")
	}
	if got := repo.partial.AuthConfig["token"]; got == "" || got == "new-token" {
		t.Fatalf("patch auth token = %q, want encrypted value", got)
	}
	plain, err := cipher.Decrypt(repo.partial.AuthConfig["token"])
	if err != nil {
		t.Fatalf("Decrypt patch token error = %v", err)
	}
	if plain != "new-token" {
		t.Fatalf("decrypted patch token = %q, want new-token", plain)
	}
	if out.Enabled {
		t.Fatal("response Enabled = true, want false")
	}
}

func TestUpdateMCPServerStoresAPIKeyQueryPlacement(t *testing.T) {
	// 验证更新 MCP 认证配置时，query placement 字段明文保存，key 字段加密保存。
	userID := uuid.New()
	mcpID := uuid.New()
	cipher := newTestCipher(t)
	repo := newFakeMCPServerRepository(&models.MCPServer{
		ID:       mcpID,
		UserID:   userID,
		Name:     "amap",
		AuthType: string(request.ApiKey),
	})
	logic := NewUpdateMCPServerLogic(context.Background(), &svc.ServiceContext{
		MCPServerRepo: repo,
		SecretCipher:  cipher,
	})

	_, err := logic.UpdateMCPServer(userID, &request.UpdateMCPServerRequest{
		UriMCPServerIDRequest: request.UriMCPServerIDRequest{ID: mcpID.String()},
		AuthConfig: &request.MCPAuthConfig{
			Placement:  "query",
			QueryParam: "key",
			Key:        "plain-key",
		},
	})
	if err != nil {
		t.Fatalf("UpdateMCPServer error = %v", err)
	}
	if !reflect.DeepEqual(repo.fields, []string{"auth_config"}) {
		t.Fatalf("fields = %v, want auth_config", repo.fields)
	}
	if repo.partial.AuthConfig["placement"] != "query" || repo.partial.AuthConfig["query_param"] != "key" {
		t.Fatalf("patch auth config = %#v, want query placement fields", repo.partial.AuthConfig)
	}
	if got := repo.partial.AuthConfig["key"]; got == "" || got == "plain-key" {
		t.Fatalf("patch auth key = %q, want encrypted value", got)
	}
	plain, err := cipher.Decrypt(repo.partial.AuthConfig["key"])
	if err != nil {
		t.Fatalf("Decrypt patch key error = %v", err)
	}
	if plain != "plain-key" {
		t.Fatalf("decrypted patch key = %q, want plain-key", plain)
	}
}

func TestUpdateMCPServerSkipsNilFields(t *testing.T) {
	userID := uuid.New()
	mcpID := uuid.New()
	repo := newFakeMCPServerRepository(&models.MCPServer{
		ID:      mcpID,
		UserID:  userID,
		Name:    "old",
		Enabled: true,
	})
	logic := NewUpdateMCPServerLogic(context.Background(), &svc.ServiceContext{MCPServerRepo: repo})
	name := ""

	_, err := logic.UpdateMCPServer(userID, &request.UpdateMCPServerRequest{
		UriMCPServerIDRequest: request.UriMCPServerIDRequest{ID: mcpID.String()},
		Name:                  &name,
	})
	if err != nil {
		t.Fatalf("UpdateMCPServer error = %v", err)
	}
	if !reflect.DeepEqual(repo.fields, []string{"name"}) {
		t.Fatalf("fields = %v, want only name", repo.fields)
	}
	if repo.partial.Name != "" {
		t.Fatalf("patch name = %q, want empty string", repo.partial.Name)
	}
}

func TestUpdateMCPServerRejectsEmptyUpdate(t *testing.T) {
	userID := uuid.New()
	mcpID := uuid.New()
	repo := newFakeMCPServerRepository(&models.MCPServer{ID: mcpID, UserID: userID})
	logic := NewUpdateMCPServerLogic(context.Background(), &svc.ServiceContext{MCPServerRepo: repo})

	_, err := logic.UpdateMCPServer(userID, &request.UpdateMCPServerRequest{
		UriMCPServerIDRequest: request.UriMCPServerIDRequest{ID: mcpID.String()},
	})
	if xerr.From(err).Kind != xerr.KindBadRequest {
		t.Fatalf("UpdateMCPServer error = %v, want bad request", err)
	}
	if repo.updateFieldsN != 1 {
		t.Fatalf("UpdateFields calls = %d, want 1", repo.updateFieldsN)
	}
}

func TestUpdateMCPServerSkipsNilAuthConfig(t *testing.T) {
	userID := uuid.New()
	mcpID := uuid.New()
	cipher := newTestCipher(t)
	encryptedToken, err := cipher.Encrypt("existing")
	if err != nil {
		t.Fatalf("Encrypt token error = %v", err)
	}
	repo := newFakeMCPServerRepository(&models.MCPServer{
		ID:         mcpID,
		UserID:     userID,
		Name:       "old",
		AuthType:   string(request.Bearer),
		AuthConfig: models.MCPAuthConfig{"token": encryptedToken},
	})
	logic := NewUpdateMCPServerLogic(context.Background(), &svc.ServiceContext{
		MCPServerRepo: repo,
		SecretCipher:  cipher,
	})
	name := "new"

	_, err = logic.UpdateMCPServer(userID, &request.UpdateMCPServerRequest{
		UriMCPServerIDRequest: request.UriMCPServerIDRequest{ID: mcpID.String()},
		Name:                  &name,
	})
	if err != nil {
		t.Fatalf("UpdateMCPServer error = %v", err)
	}
	if !reflect.DeepEqual(repo.fields, []string{"name"}) {
		t.Fatalf("fields = %v, want only name", repo.fields)
	}
}

func TestToggleMCPServerPreservesFalseAndDecryptsAuthMask(t *testing.T) {
	userID := uuid.New()
	mcpID := uuid.New()
	cipher := newTestCipher(t)
	encryptedToken, err := cipher.Encrypt("toggle-token")
	if err != nil {
		t.Fatalf("Encrypt token error = %v", err)
	}
	repo := newFakeMCPServerRepository(&models.MCPServer{
		ID:         mcpID,
		UserID:     userID,
		Name:       "toggle",
		AuthType:   string(request.Bearer),
		AuthConfig: models.MCPAuthConfig{"token": encryptedToken},
		Enabled:    true,
	})
	logic := NewTroggleMCPServerLogic(context.Background(), &svc.ServiceContext{
		MCPServerRepo: repo,
		SecretCipher:  cipher,
	})
	enabled := false

	out, err := logic.ToggleMCPServer(userID, &request.ToggleMCPServerRequest{
		UriMCPServerIDRequest: request.UriMCPServerIDRequest{ID: mcpID.String()},
		Enabled:               &enabled,
	})
	if err != nil {
		t.Fatalf("ToggleMCPServer error = %v", err)
	}
	if repo.updateID != mcpID {
		t.Fatalf("update ID = %s, want %s", repo.updateID, mcpID)
	}
	if !reflect.DeepEqual(repo.fields, []string{"enabled"}) {
		t.Fatalf("fields = %v, want only enabled", repo.fields)
	}
	if repo.partial.Enabled {
		t.Fatal("patch Enabled = true, want false")
	}
	if out.Enabled {
		t.Fatal("response Enabled = true, want false")
	}
	if out.AuthMasked != "********oken" {
		t.Fatalf("auth_masked = %q, want masked decrypted token", out.AuthMasked)
	}
}

func TestToggleMCPServerRejectsInvalidID(t *testing.T) {
	repo := newFakeMCPServerRepository()
	logic := NewTroggleMCPServerLogic(context.Background(), &svc.ServiceContext{MCPServerRepo: repo})
	enabled := true

	_, err := logic.ToggleMCPServer(uuid.New(), &request.ToggleMCPServerRequest{
		UriMCPServerIDRequest: request.UriMCPServerIDRequest{ID: "not-a-uuid"},
		Enabled:               &enabled,
	})
	if xerr.From(err).Kind != xerr.KindBadRequest {
		t.Fatalf("ToggleMCPServer error = %v, want bad request", err)
	}
	if repo.updateFieldsN != 0 {
		t.Fatalf("UpdateFields calls = %d, want 0", repo.updateFieldsN)
	}
}

func TestToggleMCPServerPropagatesNotFound(t *testing.T) {
	repo := newFakeMCPServerRepository()
	logic := NewTroggleMCPServerLogic(context.Background(), &svc.ServiceContext{MCPServerRepo: repo})
	enabled := true

	_, err := logic.ToggleMCPServer(uuid.New(), &request.ToggleMCPServerRequest{
		UriMCPServerIDRequest: request.UriMCPServerIDRequest{ID: uuid.New().String()},
		Enabled:               &enabled,
	})
	if xerr.From(err).Kind != xerr.KindNotFound {
		t.Fatalf("ToggleMCPServer error = %v, want not found", err)
	}
}

func TestSyncMCPServerUpdatesToolsAndClearsLastError(t *testing.T) {
	// 验证同步成功会写入工具元信息、ready 状态，并清空上一次错误。
	userID := uuid.New()
	mcpID := uuid.New()
	oldError := "previous error"
	cipher := newTestCipher(t)
	encryptedToken, err := cipher.Encrypt("sync-token")
	if err != nil {
		t.Fatalf("Encrypt token error = %v", err)
	}
	repo := newFakeMCPServerRepository(&models.MCPServer{
		ID:         mcpID,
		UserID:     userID,
		Name:       "sync",
		Transport:  string(request.StreamableHTTP),
		Url:        "https://example.com/mcp",
		AuthType:   string(request.Bearer),
		AuthConfig: models.MCPAuthConfig{"token": encryptedToken},
		Status:     "error",
		LastError:  &oldError,
		UpdatedAt:  time.Now(),
	})
	client := &recordingMCPToolClient{
		tools: []coremcp.ToolInfo{{
			Name:         "search",
			Description:  "web search",
			Title:        "Search",
			InputSchema:  map[string]any{"type": "object"},
			OutputSchema: map[string]any{"type": "string"},
		}},
	}
	cache := &recordingMCPToolCache{}
	logic := NewSyncMCPServerLogic(context.Background(), &svc.ServiceContext{
		MCPServerRepo:  repo,
		SecretCipher:   cipher,
		MCPToolService: coremcp.NewService(coremcp.WithClient(client), coremcp.WithCache(cache)),
	})

	out, err := logic.SyncMCPServer(userID, &request.UriMCPServerIDRequest{ID: mcpID.String()})
	if err != nil {
		t.Fatalf("SyncMCPServer error = %v", err)
	}
	if !reflect.DeepEqual(repo.fields, []string{"tools_cache", "synced_at", "status", "last_error"}) {
		t.Fatalf("fields = %v, want sync update fields", repo.fields)
	}
	if repo.partial.Status != "ready" {
		t.Fatalf("status = %q, want ready", repo.partial.Status)
	}
	if repo.partial.LastError == nil || *repo.partial.LastError != "" {
		t.Fatalf("last_error = %#v, want empty string pointer", repo.partial.LastError)
	}
	if len(out.ToolsCache) != 1 || out.ToolsCache[0].Name != "search" {
		t.Fatalf("ToolsCache = %#v, want synced tools", out.ToolsCache)
	}
	if len(repo.partial.ToolsCache) != 1 || repo.partial.ToolsCache[0].Name != "search" || repo.partial.ToolsCache[0].Description != "web search" {
		t.Fatalf("patch ToolsCache = %#v, want name+description only", repo.partial.ToolsCache)
	}
	if cache.set == nil || len(cache.set.Tools) != 1 || cache.set.Tools[0].InputSchema == nil || cache.set.Tools[0].Title != "Search" {
		t.Fatalf("runtime cache = %#v, want full tool info", cache.set)
	}
	if client.lastServer.AuthConfig["token"] != "sync-token" {
		t.Fatalf("client token = %q, want decrypted token", client.lastServer.AuthConfig["token"])
	}
}

// TestSyncMCPServerBypassesRuntimeTTL 验证显式同步即使已有有效运行时缓存也会重新访问远端。
func TestSyncMCPServerBypassesRuntimeTTL(t *testing.T) {
	userID := uuid.New()
	server := &models.MCPServer{
		ID: uuid.New(), UserID: userID, Name: "sync", Transport: string(request.StreamableHTTP),
		Url: "https://example.com/mcp", AuthType: string(request.None), AuthConfig: models.MCPAuthConfig{}, UpdatedAt: time.Now(),
	}
	repo := newFakeMCPServerRepository(server)
	client := &countingSyncMCPToolClient{tools: []coremcp.ToolInfo{{Name: "cached"}}}
	service := coremcp.NewService(coremcp.WithClient(client))
	coreServer := coremcp.ServerConfig{ID: server.ID, Transport: server.Transport, URL: server.Url, AuthType: server.AuthType, UpdatedAt: server.UpdatedAt}
	if _, err := service.BuildToolList(context.Background(), coreServer); err != nil {
		t.Fatalf("BuildToolList error = %v, want nil", err)
	}
	client.tools = []coremcp.ToolInfo{{Name: "fresh"}}

	logic := NewSyncMCPServerLogic(context.Background(), &svc.ServiceContext{
		MCPServerRepo: repo, SecretCipher: newTestCipher(t), MCPToolService: service,
	})
	out, err := logic.SyncMCPServer(userID, &request.UriMCPServerIDRequest{ID: server.ID.String()})
	if err != nil {
		t.Fatalf("SyncMCPServer error = %v, want nil", err)
	}
	if client.calls != 2 || len(out.ToolsCache) != 1 || out.ToolsCache[0].Name != "fresh" {
		t.Fatalf("remote calls/tools = %d/%#v, want 2/fresh", client.calls, out.ToolsCache)
	}
}

func TestSyncMCPServerPassesDecryptedAPIKeyQueryConfig(t *testing.T) {
	// 验证同步前会解密 API key，并把 query placement 配置传给 core service。
	userID := uuid.New()
	mcpID := uuid.New()
	cipher := newTestCipher(t)
	encryptedKey, err := cipher.Encrypt("sync-key")
	if err != nil {
		t.Fatalf("Encrypt key error = %v", err)
	}
	repo := newFakeMCPServerRepository(&models.MCPServer{
		ID:        mcpID,
		UserID:    userID,
		Name:      "amap",
		Transport: string(request.StreamableHTTP),
		Url:       "https://mcp.amap.com/mcp",
		AuthType:  string(request.ApiKey),
		AuthConfig: models.MCPAuthConfig{
			"placement":   "query",
			"query_param": "key",
			"key":         encryptedKey,
		},
		UpdatedAt: time.Now(),
	})
	client := &recordingMCPToolClient{tools: []coremcp.ToolInfo{{Name: "maps", Description: "maps"}}}
	logic := NewSyncMCPServerLogic(context.Background(), &svc.ServiceContext{
		MCPServerRepo:  repo,
		SecretCipher:   cipher,
		MCPToolService: coremcp.NewService(coremcp.WithClient(client)),
	})

	_, err = logic.SyncMCPServer(userID, &request.UriMCPServerIDRequest{ID: mcpID.String()})
	if err != nil {
		t.Fatalf("SyncMCPServer error = %v", err)
	}
	if client.lastServer.AuthConfig["placement"] != "query" || client.lastServer.AuthConfig["query_param"] != "key" {
		t.Fatalf("core auth config = %#v, want query placement fields", client.lastServer.AuthConfig)
	}
	if client.lastServer.AuthConfig["key"] != "sync-key" {
		t.Fatalf("core auth key = %q, want decrypted sync-key", client.lastServer.AuthConfig["key"])
	}
}

func TestSyncMCPServerRecordsLastErrorAndKeepsToolsCache(t *testing.T) {
	// 验证同步失败会记录 last_error，但不会覆盖数据库中已有的工具元信息。
	userID := uuid.New()
	mcpID := uuid.New()
	oldTools := models.MCPMetas{{Name: "old", Description: "old desc"}}
	repo := newFakeMCPServerRepository(&models.MCPServer{
		ID:         mcpID,
		UserID:     userID,
		Name:       "sync",
		Transport:  string(request.SSETransport),
		Url:        "https://example.com/sse",
		AuthType:   string(request.None),
		AuthConfig: models.MCPAuthConfig{},
		Status:     "unknown",
		ToolsCache: oldTools,
		UpdatedAt:  time.Now(),
	})
	client := &recordingMCPToolClient{err: errors.New("remote unavailable")}
	logic := NewSyncMCPServerLogic(context.Background(), &svc.ServiceContext{
		MCPServerRepo:  repo,
		SecretCipher:   newTestCipher(t),
		MCPToolService: coremcp.NewService(coremcp.WithClient(client)),
	})

	_, err := logic.SyncMCPServer(userID, &request.UriMCPServerIDRequest{ID: mcpID.String()})
	if err == nil {
		t.Fatal("SyncMCPServer returned nil error, want remote error")
	}
	if !reflect.DeepEqual(repo.fields, []string{"status", "last_error"}) {
		t.Fatalf("fields = %v, want only status and last_error", repo.fields)
	}
	if repo.partial.Status != "error" {
		t.Fatalf("status = %q, want error", repo.partial.Status)
	}
	if repo.partial.LastError == nil || *repo.partial.LastError == "" {
		t.Fatalf("last_error = %#v, want remote error message", repo.partial.LastError)
	}
	if !reflect.DeepEqual(repo.rows[mcpID].ToolsCache, oldTools) {
		t.Fatalf("tools_cache = %#v, want old tools %#v", repo.rows[mcpID].ToolsCache, oldTools)
	}
}

type recordingMCPToolClient struct {
	tools      []coremcp.ToolInfo
	err        error
	lastServer coremcp.ServerConfig
}

type countingSyncMCPToolClient struct {
	tools []coremcp.ToolInfo
	calls int
}

func (c *countingSyncMCPToolClient) ListTools(context.Context, coremcp.ServerConfig) ([]coremcp.ToolInfo, error) {
	c.calls++
	return append([]coremcp.ToolInfo(nil), c.tools...), nil
}

func (c *recordingMCPToolClient) ListTools(ctx context.Context, server coremcp.ServerConfig) ([]coremcp.ToolInfo, error) {
	c.lastServer = server
	if c.err != nil {
		return nil, c.err
	}
	return c.tools, nil
}

type recordingMCPToolCache struct {
	set *coremcp.CacheEntry
}

func (c *recordingMCPToolCache) Get(ctx context.Context, key string) (coremcp.CacheEntry, bool, error) {
	return coremcp.CacheEntry{}, false, nil
}

func (c *recordingMCPToolCache) Set(ctx context.Context, key string, entry coremcp.CacheEntry) error {
	c.set = &entry
	return nil
}

func (c *recordingMCPToolCache) Valid(server coremcp.ServerConfig, entry coremcp.CacheEntry) bool {
	return false
}

func newTestCipher(t *testing.T) *security.SecretCipher {
	t.Helper()
	cipher, err := security.NewSecretCipher("0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatalf("NewSecretCipher error = %v", err)
	}
	return cipher
}
