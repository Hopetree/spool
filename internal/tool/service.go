package tool

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/Hopetree/spool/internal/store"
)

type toolRow struct {
	ID               string `db:"id"`
	Name             string `db:"name"`
	Description      string `db:"description"`
	Enabled          bool   `db:"enabled"`
	CurrentVersionID string `db:"current_version_id"`
	CreatedAt        int64  `db:"created_at"`
	UpdatedAt        int64  `db:"updated_at"`
	VersionNo        int    `db:"version_no"`
	Type             string `db:"type"`
	Interpreter      string `db:"interpreter"`
	ParamsJSON       string `db:"params_json"`
	TimeoutSec       int    `db:"default_timeout_sec"`
	Sudo             bool   `db:"sudo"`
	ContentHash      string `db:"content_hash"`
}

type versionRow struct {
	ID             string `db:"id"`
	ToolID         string `db:"tool_id"`
	VersionNo      int    `db:"version_no"`
	Type           string `db:"type"`
	Interpreter    string `db:"interpreter"`
	Content        string `db:"content"`
	ParamsJSON     string `db:"params_json"`
	PrepareCommand string `db:"prepare_command"`
	TimeoutSec     int    `db:"default_timeout_sec"`
	Sudo           bool   `db:"sudo"`
	ContentHash    string `db:"content_hash"`
	ChangeNote     string `db:"change_note"`
	CreatedBy      string `db:"created_by"`
	CreatedAt      int64  `db:"created_at"`
}

func (r versionRow) toVersion() (Version, error) {
	params := []Param{}
	if r.ParamsJSON != "" {
		if err := json.Unmarshal([]byte(r.ParamsJSON), &params); err != nil {
			return Version{}, fmt.Errorf("parse params: %w", err)
		}
	}
	return Version{
		ID:             r.ID,
		ToolID:         r.ToolID,
		VersionNo:      r.VersionNo,
		Type:           r.Type,
		Interpreter:    r.Interpreter,
		Content:        r.Content,
		Params:         params,
		PrepareCommand: r.PrepareCommand,
		TimeoutSec:     r.TimeoutSec,
		Sudo:           r.Sudo,
		ContentHash:    r.ContentHash,
		ChangeNote:     r.ChangeNote,
		CreatedBy:      r.CreatedBy,
		CreatedAt:      r.CreatedAt,
	}, nil
}

// Service manages tools and immutable versions.
type Service struct {
	db  *sqlx.DB
	now func() time.Time
}

// NewService builds a tool Service.
func NewService(db *sqlx.DB) *Service { return &Service{db: db, now: time.Now} }

func (s *Service) nowMs() int64 { return s.now().UnixMilli() }

const toolSelect = `SELECT t.id, t.name, t.description, t.enabled, t.current_version_id, t.created_at, t.updated_at,
	v.version_no, v.type, v.interpreter, v.params_json, v.default_timeout_sec, v.sudo, v.content_hash
	FROM tools t JOIN tool_versions v ON v.id = t.current_version_id`

func (r toolRow) toTool() Tool {
	params := []Param{}
	if r.ParamsJSON != "" {
		_ = json.Unmarshal([]byte(r.ParamsJSON), &params)
	}
	return Tool{
		ID:          r.ID,
		Name:        r.Name,
		Description: r.Description,
		Enabled:     r.Enabled,
		VersionNo:   r.VersionNo,
		Type:        r.Type,
		Interpreter: r.Interpreter,
		TimeoutSec:  r.TimeoutSec,
		Sudo:        r.Sudo,
		ContentHash: r.ContentHash,
		ParamCount:  len(params),
		CreatedAt:   r.CreatedAt,
		UpdatedAt:   r.UpdatedAt,
	}
}

// List returns a page of tools and the total count.
func (s *Service) List(ctx context.Context, f ListFilter) ([]Tool, int64, error) {
	if f.Page < 1 {
		f.Page = 1
	}
	if f.PageSize < 1 {
		f.PageSize = 20
	}
	if f.PageSize > 100 {
		f.PageSize = 100
	}
	where := []string{"1=1"}
	args := []any{}
	if kw := strings.TrimSpace(f.Keyword); kw != "" {
		where = append(where, "(t.name LIKE ? OR t.description LIKE ?)")
		like := "%" + kw + "%"
		args = append(args, like, like)
	}
	if f.Enabled != nil {
		where = append(where, "t.enabled = ?")
		args = append(args, *f.Enabled)
	}
	cond := strings.Join(where, " AND ")
	var total int64
	if err := s.db.GetContext(ctx, &total, `SELECT count(*) FROM tools t WHERE `+cond, args...); err != nil {
		return nil, 0, fmt.Errorf("count tools: %w", err)
	}
	pageArgs := append([]any{}, args...)
	pageArgs = append(pageArgs, f.PageSize, (f.Page-1)*f.PageSize)
	rows := []toolRow{}
	if err := s.db.SelectContext(ctx, &rows, toolSelect+` WHERE `+cond+` ORDER BY t.name LIMIT ? OFFSET ?`, pageArgs...); err != nil {
		return nil, 0, fmt.Errorf("list tools: %w", err)
	}
	out := make([]Tool, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.toTool())
	}
	return out, total, nil
}

