package auth

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/Home/galaxy-mmo/internal/domain"
)

type AuthService struct {
	db           *sql.DB
	sessionCache domain.SessionCache
}

func NewAuthService(db *sql.DB, cache domain.SessionCache) *AuthService {
	return &AuthService{
		db:           db,
		sessionCache: cache,
	}
}

// Register registers a new account by hashing their password and storing it.
func (s *AuthService) Register(ctx context.Context, login, password string) (uint64, error) {
	if login == "" || password == "" {
		return 0, domain.ErrInvalidCredentials
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return 0, fmt.Errorf("failed to hash password: %w", err)
	}

	query := "INSERT INTO accounts (login, password_hash) VALUES ($1, $2) RETURNING id"
	
	var accountID uint64
	err = s.db.QueryRowContext(ctx, query, login, string(hash)).Scan(&accountID)
	if err != nil {
		// Postgres duplicate key error could be handled, but for MVP we return a generic error
		return 0, fmt.Errorf("failed to insert account: %w", err)
	}

	return accountID, nil
}

// Login verifies credentials and returns a session token.
func (s *AuthService) Login(ctx context.Context, login, password string) (string, uint64, error) {
	if login == "" || password == "" {
		return "", 0, domain.ErrInvalidCredentials
	}

	query := "SELECT id, password_hash FROM accounts WHERE login = $1"
	
	var accountID uint64
	var hash string
	err := s.db.QueryRowContext(ctx, query, login).Scan(&accountID, &hash)
	if err == sql.ErrNoRows {
		return "", 0, domain.ErrInvalidCredentials
	} else if err != nil {
		return "", 0, fmt.Errorf("database query failed: %w", err)
	}

	err = bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	if err != nil {
		return "", 0, domain.ErrInvalidCredentials
	}

	// Generate session token
	token, err := GenerateSessionToken()
	if err != nil {
		return "", 0, fmt.Errorf("failed to generate session token: %w", err)
	}

	// Save to Redis session cache (24 hours TTL)
	err = s.sessionCache.Set(ctx, token, accountID, 24*time.Hour)
	if err != nil {
		return "", 0, fmt.Errorf("failed to cache session: %w", err)
	}

	return token, accountID, nil
}
