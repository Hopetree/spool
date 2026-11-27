package envars

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/Hopetree/spool/internal/credential"
	"github.com/Hopetree/spool/internal/store"
)

// ErrNotFound marks a missing environment variable.
var ErrNotFound = errors.New("environment variable not found")

// ValidationError marks invalid input.
type ValidationError struct{ Message string }

func (e *ValidationError) Error() string { return e.Message }

func invalid(msg string) error { return &ValidationError{Message: msg} }

var envNameRe = regexp.MustCompile(`^SP_[A-Z_]+$`)

// EnvVar is the public view of a platform environment variable. Sensitive
// values are only exposed through RevealValue.
type EnvVar struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Value         string `json:"value,omitempty"`
	Description   string `json:"description"`
	Sensitive     bool   `json:"sensitive"`
	UpdatedAt     int64  `json:"updated_at"`
	CreatedAt     int64  `json:"created_at"`
	UpdatedByName string `json:"updated_by_name"`
}

// Input carries create/update fields. Empty Value on update means "unchanged".
type Input struct {
	Name        string
	Value       string
	Description string
	Sensitive   bool
}

type row struct {
	ID            string `db:"id"`
	Name          string `db:"name"`
	Description   string `db:"description"`
	Sensitive     bool   `db:"sensitive"`
	ValueEnc      []byte `db:"value_enc"`
	CreatedBy     string `db:"created_by"`
	UpdatedBy     string `db:"updated_by"`
	CreatedAt     int64  `db:"created_at"`
	UpdatedAt     int64  `db:"updated_at"`
	UpdatedByName string `db:"updated_by_name"`
}

func (r row) view(cipher *credential.Cipher, revealValue bool) EnvVar {
	value := ""
	if revealValue {
		plain, err := cipher.Decrypt(r.ValueEnc, r.ID)
		if err != nil {
			return EnvVar{}
		}
		value = string(plain)
	}
	return EnvVar{
		ID: r.ID, Name: r.Name, Value: value, Description: r.Description, Sensitive: r.Sensitive,
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt, UpdatedByName: r.UpdatedByName,
	}
}

// Service stores encrypted platform environment variables.
type Service struct {
	db     *sqlx.DB
	cipher *credential.Cipher
	now    func() time.Time
}

// NewService builds an envars Service.
func NewService(db *sqlx.DB, cipher *credential.Cipher) *Service {
	return &Service{db: db, cipher: cipher, now: time.Now}
}

func (s *Service) nowMs() int64 { return s.now().UnixMilli() }

func validateName(name string) error {
	if name == "" {
		return invalid("变量名不能为空")
	}
	if len(name) > 64 {
		return invalid("变量名不能超过 64 个字符")
	}
	if !envNameRe.MatchString(name) {
		return invalid("变量名必须使用统一前缀 SP_，且只能包含大写字母和下划线")
	}
	return nil
}

func validate(in *Input, requireValue bool) error {
	in.Name = strings.TrimSpace(in.Name)
	if err := validateName(in.Name); err != nil {
		return err
	}
	if requireValue && strings.TrimSpace(in.Value) == "" {
		return invalid("变量值不能为空")
	}
	if len(in.Value) > 4096 {
		return invalid("变量值不能超过 4096 个字符")
	}
	in.Description = strings.TrimSpace(in.Description)
	if len(in.Description) > 200 {
		return invalid("说明不能超过 200 个字符")
	}
	return nil
}

const selectColumns = `e.id, e.name, e.description, e.sensitive, e.value_enc, e.created_by, e.updated_by,
	e.created_at, e.updated_at,
	COALESCE(NULLIF(u.username, ''), NULLIF(u.display_name, ''), e.updated_by) AS updated_by_name`

// List returns all platform environment variables without values.
func (s *Service) List(ctx context.Context) ([]EnvVar, error) {
	rows := []row{}
	query := `SELECT ` + selectColumns + ` FROM platform_env_vars e
		LEFT JOIN users u ON u.id = e.updated_by ORDER BY e.name`
	if err := s.db.SelectContext(ctx, &rows, query); err != nil {
		return nil, fmt.Errorf("list env vars: %w", err)
	}
	out := make([]EnvVar, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.view(s.cipher, !r.Sensitive))
	}
	return out, nil
}