// Get returns a tool with its current version.
func (s *Service) Get(ctx context.Context, id string) (*Detail, error) {
	var t toolRow
	err := s.db.GetContext(ctx, &t, toolSelect+` WHERE t.id = ?`, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get tool: %w", err)
	}
	var v versionRow
	if err := s.db.GetContext(ctx, &v, `SELECT * FROM tool_versions WHERE id = ?`, t.CurrentVersionID); err != nil {
		return nil, fmt.Errorf("get current version: %w", err)
	}
	version, err := v.toVersion()
	if err != nil {
		return nil, err
	}
	return &Detail{Tool: t.toTool(), Version: version}, nil
}

func marshalParams(params []Param) (string, error) {
	if params == nil {
		params = []Param{}
	}
	b, err := json.Marshal(params)
	if err != nil {
		return "", fmt.Errorf("marshal params: %w", err)
	}
	return string(b), nil
}

func (s *Service) insertVersion(ctx context.Context, tx *sqlx.Tx, toolID string, in Input, versionNo int, actorID string) (string, error) {
	paramsJSON, err := marshalParams(in.Params)
	if err != nil {
		return "", err
	}
	id := store.NewULID()
	_, err = tx.ExecContext(ctx, `INSERT INTO tool_versions
		(id, tool_id, version_no, type, interpreter, content, params_json, prepare_command,
		 default_timeout_sec, sudo, content_hash, change_note, created_by, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, toolID, versionNo, in.Type, in.Interpreter, in.Content, paramsJSON, in.PrepareCommand,
		in.TimeoutSec, in.Sudo, HashContent(in.Content), in.ChangeNote, actorID, s.nowMs())
	if err != nil {
		return "", fmt.Errorf("insert version: %w", err)
	}
	return id, nil
}

func uniqueNameError(err error) error {
	if err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed") {
		return invalid("工具名称已存在")
	}
	return nil
}

// Create inserts a tool with version 1.
func (s *Service) Create(ctx context.Context, in Input, actorID string) (*Detail, error) {
	if err := validateInput(&in); err != nil {
		return nil, err
	}
	id := store.NewULID()
	now := s.nowMs()
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	_, err = tx.ExecContext(ctx, `INSERT INTO tools
		(id, name, description, enabled, current_version_id, created_by, created_at, updated_at)
		VALUES (?, ?, ?, 1, '', ?, ?, ?)`,
		id, in.Name, in.Description, actorID, now, now)
	if err != nil {
		if ue := uniqueNameError(err); ue != nil {
			return nil, ue
		}
		return nil, fmt.Errorf("insert tool: %w", err)
	}
	versionID, err := s.insertVersion(ctx, tx, id, in, 1, actorID)
	if err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE tools SET current_version_id = ? WHERE id = ?`, versionID, id); err != nil {
		return nil, fmt.Errorf("set current version: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit tool: %w", err)
	}
	return s.Get(ctx, id)
}

func (s *Service) nextVersionNo(ctx context.Context, tx *sqlx.Tx, toolID string) (int, error) {
	var no int
	err := tx.GetContext(ctx, &no, `SELECT COALESCE(max(version_no), 0) + 1 FROM tool_versions WHERE tool_id = ?`, toolID)
	if err != nil {
		return 0, fmt.Errorf("next version: %w", err)
	}
	return no, nil
}

// Update creates a new version and makes it current.
func (s *Service) Update(ctx context.Context, id string, in Input, actorID string) (*Detail, error) {
	if _, err := s.Get(ctx, id); err != nil {
		return nil, err
	}
	if err := validateInput(&in); err != nil {
		return nil, err
	}
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	no, err := s.nextVersionNo(ctx, tx, id)
	if err != nil {
		return nil, err
	}
	versionID, err := s.insertVersion(ctx, tx, id, in, no, actorID)
	if err != nil {
		return nil, err
	}
	_, err = tx.ExecContext(ctx, `UPDATE tools SET name = ?, description = ?, current_version_id = ?, updated_at = ? WHERE id = ?`,
		in.Name, in.Description, versionID, s.nowMs(), id)
	if err != nil {
		if ue := uniqueNameError(err); ue != nil {
			return nil, ue
		}
		return nil, fmt.Errorf("update tool: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit tool: %w", err)
	}
	return s.Get(ctx, id)
}

