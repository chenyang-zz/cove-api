package chat

import (
	"context"
	"testing"

	corecontext "github.com/boxify/api-go/internal/core/context"
	"github.com/boxify/api-go/internal/models"
	"github.com/google/uuid"
)

// TestConversationContextStoreMapsStateAndUsesConversationKey 验证聊天适配器正确映射 core 状态、消息游标和 CAS 版本。
func TestConversationContextStoreMapsStateAndUsesConversationKey(t *testing.T) {
	conversationID := uuid.New()
	throughID := uuid.New()
	repository := &fakeConversationContextStateRepository{row: &models.ConversationContextState{
		ConversationID: conversationID, Summary: "old", ThroughMessageID: &throughID,
		Version: 2, FormatVersion: 1, PolicyFingerprint: "old-fingerprint",
	}}
	store := newConversationContextStore(repository, uuid.New(), conversationID)

	loaded, err := store.Load(context.Background(), conversationID.String())
	if err != nil || loaded == nil || loaded.ThroughID != throughID.String() || loaded.Version != 2 {
		t.Fatalf("conversationContextStore.Load() = %#v, error=%v, want mapped state", loaded, err)
	}
	newThroughID := uuid.New()
	saved, err := store.CompareAndSwap(context.Background(), conversationID.String(), 2, &corecontext.State{
		Summary: "next", ThroughID: newThroughID.String(), Version: 3, FormatVersion: 1, PolicyFingerprint: "next-fingerprint",
	})
	if err != nil || !saved || repository.expectedVersion != 2 || repository.row.ThroughMessageID == nil || *repository.row.ThroughMessageID != newThroughID {
		t.Fatalf("conversationContextStore.CompareAndSwap() saved=%v error=%v repo=%#v, want mapped CAS", saved, err, repository)
	}
}

// TestConversationContextStoreRejectsMismatchedKey 验证适配器拒绝把一个会话的摘要写入另一个 key。
func TestConversationContextStoreRejectsMismatchedKey(t *testing.T) {
	store := newConversationContextStore(&fakeConversationContextStateRepository{}, uuid.New(), uuid.New())
	if _, err := store.Load(context.Background(), uuid.NewString()); err == nil {
		t.Fatal("conversationContextStore.Load(mismatched key) error = nil, want error")
	}
}

type fakeConversationContextStateRepository struct {
	row             *models.ConversationContextState
	expectedVersion int64
}

func (f *fakeConversationContextStateRepository) LoadContextState(context.Context, uuid.UUID, uuid.UUID) (*models.ConversationContextState, error) {
	return f.row, nil
}

func (f *fakeConversationContextStateRepository) CompareAndSwapContextState(_ context.Context, _ uuid.UUID, expectedVersion int64, state *models.ConversationContextState) (bool, error) {
	f.expectedVersion = expectedVersion
	f.row = state
	return true, nil
}
