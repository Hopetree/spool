package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	"golang.org/x/crypto/bcrypt"

	"github.com/Hopetree/spool/internal/store"
)

// Sentinel errors returned by the auth service.
var (
	ErrInvalidCredentials = errors.New("invalid username or password")
	ErrUserDisabled       = errors.New("user is disabled")
	ErrSessionInvalid     = errors.New("session is invalid or expired")
	ErrWeakPassword       = errors.New("password must be at least 8 characters and contain letters and digits")
)

// User is an authenticated local account.
type User struct {
	ID                 string `db:"id"`
	Username           string `db:"username"`
	DisplayName        string `db:"display_name"`
	PasswordHash       string `db:"password_hash"`
	Role               string `db:"role"`
	Enabled            bool   `db:"enabled"`
	MustChangePassword bool   `db:"must_change_password"`
	LastLoginAt        *int64 `db:"last_login_at"`
	CreatedAt          int64  `db:"created_at"`
	UpdatedAt          int64  `db:"updated_at"`
}

// Service provides password, user and session operations.
type Service struct {
	db  *sqlx.DB
	ttl time.Duration
	now func() time.Time
}

// NewService builds a Service with the given session lifetime.
func NewService(db *sqlx.DB, ttl time.Duration) *Service {
	return &Service{db: db, ttl: ttl, now: time.Now}
}

// SetNowForTest replaces the clock. Test-only.
func (s *Service) SetNowForTest(now func() time.Time) { s.now = now }

func (s *Service) nowMs() int64 { return s.now().UnixMilli() }

// HashPassword hashes a plaintext password with bcrypt cost 12.
func (s *Service) HashPassword(password string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(password), 12)
	if err != nil {
		return "", fmt.Errorf("hash password: %w", err)
	}
	return string(b), nil
}

// VerifyPassword reports whether password matches the bcrypt hash.
func (s *Service) VerifyPassword(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

// ValidatePassword enforces the password policy.
func ValidatePassword(password string) error {
	if len(password) < 8 {
		return ErrWeakPassword
	}
	if !strings.ContainsAny(password, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ") {
		return ErrWeakPassword
	}
	if !strings.ContainsAny(password, "0123456789") {
		return ErrWeakPassword
	}
	return nil
}

// Authenticate verifies credentials and updates last_login_at.
func (s *Service) Authenticate(ctx context.Context, username, password string) (*User, error) {
	var u User
	err := s.db.GetContext(ctx, &u, `SELECT id, username, display_name, password_hash, role, enabled,
		must_change_password, last_login_at, created_at, updated_at
		FROM users WHERE username = ?`, username)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrInvalidCredentials
	}
	if err != nil {
		return nil, fmt.Errorf("query user: %w", err)
	}
	if !s.VerifyPassword(u.PasswordHash, password) {
		return nil, ErrInvalidCredentials
	}
	if !u.Enabled {
		return nil, ErrUserDisabled
	}
	now := s.nowMs()
	if _, err := s.db.ExecContext(ctx, `UPDATE users SET last_login_at = ? WHERE id = ?`, now, u.ID); err != nil {
		return nil, fmt.Errorf("update last login: %w", err)
	}
	u.LastLoginAt = &now
	return &u, nil
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// CreateSession issues a random token and stores only its hash.
func (s *Service) CreateSession(ctx context.Context, userID, ip, userAgent string) (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("session token: %w", err)
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	now := s.nowMs()
	_, err := s.db.ExecContext(ctx, `INSERT INTO sessions (id, user_id, token_hash, ip, user_agent, created_at, expires_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		store.NewULID(), userID, hashToken(token), ip, userAgent, now, now+s.ttl.Milliseconds())
	if err != nil {
		return "", fmt.Errorf("insert session: %w", err)
	}
	return token, nil
}

// UserBySessionToken resolves a live session to its user.
func (s *Service) UserBySessionToken(ctx context.Context, token string) (*User, error) {
	var u User
	err := s.db.GetContext(ctx, &u, `SELECT u.id, u.username, u.display_name, u.password_hash, u.role, u.enabled,
		u.must_change_password, u.last_login_at, u.created_at, u.updated_at
		FROM sessions s JOIN users u ON u.id = s.user_id
		WHERE s.token_hash = ? AND s.expires_at > ? AND u.enabled = 1`, hashToken(token), s.nowMs())
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrSessionInvalid
	}
	if err != nil {
		return nil, fmt.Errorf("query session: %w", err)
	}
	return &u, nil
}

// DeleteSession removes one session by token.
func (s *Service) DeleteSession(ctx context.Context, token string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE token_hash = ?`, hashToken(token))
	if err != nil {
		return fmt.Errorf("delete session: %w", err)
	}
	return nil
}

// DeleteOtherSessions revokes every session of userID except keepToken.
func (s *Service) DeleteOtherSessions(ctx context.Context, userID, keepToken string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE user_id = ? AND token_hash <> ?`,
		userID, hashToken(keepToken))
	if err != nil {
		return fmt.Errorf("delete other sessions: %w", err)
	}
	return nil
}

// ChangePassword verifies the old password and stores a new one.
func (s *Service) ChangePassword(ctx context.Context, userID, oldPassword, newPassword string) error {
	var hash string
	if err := s.db.GetContext(ctx, &hash, `SELECT password_hash FROM users WHERE id = ?`, userID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrInvalidCredentials
		}
		return fmt.Errorf("query password: %w", err)
	}
	if !s.VerifyPassword(hash, oldPassword) {
		return ErrInvalidCredentials
	}
	if err := ValidatePassword(newPassword); err != nil {
		return err
	}
	newHash, err := s.HashPassword(newPassword)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `UPDATE users SET password_hash = ?, must_change_password = 0, updated_at = ? WHERE id = ?`,
		newHash, s.nowMs(), userID)
	if err != nil {
		return fmt.Errorf("update password: %w", err)
	}
	return nil
}

const passwordCharset = "abcdefghjkmnpqrstuvwxyzABCDEFGHJKMNPQRSTUVWXYZ23456789"

func randomPassword(n int) (string, error) {
	var b strings.Builder
	max := big.NewInt(int64(len(passwordCharset)))
	for i := 0; i < n; i++ {
		idx, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", err
		}
		b.WriteByte(passwordCharset[idx.Int64()])
	}
	return b.String(), nil
}

// BootstrapAdmin creates the initial admin account when the users table is empty.
func (s *Service) BootstrapAdmin(ctx context.Context) (string, bool, error) {
	var count int
	if err := s.db.GetContext(ctx, &count, `SELECT count(*) FROM users`); err != nil {
		return "", false, fmt.Errorf("count users: %w", err)
	}
	if count > 0 {
		return "", false, nil
	}
	password, err := randomPassword(16)
	if err != nil {
		return "", false, fmt.Errorf("generate password: %w", err)
	}
	hash, err := s.HashPassword(password)
	if err != nil {
		return "", false, err
	}
	now := s.nowMs()
	_, err = s.db.ExecContext(ctx, `INSERT INTO users (id, username, display_name, password_hash, role, enabled, must_change_password, created_at, updated_at)
		VALUES (?, 'admin', 'Administrator', ?, 'admin', 1, 1, ?, ?)`,
		store.NewULID(), hash, now, now)
	if err != nil {
		return "", false, fmt.Errorf("insert admin: %w", err)
	}
	return password, true, nil
}
