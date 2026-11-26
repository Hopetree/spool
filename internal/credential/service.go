package credential

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/Hopetree/spool/internal/store"
)

// Credential is the public view of a stored credential. It never carries secrets.
type Credential struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	AuthType      string `json:"auth_type"`
	Username      string `json:"username"`
	Description   string `json:"description"`
	HasPassphrase bool   `json:"has_passphrase"`
	CreatedAt     int64  `json:"created_at"`
	UpdatedAt     int64  `json:"updated_at"`
}

// Input carries create/update fields. Empty Secret on update means "unchanged".
type Input struct {
	Name        string
	AuthType    string
	Username    string
	Secret      string
	Passphrase  string
	Description string
}

// ValidationError marks invalid input.
type ValidationError struct{ Message string }

func (e *ValidationError) Error() string { return e.Message }

// ErrInUse reports a credential still referenced by hosts.
type ErrInUse struct{ Hosts int }

func (e *ErrInUse) Error() string {
	return fmt.Sprintf("凭据仍被 %d 台主机引用", e.Hosts)
}

// ErrNotFound marks a missing credential.
var ErrNotFound = errors.New("credential not found")

type row struct {
	ID            string `db:"id"`
	Name          string `db:"name"`
	AuthType      string `db:"auth_type"`
	Username      string `db:"username"`
	Description   string `db:"description"`
	SecretEnc     []byte `db:"secret_enc"`
	PassphraseEnc []byte `db:"passphrase_enc"`
	CreatedAt     int64  `db:"created_at"`
	UpdatedAt     int64  `db:"updated_at"`
}

func (r row) view() *Credential {
	return &Credential{
		ID:            r.ID,
		Name:          r.Name,
		AuthType:      r.AuthType,
		Username:      r.Username,
		Description:   r.Description,
		HasPassphrase: len(r.PassphraseEnc) > 0,
		CreatedAt:     r.CreatedAt,
		UpdatedAt:     r.UpdatedAt,
	}
}

// Service stores encrypted credentials.
type Service struct {
	db     *sqlx.DB
	cipher *Cipher
	now    func() time.Time
}

// NewService builds a credential Service.
func NewService(db *sqlx.DB, cipher *Cipher) *Service {
	return &Service{db: db, cipher: cipher, now: time.Now}
}

func invalid(msg string) error { return &ValidationError{Message: msg} }

func validate(in Input, requireSecret bool) error {
	in.Name = strings.TrimSpace(in.Name)
	in.Username = strings.TrimSpace(in.Username)
	if in.Name == "" {
		return invalid("名称不能为空")
	}
	if len(in.Name) > 64 {
		return invalid("名称不能超过 64 个字符")
	}
	if in.AuthType != "password" && in.AuthType != "private_key" {
		return invalid("认证方式必须是 password 或 private_key")
	}
	if in.Username == "" {
		return invalid("登录用户名不能为空")
	}
	if len(in.Username) > 64 {
		return invalid("登录用户名不能超过 64 个字符")
	}
	if requireSecret && in.Secret == "" {
		return invalid("密码或私钥不能为空")
	}
	if in.AuthType == "password" && in.Passphrase != "" {
		return invalid("密码认证不能设置私钥口令")
	}
	if len(in.Description) > 255 {
		return invalid("描述不能超过 255 个字符")
	}
	return nil
}

func (s *Service) nowMs() int64 { return s.now().UnixMilli() }

const selectColumns = `id, name, auth_type, username, description, secret_enc, passphrase_enc, created_at, updated_at`

// List returns all credentials ordered by name.
func (s *Service) List(ctx context.Context) ([]Credential, error) {
	rows := []row{}
	if err := s.db.SelectContext(ctx, &rows, `SELECT `+selectColumns+` FROM credentials ORDER BY name`); err != nil {
		return nil, fmt.Errorf("list credentials: %w", err)
	}
	out := make([]Credential, 0, len(rows))
	for _, r := range rows {
		out = append(out, *r.view())
	}
	return out, nil
}

// Get returns one credential's metadata.
func (s *Service) Get(ctx context.Context, id string) (*Credential, error) {
	r, err := s.load(ctx, id)
	if err != nil {
		return nil, err
	}
	return r.view(), nil
}

func (s *Service) load(ctx context.Context, id string) (row, error) {
	var r row
	err := s.db.GetContext(ctx, &r, `SELECT `+selectColumns+` FROM credentials WHERE id = ?`, id)
	if errors.Is(err, sql.ErrNoRows) {
		return row{}, ErrNotFound
	}
	if err != nil {
		return row{}, fmt.Errorf("get credential: %w", err)
	}
	return r, nil
}