// SetEnabled toggles whether a tool can be selected.
func (s *Service) SetEnabled(ctx context.Context, id string, enabled bool) error {
	res, err := s.db.ExecContext(ctx, `UPDATE tools SET enabled = ?, updated_at = ? WHERE id = ?`, enabled, s.nowMs(), id)
	if err != nil {
		return fmt.Errorf("set enabled: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// Delete removes a tool only when it has no execution history.
func (s *Service) Delete(ctx context.Context, id string) error {
	if _, err := s.Get(ctx, id); err != nil {
		return err
	}
	var history int
	err := s.db.GetContext(ctx, &history, `SELECT count(*) FROM step_runs s
		JOIN tool_versions v ON v.id = s.tool_version_id WHERE v.tool_id = ?`, id)
	if err != nil {
		return fmt.Errorf("check history: %w", err)
	}
	if history > 0 {
		return ErrHasHistory
	}
	if _, err := s.db.ExecContext(ctx, `DELETE FROM tools WHERE id = ?`, id); err != nil {
		return fmt.Errorf("delete tool: %w", err)
	}
	return nil
}

// Versions lists all versions newest first.
func (s *Service) Versions(ctx context.Context, toolID string) ([]Version, error) {
	if _, err := s.Get(ctx, toolID); err != nil {
		return nil, err
	}
	rows := []versionRow{}
	if err := s.db.SelectContext(ctx, &rows, `SELECT * FROM tool_versions WHERE tool_id = ? ORDER BY version_no DESC`, toolID); err != nil {
		return nil, fmt.Errorf("list versions: %w", err)
	}
	out := make([]Version, 0, len(rows))
	for _, r := range rows {
		v, err := r.toVersion()
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, nil
}

// Version returns one specific version.
func (s *Service) Version(ctx context.Context, toolID string, versionNo int) (*Version, error) {
	var r versionRow
	err := s.db.GetContext(ctx, &r, `SELECT * FROM tool_versions WHERE tool_id = ? AND version_no = ?`, toolID, versionNo)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get version: %w", err)
	}
	v, err := r.toVersion()
	if err != nil {
		return nil, err
	}
	return &v, nil
}

// Rollback copies an old version's content into a new current version.
func (s *Service) Rollback(ctx context.Context, toolID string, versionNo int, note, actorID string) (*Detail, error) {
	current, err := s.Get(ctx, toolID)
	if err != nil {
		return nil, err
	}
	target, err := s.Version(ctx, toolID, versionNo)
	if err != nil {
		return nil, err
	}
	if note == "" {
		note = fmt.Sprintf("回滚到 v%d", versionNo)
	}
	return s.Update(ctx, toolID, Input{
		Name:           current.Name,
		Description:    current.Description,
		Type:           target.Type,
		Interpreter:    target.Interpreter,
		Content:        target.Content,
		PrepareCommand: target.PrepareCommand,
		Params:         target.Params,
		TimeoutSec:     target.TimeoutSec,
		Sudo:           target.Sudo,
		ChangeNote:     note,
	}, actorID)
}

// Copy clones a tool's current content under a new name.
func (s *Service) Copy(ctx context.Context, toolID, actorID string) (*Detail, error) {
	src, err := s.Get(ctx, toolID)
	if err != nil {
		return nil, err
	}
	base := src.Name + "-copy"
	name := base
	for i := 2; i < 100; i++ {
		var n int
		if err := s.db.GetContext(ctx, &n, `SELECT count(*) FROM tools WHERE name = ?`, name); err != nil {
			return nil, fmt.Errorf("check name: %w", err)
		}
		if n == 0 {
			break
		}
		name = fmt.Sprintf("%s-%d", base, i)
	}
	return s.Create(ctx, Input{
		Name:           name,
		Description:    src.Description,
		Type:           src.Version.Type,
		Interpreter:    src.Version.Interpreter,
		Content:        src.Version.Content,
		PrepareCommand: src.Version.PrepareCommand,
		Params:         src.Version.Params,
		TimeoutSec:     src.Version.TimeoutSec,
		Sudo:           src.Version.Sudo,
		ChangeNote:     fmt.Sprintf("复制自 %s v%d", src.Name, src.Version.VersionNo),
	}, actorID)
}
