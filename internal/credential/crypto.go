package credential

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jmoiron/sqlx"
)

// ErrMasterKeyMismatch means the configured master key cannot decrypt stored
// credentials (usually a lost or replaced key file).
var ErrMasterKeyMismatch = errors.New("master key does not match stored credentials")

// Cipher encrypts and decrypts credential secrets with AES-256-GCM.
type Cipher struct {
	aead cipher.AEAD
}

// LoadOrCreateMasterKey reads a base64-encoded 32-byte key or creates one with
// mode 0600 on first use.
func LoadOrCreateMasterKey(path string) ([]byte, error) {
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("create key dir: %w", err)
		}
	}
	raw, err := os.ReadFile(path)
	if err == nil {
		key, derr := base64.StdEncoding.DecodeString(strings.TrimSpace(string(raw)))
		if derr != nil {
			return nil, fmt.Errorf("decode master key: %w", derr)
		}
		if len(key) != 32 {
			return nil, fmt.Errorf("master key must be 32 bytes, got %d", len(key))
		}
		return key, nil
	}
	if !os.IsNotExist(err) {
		return nil, fmt.Errorf("read master key: %w", err)
	}

	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("generate master key: %w", err)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		if os.IsExist(err) {
			return LoadOrCreateMasterKey(path)
		}
		return nil, fmt.Errorf("create master key: %w", err)
	}
	defer f.Close()
	if _, err := f.WriteString(base64.StdEncoding.EncodeToString(key) + "\n"); err != nil {
		return nil, fmt.Errorf("write master key: %w", err)
	}
	return key, nil
}

// NewCipher builds a Cipher from a 32-byte key.
func NewCipher(key []byte) (*Cipher, error) {
	if len(key) != 32 {
		return nil, fmt.Errorf("key must be 32 bytes, got %d", len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("aes: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("gcm: %w", err)
	}
	return &Cipher{aead: aead}, nil
}

// Encrypt returns nonce||ciphertext bound to aad.
func (c *Cipher) Encrypt(plaintext []byte, aad string) ([]byte, error) {
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("nonce: %w", err)
	}
	return c.aead.Seal(nonce, nonce, plaintext, []byte(aad)), nil
}

// Decrypt opens nonce||ciphertext bound to aad.
func (c *Cipher) Decrypt(ciphertext []byte, aad string) ([]byte, error) {
	ns := c.aead.NonceSize()
	if len(ciphertext) < ns {
		return nil, errors.New("ciphertext too short")
	}
	plaintext, err := c.aead.Open(nil, ciphertext[:ns], ciphertext[ns:], []byte(aad))
	if err != nil {
		return nil, fmt.Errorf("decrypt: %w", err)
	}
	return plaintext, nil
}

// CheckMasterKey verifies the key against an existing credential row, if any.
func CheckMasterKey(ctx context.Context, db *sqlx.DB, c *Cipher) error {
	var row struct {
		ID        string `db:"id"`
		SecretEnc []byte `db:"secret_enc"`
	}
	err := db.GetContext(ctx, &row, `SELECT id, secret_enc FROM credentials LIMIT 1`)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("query credentials: %w", err)
	}
	if _, err := c.Decrypt(row.SecretEnc, row.ID); err != nil {
		return ErrMasterKeyMismatch
	}
	return nil
}