// Create encrypts and stores a new credential.
func (s *Service) Create(ctx context.Context, in Input, actorID string) (*Credential, error) {
	if err := validate(in, true); err != nil {
		return nil, err
	}
	id := store.NewULID()
	secretEnc, err := s.cipher.Encrypt([]byte(in.Secret), id)
	if err != nil {
		return nil, err
	}
	var passEnc []byte
	if in.Passphrase != "" {
		passEnc, err = s.cipher.Encrypt([]byte(in.Passphrase), id)
		if err != nil {
			return nil, err
		}
	}
	now := s.nowMs()
	_, err = s.db.ExecContext(ctx, `INSERT INTO credentials
		(id, name, auth_type, username, secret_enc, passphrase_enc, description, created_by, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, strings.TrimSpace(in.Name), in.AuthType, strings.TrimSpace(in.Username),
		secretEnc, passEnc, in.Description, actorID, now, now)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return nil, invalid("凭据名称已存在")
		}
		return nil, fmt.Errorf("insert credential: %w", err)
	}
	return s.Get(ctx, id)
}

// Update modifies a credential. Empty Secret/Passphrase means keep the old value;
// switching auth_type requires a new Secret.
func (s *Service) Update(ctx context.Context, id string, in Input) (*Credential, error) {
	existing, err := s.load(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := validate(in, in.Secret != ""); err != nil {
		return nil, err
	}
	if in.AuthType != existing.AuthType && in.Secret == "" {
		return nil, invalid("切换认证方式时必须提供新的密码或私钥")
	}
	secretEnc := existing.SecretEnc
	if in.Secret != "" {
		secretEnc, err = s.cipher.Encrypt([]byte(in.Secret), id)
		if err != nil {
			return nil, err
		}
	}
	passEnc := existing.PassphraseEnc
	if in.AuthType == "password" {
		passEnc = nil
	} else if in.Passphrase != "" {
		passEnc, err = s.cipher.Encrypt([]byte(in.Passphrase), id)
		if err != nil {
			return nil, err
		}
	}
	_, err = s.db.ExecContext(ctx, `UPDATE credentials
		SET name = ?, auth_type = ?, username = ?, secret_enc = ?, passphrase_enc = ?, description = ?, updated_at = ?
		WHERE id = ?`,
		strings.TrimSpace(in.Name), in.AuthType, strings.TrimSpace(in.Username),
		secretEnc, passEnc, in.Description, s.nowMs(), id)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return nil, invalid("凭据名称已存在")
		}
		return nil, fmt.Errorf("update credential: %w", err)
	}
	return s.Get(ctx, id)
}

// Delete removes an unreferenced credential.
func (s *Service) Delete(ctx context.Context, id string) error {
	if _, err := s.load(ctx, id); err != nil {
		return err
	}
	var hosts int
	if err := s.db.GetContext(ctx, &hosts, `SELECT count(*) FROM hosts WHERE credential_id = ?`, id); err != nil {
		return fmt.Errorf("count hosts: %w", err)
	}
	if hosts > 0 {
		return &ErrInUse{Hosts: hosts}
	}
	res, err := s.db.ExecContext(ctx, `DELETE FROM credentials WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete credential: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// Open decrypts a credential for internal use (SSH dialing). Never expose the
// returned values through the API.
func (s *Service) Open(ctx context.Context, id string) (authType, username, secret, passphrase string, err error) {
	r, err := s.load(ctx, id)
	if err != nil {
		return "", "", "", "", err
	}
	secretBytes, err := s.cipher.Decrypt(r.SecretEnc, id)
	if err != nil {
		return "", "", "", "", ErrMasterKeyMismatch
	}
	var passBytes []byte
	if len(r.PassphraseEnc) > 0 {
		passBytes, err = s.cipher.Decrypt(r.PassphraseEnc, id)
		if err != nil {
			return "", "", "", "", ErrMasterKeyMismatch
		}
	}
	return r.AuthType, r.Username, string(secretBytes), string(passBytes), nil
}

// EncryptSensitive seals arbitrary sensitive values (e.g. execution params).
func (s *Service) EncryptSensitive(plaintext []byte, aad string) ([]byte, error) {
	return s.cipher.Encrypt(plaintext, aad)
}

// DecryptSensitive opens values sealed by EncryptSensitive.
func (s *Service) DecryptSensitive(ciphertext []byte, aad string) ([]byte, error) {
	return s.cipher.Decrypt(ciphertext, aad)
}