func uniqueNameError(err error) error {
	if err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed") {
		return invalid("环境变量名已存在")
	}
	return nil
}

// Create stores a new environment variable.
func (s *Service) Create(ctx context.Context, in Input, actorID string) (*EnvVar, error) {
	if err := validate(&in, true); err != nil {
		return nil, err
	}
	id := store.NewULID()
	valueEnc, err := s.cipher.Encrypt([]byte(in.Value), id)
	if err != nil {
		return nil, fmt.Errorf("encrypt env value: %w", err)
	}
	now := s.nowMs()
	if _, err := s.db.ExecContext(ctx, `INSERT INTO platform_env_vars
		(id, name, description, sensitive, value_enc, created_by, updated_by, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, in.Name, in.Description, in.Sensitive, valueEnc, actorID, actorID, now, now); err != nil {
		if ue := uniqueNameError(err); ue != nil {
			return nil, ue
		}
		return nil, fmt.Errorf("insert env var: %w", err)
	}
	return s.Get(ctx, id)
}

// Get returns one environment variable's metadata.
func (s *Service) Get(ctx context.Context, id string) (*EnvVar, error) {
	var r row
	query := `SELECT ` + selectColumns + ` FROM platform_env_vars e
		LEFT JOIN users u ON u.id = e.updated_by WHERE e.id = ?`
	if err := s.db.GetContext(ctx, &r, query, id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get env var: %w", err)
	}
	view := r.view(s.cipher, false)
	return &view, nil
}

// Update edits metadata or value. An empty value keeps the stored value.
func (s *Service) Update(ctx context.Context, id string, in Input, actorID string) (*EnvVar, error) {
	if err := validate(&in, false); err != nil {
		return nil, err
	}
	existing, err := s.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	var valueEnc []byte
	if strings.TrimSpace(in.Value) != "" {
		valueEnc, err = s.cipher.Encrypt([]byte(in.Value), id)
		if err != nil {
			return nil, fmt.Errorf("encrypt env value: %w", err)
		}
	}
	now := s.nowMs()
	if valueEnc != nil {
		_, err = s.db.ExecContext(ctx, `UPDATE platform_env_vars SET name=?, description=?, sensitive=?, value_enc=?, updated_by=?, updated_at=? WHERE id=?`,
			in.Name, in.Description, in.Sensitive, valueEnc, actorID, now, id)
	} else {
		_, err = s.db.ExecContext(ctx, `UPDATE platform_env_vars SET name=?, description=?, sensitive=?, updated_by=?, updated_at=? WHERE id=?`,
			in.Name, in.Description, in.Sensitive, actorID, now, id)
	}
	if err != nil {
		if ue := uniqueNameError(err); ue != nil {
			return nil, ue
		}
		return nil, fmt.Errorf("update env var: %w", err)
	}
	_ = existing
	return s.Get(ctx, id)
}

// RevealValue decrypts and returns one variable value.
func (s *Service) RevealValue(ctx context.Context, id string) (string, error) {
	var r row
	if err := s.db.GetContext(ctx, &r, `SELECT id, name, value_enc FROM platform_env_vars WHERE id=?`, id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", ErrNotFound
		}
		return "", fmt.Errorf("get env var: %w", err)
	}
	plain, err := s.cipher.Decrypt(r.ValueEnc, r.ID)
	if err != nil {
		return "", fmt.Errorf("decrypt env var: %w", err)
	}
	return string(plain), nil
}

// Delete removes an environment variable.
func (s *Service) Delete(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM platform_env_vars WHERE id=?`, id)
	if err != nil {
		return fmt.Errorf("delete env var: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// Values decrypts all variables for execution injection.
func (s *Service) Values(ctx context.Context) (map[string]string, error) {
	rows := []row{}
	if err := s.db.SelectContext(ctx, &rows, `SELECT id, name, value_enc FROM platform_env_vars ORDER BY name`); err != nil {
		return nil, fmt.Errorf("load env vars: %w", err)
	}
	out := make(map[string]string, len(rows))
	for _, r := range rows {
		plain, err := s.cipher.Decrypt(r.ValueEnc, r.ID)
		if err != nil {
			return nil, fmt.Errorf("decrypt env var %q: %w", r.Name, err)
		}
		out[r.Name] = string(plain)
	}
	return out, nil
}
