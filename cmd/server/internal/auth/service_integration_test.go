package auth

import (
	"context"
	"testing"
	"time"

	"github.com/sleklere/realtime-chat/cmd/server/internal/testhelper"
)

func integrationAuthCfg() *Config {
	return &Config{
		JWTSecret: []byte("test-secret-key"),
		Issuer:    "test",
		AccessTTL: 15 * time.Minute,
	}
}

func TestRegister_HappyPath(t *testing.T) {
	if testPool == nil {
		t.Skip("testcontainers not available")
	}
	q := testhelper.WithTx(t, testPool)
	svc := NewService(q, testhelper.DiscardLogger(), integrationAuthCfg())

	res, err := svc.Register(context.Background(), RegisterInput{Username: "alice", Password: "password123"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Username != "alice" {
		t.Errorf("username: want alice, got %s", res.Username)
	}
	if res.UserID == 0 {
		t.Error("expected non-zero user ID")
	}
	if res.Token == "" {
		t.Error("expected non-empty token")
	}
}

func TestRegister_DuplicateUsername(t *testing.T) {
	if testPool == nil {
		t.Skip("testcontainers not available")
	}
	q := testhelper.WithTx(t, testPool)
	svc := NewService(q, testhelper.DiscardLogger(), integrationAuthCfg())
	ctx := context.Background()

	if _, err := svc.Register(ctx, RegisterInput{Username: "bob", Password: "pass1"}); err != nil {
		t.Fatalf("first register: %v", err)
	}

	_, err := svc.Register(ctx, RegisterInput{Username: "bob", Password: "pass2"})
	if err != ErrUsernameTaken {
		t.Errorf("want ErrUsernameTaken, got %v", err)
	}
}

func TestLogin_HappyPath(t *testing.T) {
	if testPool == nil {
		t.Skip("testcontainers not available")
	}
	q := testhelper.WithTx(t, testPool)
	svc := NewService(q, testhelper.DiscardLogger(), integrationAuthCfg())
	ctx := context.Background()

	if _, err := svc.Register(ctx, RegisterInput{Username: "carol", Password: "secret"}); err != nil {
		t.Fatalf("register: %v", err)
	}

	res, err := svc.Login(ctx, LoginInput{Username: "carol", Password: "secret"})
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if res.Username != "carol" {
		t.Errorf("username: want carol, got %s", res.Username)
	}
}

func TestLogin_WrongPassword(t *testing.T) {
	if testPool == nil {
		t.Skip("testcontainers not available")
	}
	q := testhelper.WithTx(t, testPool)
	svc := NewService(q, testhelper.DiscardLogger(), integrationAuthCfg())
	ctx := context.Background()

	if _, err := svc.Register(ctx, RegisterInput{Username: "dan", Password: "correct"}); err != nil {
		t.Fatalf("register: %v", err)
	}

	_, err := svc.Login(ctx, LoginInput{Username: "dan", Password: "wrong"})
	if err != ErrInvalidCreds {
		t.Errorf("want ErrInvalidCreds, got %v", err)
	}
}

func TestLogin_UnknownUser(t *testing.T) {
	if testPool == nil {
		t.Skip("testcontainers not available")
	}
	q := testhelper.WithTx(t, testPool)
	svc := NewService(q, testhelper.DiscardLogger(), integrationAuthCfg())

	_, err := svc.Login(context.Background(), LoginInput{Username: "nobody", Password: "pass"})
	if err != ErrUserNotFound {
		t.Errorf("want ErrUserNotFound, got %v", err)
	}
}
