package security_test

import (
	"testing"
	"time"

	"github.com/boxify/api-go/internal/infrastructure/security"
	"github.com/google/uuid"
)

func TestSecretCipherEncryptsDecryptsAndMasks(t *testing.T) {
	cipher, err := security.NewSecretCipher("0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatalf("NewSecretCipher error = %v", err)
	}

	encrypted, err := cipher.Encrypt("sk-test-secret")
	if err != nil {
		t.Fatalf("Encrypt error = %v", err)
	}
	if encrypted == "sk-test-secret" {
		t.Fatalf("encrypted secret must not equal plaintext")
	}

	plain, err := cipher.Decrypt(encrypted)
	if err != nil {
		t.Fatalf("Decrypt error = %v", err)
	}
	if plain != "sk-test-secret" {
		t.Fatalf("plain = %q, want original", plain)
	}
	if got := security.MaskSecret("sk-test-secret"); got != "**********cret" {
		t.Fatalf("mask = %q", got)
	}
}

func TestTokenIssuerVerifiesAccessToken(t *testing.T) {
	issuer := security.NewTokenIssuer("test-secret", time.Hour)
	userID := uuid.New()

	token, err := issuer.IssueAccessToken(userID)
	if err != nil {
		t.Fatalf("IssueAccessToken error = %v", err)
	}
	verifiedID, err := issuer.VerifyAccessToken(t.Context(), token)
	if err != nil {
		t.Fatalf("VerifyAccessToken error = %v", err)
	}
	if verifiedID != userID {
		t.Fatalf("user id = %s, want %s", verifiedID, userID)
	}
}
