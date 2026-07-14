package chat

import (
	"context"
	"errors"
	"strings"

	corecontext "github.com/boxify/api-go/internal/core/context"
	"github.com/boxify/api-go/internal/models"
	"github.com/boxify/api-go/internal/repository"
	"github.com/google/uuid"
)

type conversationContextStore struct {
	repository     repository.ConversationContextStateRepository
	userID         uuid.UUID
	conversationID uuid.UUID
}

func newConversationContextStore(repository repository.ConversationContextStateRepository, userID uuid.UUID, conversationID uuid.UUID) *conversationContextStore {
	return &conversationContextStore{repository: repository, userID: userID, conversationID: conversationID}
}

func (s *conversationContextStore) Load(ctx context.Context, key string) (*corecontext.State, error) {
	if err := s.validateKey(key); err != nil {
		return nil, err
	}
	row, err := s.repository.LoadContextState(ctx, s.userID, s.conversationID)
	if err != nil || row == nil {
		return nil, err
	}
	throughID := ""
	if row.ThroughMessageID != nil {
		throughID = row.ThroughMessageID.String()
	}
	return &corecontext.State{
		Summary:           row.Summary,
		ThroughID:         throughID,
		Version:           row.Version,
		FormatVersion:     row.FormatVersion,
		PolicyFingerprint: row.PolicyFingerprint,
	}, nil
}

func (s *conversationContextStore) CompareAndSwap(ctx context.Context, key string, expectedVersion int64, next *corecontext.State) (bool, error) {
	if err := s.validateKey(key); err != nil {
		return false, err
	}
	if next == nil {
		return false, errors.New("conversation context state is nil")
	}
	var throughMessageID *uuid.UUID
	if strings.TrimSpace(next.ThroughID) != "" {
		parsed, err := uuid.Parse(next.ThroughID)
		if err != nil {
			return false, errors.New("conversation context through message id is invalid")
		}
		throughMessageID = &parsed
	}
	return s.repository.CompareAndSwapContextState(ctx, s.userID, expectedVersion, &models.ConversationContextState{
		ConversationID:    s.conversationID,
		Summary:           next.Summary,
		ThroughMessageID:  throughMessageID,
		Version:           next.Version,
		FormatVersion:     next.FormatVersion,
		PolicyFingerprint: next.PolicyFingerprint,
	})
}

func (s *conversationContextStore) validateKey(key string) error {
	if s == nil || s.repository == nil {
		return errors.New("conversation context repository is nil")
	}
	if key != s.conversationID.String() {
		return errors.New("conversation context key does not match conversation")
	}
	return nil
}

var _ corecontext.Store = (*conversationContextStore)(nil)
