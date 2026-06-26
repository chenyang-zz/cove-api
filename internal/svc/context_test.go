package svc_test

import (
	"context"
	"strings"
	"testing"

	"github.com/boxify/api-go/internal/config"
	"github.com/boxify/api-go/internal/svc"
)

func TestNewReturnsErrorForInvalidPostgresURL(t *testing.T) {
	cfg := config.Config{}
	cfg.Database.URL = "   "
	cfg.JWT.Secret = "test-secret"
	cfg.JWT.AccessTokenTTL = "168h"
	cfg.SecretKey = "0123456789abcdef0123456789abcdef"

	if _, err := svc.New(context.Background(), cfg); err == nil {
		t.Fatal("svc.New error = nil, want error")
	}
}

func TestNewReturnsErrorForInvalidAccessTokenTTL(t *testing.T) {
	cfg := config.Config{}
	cfg.Database.URL = "   "
	cfg.JWT.Secret = "test-secret"
	cfg.JWT.AccessTokenTTL = "not-a-duration"
	cfg.SecretKey = "0123456789abcdef0123456789abcdef"

	_, err := svc.New(context.Background(), cfg)
	if err == nil || !strings.Contains(err.Error(), "JWT access token TTL 配置无效") {
		t.Fatalf("svc.New error = %v, want invalid ttl error", err)
	}
}

func TestCloseCanBeCalledRepeatedly(t *testing.T) {
	svcCtx := &svc.ServiceContext{}
	ctx := context.Background()

	if err := svcCtx.Close(ctx); err != nil {
		t.Fatalf("first Close error = %v", err)
	}
	if err := svcCtx.Close(ctx); err != nil {
		t.Fatalf("second Close error = %v", err)
	}
}
