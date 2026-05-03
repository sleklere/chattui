// Package auth provides authentication services, including user
// registration, login, and related domain logic.
package auth

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/golang-jwt/jwt/v5"
	dbstore "github.com/sleklere/chattui/cmd/server/internal/store"
	"golang.org/x/crypto/bcrypt"
)

// RegisterInput holds the data needed to register a new user.
type RegisterInput struct {
	Username string
	Password string
}

// LoginInput holds the data needed to authenticate a user.
type LoginInput struct {
	Username string
	Password string
}

// Result is returned on successful authentication.
type Result struct {
	UserID    int64
	Username  string
	CreatedAt time.Time
	Token     string
	ExpiresAt int64
}

// Store defines the persistence methods required for authentication operations.
type Store interface {
	GetUserByUsername(ctx context.Context, username string) (dbstore.User, error)
	CreateUser(ctx context.Context, arg dbstore.CreateUserParams) (dbstore.User, error)
}

// Service provides authentication-related business logic using a Store.
type Service struct {
	store  Store
	logger *slog.Logger
	auth   *Config
}

// NewService creates a new Service with the given Store and logger.
func NewService(s Store, l *slog.Logger, authCfg *Config) *Service {
	return &Service{store: s, logger: l, auth: authCfg}
}

// ErrUsernameTaken indicates that the username is already registered.
// ErrInvalidCreds indicates that the provided credentials are invalid.
// ErrUserNotFound indicates that no user was found in the database.
var (
	ErrUsernameTaken = errors.New("username already in use")
	ErrInvalidCreds  = errors.New("invalid credentials")
	ErrUserNotFound  = errors.New("user not found")
)

// Register creates a new user account, hashing the password and checking for duplicates.
func (s *Service) Register(ctx context.Context, in RegisterInput) (Result, error) {
	if _, err := s.store.GetUserByUsername(ctx, in.Username); err == nil {
		return Result{}, ErrUsernameTaken
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
	if err != nil {
		return Result{}, err
	}

	u, err := s.store.CreateUser(ctx, dbstore.CreateUserParams{
		Username: in.Username,
		Password: string(hash),
	})
	if err != nil {
		return Result{}, err
	}

	token, exp, err := s.generateAccessToken(u)
	if err != nil {
		return Result{}, err
	}

	return Result{
		UserID:    u.ID,
		Username:  u.Username,
		CreatedAt: u.CreatedAt.Time,
		Token:     token,
		ExpiresAt: exp.Unix(),
	}, nil
}

// Login checks the credentials and returns an Result on success.
func (s *Service) Login(ctx context.Context, in LoginInput) (Result, error) {
	u, err := s.store.GetUserByUsername(ctx, in.Username)
	if err != nil {
		return Result{}, ErrUserNotFound
	}

	if err := bcrypt.CompareHashAndPassword([]byte(u.Password), []byte(in.Password)); err != nil {
		return Result{}, ErrInvalidCreds
	}

	token, exp, err := s.generateAccessToken(u)
	if err != nil {
		return Result{}, err
	}

	return Result{
		UserID:    u.ID,
		Username:  u.Username,
		CreatedAt: u.CreatedAt.Time,
		Token:     token,
		ExpiresAt: exp.Unix(),
	}, nil
}

func (s *Service) generateAccessToken(u dbstore.User) (string, time.Time, error) {
	now := time.Now().UTC()
	exp := now.Add(s.auth.AccessTTL)
	claims := Claims{
		UserID:   u.ID,
		Username: u.Username,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    s.auth.Issuer,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(exp),
		},
	}
	t := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	token, err := t.SignedString(s.auth.JWTSecret)
	return token, exp, err
}
