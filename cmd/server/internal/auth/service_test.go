package auth

import (
	"io"
	"log/slog"
	"testing"
	"time"

	dbstore "github.com/sleklere/chattui/cmd/server/internal/store"
)

var testCfg = &Config{
	JWTSecret: []byte("test-secret"),
	Issuer:    "test-issuer",
	AccessTTL: 15 * time.Minute,
}

func newTestService() *Service {
	return &Service{
		auth:   testCfg,
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
}

// ---------------------------------------------------------------------------
// generateAccessToken
// ---------------------------------------------------------------------------

func TestGenerateAccessToken_ClaimsAreCorrect(t *testing.T) {
	svc := newTestService()
	user := dbstore.User{ID: 42, Username: "bob"}

	tokenStr, exp, err := svc.generateAccessToken(user)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	claims, err := ParseToken(tokenStr, testCfg)
	if err != nil {
		t.Fatalf("token did not parse: %v", err)
	}

	if claims.UserID != user.ID {
		t.Errorf("expected UserID %d, got %d", user.ID, claims.UserID)
	}
	if claims.Username != user.Username {
		t.Errorf("expected Username %q, got %q", user.Username, claims.Username)
	}
	if !exp.After(time.Now()) {
		t.Errorf("expected expiration in the future, got %v", exp)
	}
	if time.Until(exp) > testCfg.AccessTTL {
		t.Errorf("expiration exceeds AccessTTL")
	}
}

func TestGenerateAccessToken_InvalidSecretRejectsToken(t *testing.T) {
	svc := newTestService()
	user := dbstore.User{ID: 1, Username: "bob"}

	tokenStr, _, err := svc.generateAccessToken(user)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	wrongCfg := &Config{JWTSecret: []byte("wrong-secret"), Issuer: testCfg.Issuer}
	_, err = ParseToken(tokenStr, wrongCfg)
	if err == nil {
		t.Fatal("expected error with wrong secret, got nil")
	}
}
