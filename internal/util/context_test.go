package util_test

import (
	"context"
	"testing"

	"github.com/boxify/api-go/internal/util"
	"github.com/boxify/api-go/internal/xerr"
	"github.com/google/uuid"
)

func TestUserIDFromContext(t *testing.T) {
	userID := uuid.New()
	ctx := util.WithUserID(context.Background(), userID)

	got, err := util.UserIDFromContext(ctx)
	if err != nil {
		t.Fatalf("UserIDFromContext error = %v, want nil", err)
	}
	if got != userID {
		t.Fatalf("UserIDFromContext = %s, want %s", got, userID)
	}
}

func TestUserIDFromContextReturnsUnauthorizedWhenMissingOrNil(t *testing.T) {
	if got, err := util.UserIDFromContext(context.Background()); got != uuid.Nil || xerr.From(err).Kind != xerr.KindUnauthorized {
		t.Fatalf("missing UserIDFromContext = %s, %v; want nil,unauthorized", got, err)
	}
	if got, err := util.UserIDFromContext(util.WithUserID(context.Background(), uuid.Nil)); got != uuid.Nil || xerr.From(err).Kind != xerr.KindUnauthorized {
		t.Fatalf("nil UserIDFromContext = %s, %v; want nil,unauthorized", got, err)
	}
}
