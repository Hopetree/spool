package host

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/Hopetree/spool/internal/credential"
	"github.com/Hopetree/spool/internal/sshx"
	"github.com/Hopetree/spool/internal/store"
)

// ErrNotFound marks a missing host or group.
var ErrNotFound = errors.New("host not found")

// ValidationError marks invalid input.
type ValidationError struct{ Message string }

func (e *ValidationError) Error() string { return e.Message }

func invalid(msg string) error { return &ValidationError{Message: msg} }

// Prober is the SSH probe dependency (fake in tests).
type Prober interface {
	Probe(ctx context.Context, t sshx.Target) (sshx.ProbeResult, error)
}

// Service manages hosts, groups and connectivity tests.
type Service struct {
	db     *sqlx.DB
	creds  *credential.Service
	prober Prober
	policy string
	now    func() time.Time
}

// NewService builds a host Service. policy is tofu or strict.
func NewService(db *sqlx.DB, creds *credential.Service, prober Prober, policy string) *Service {
	return &Service{db: db, creds: creds, prober: prober, policy: policy, now: time.Now}
}

func (s *Service) nowMs() int64 { return s.now().UnixMilli() }

const selectSQL = `SELECT h.id, h.name, h.address, h.port, h.group_id, g.name AS group_name,
	h.credential_id, c.name AS credential_name, h.enabled, h.fingerprint, h.fingerprint_type,
	h.capability_json, h.last_test_at, h.last_test_ok, h.note, h.created_at, h.updated_at,
	(SELECT group_concat(tag, ',') FROM host_tags t WHERE t.host_id = h.id) AS tags_csv
	FROM hosts h
	LEFT JOIN host_groups g ON g.id = h.group_id
	LEFT JOIN credentials c ON c.id = h.credential_id`

type hostRow struct {
	ID              string         `db:"id"`
	Name            string         `db:"name"`
	Address         string         `db:"address"`
	Port            int            `db:"port"`
	GroupID         sql.NullString `db:"group_id"`
	GroupName       sql.NullString `db:"group_name"`
	CredentialID    string         `db:"credential_id"`
	CredentialName  sql.NullString `db:"credential_name"`
	Enabled         bool           `db:"enabled"`
	Fingerprint     string         `db:"fingerprint"`
	FingerprintType string         `db:"fingerprint_type"`
	CapabilityJSON  string         `db:"capability_json"`
	LastTestAt      sql.NullInt64  `db:"last_test_at"`
	LastTestOK      sql.NullBool   `db:"last_test_ok"`
	Note            string         `db:"note"`
	CreatedAt       int64          `db:"created_at"`
	UpdatedAt       int64          `db:"updated_at"`
	TagsCSV         sql.NullString `db:"tags_csv"`
}

func (r hostRow) toHost() Host {
	h := Host{
		ID:              r.ID,
		Name:            r.Name,
		Address:         r.Address,
		Port:            r.Port,
		GroupName:       r.GroupName.String,
		CredentialID:    r.CredentialID,
		CredentialName:  r.CredentialName.String,
		Enabled:         r.Enabled,
		Fingerprint:     r.Fingerprint,
		FingerprintType: r.FingerprintType,
		Note:            r.Note,
		CreatedAt:       r.CreatedAt,
		UpdatedAt:       r.UpdatedAt,
		Tags:            []string{},
	}
	if r.GroupID.Valid {
		v := r.GroupID.String
		h.GroupID = &v
	}
	if r.LastTestAt.Valid {
		v := r.LastTestAt.Int64
		h.LastTestAt = &v
	}
	if r.LastTestOK.Valid {
		v := r.LastTestOK.Bool
		h.LastTestOK = &v
	}
	if r.TagsCSV.Valid && r.TagsCSV.String != "" {
		tags := strings.Split(r.TagsCSV.String, ",")
		sort.Strings(tags)
		h.Tags = tags
	}
	if r.CapabilityJSON != "" {
		_ = json.Unmarshal([]byte(r.CapabilityJSON), &h.Capability)
	}
	return h
}

func normalizeTags(tags []string) []string {
	seen := make(map[string]struct{}, len(tags))
	out := make([]string, 0, len(tags))
	for _, raw := range tags {
		tag := strings.TrimSpace(raw)
		if tag == "" {
			continue
		}
		if _, ok := seen[tag]; ok {
			continue
		}
		seen[tag] = struct{}{}
		out = append(out, tag)
	}
	sort.Strings(out)
	return out
}

func validateInput(in Input) error {
	if strings.TrimSpace(in.Name) == "" {
		return invalid("主机名称不能为空")
	}
	if len(in.Name) > 64 {
		return invalid("主机名称不能超过 64 个字符")
	}
	if strings.TrimSpace(in.Address) == "" {
		return invalid("主机地址不能为空")
	}
	if len(in.Address) > 255 {
		return invalid("主机地址不能超过 255 个字符")
	}
	if in.Port < 1 || in.Port > 65535 {
		return invalid("端口必须在 1-65535 之间")
	}
	if in.CredentialID == "" {
		return invalid("必须选择凭据")
	}
	if len(in.Note) > 500 {
		return invalid("备注不能超过 500 个字符")
	}
	tags := normalizeTags(in.Tags)
	if len(tags) > 20 {
		return invalid("标签最多 20 个")
	}
	for _, tag := range tags {
		if len(tag) > 32 {
			return invalid("单个标签不能超过 32 个字符")
		}
		if strings.Contains(tag, ",") {
			return invalid("标签不能包含逗号")
		}
	}
	return nil
}

func (s *Service) checkRefs(ctx context.Context, in Input) error {
	var n int
	if err := s.db.GetContext(ctx, &n, `SELECT count(*) FROM credentials WHERE id = ?`, in.CredentialID); err != nil {
		return fmt.Errorf("check credential: %w", err)
	}
	if n == 0 {
		return invalid("凭据不存在")
	}
	if in.GroupID != "" {
		if err := s.db.GetContext(ctx, &n, `SELECT count(*) FROM host_groups WHERE id = ?`, in.GroupID); err != nil {
			return fmt.Errorf("check group: %w", err)
		}
		if n == 0 {
			return invalid("分组不存在")
		}
	}
	return nil
}

func (s *Service) insertTags(ctx context.Context, tx *sqlx.Tx, hostID string, tags []string) error {
	for _, tag := range normalizeTags(tags) {
		if _, err := tx.ExecContext(ctx, `INSERT INTO host_tags (host_id, tag) VALUES (?, ?)`, hostID, tag); err != nil {
			return fmt.Errorf("insert tag: %w", err)
		}
	}
	return nil
}

// List returns a page of hosts and the total count.
func (s *Service) List(ctx context.Context, f Filter) ([]Host, int64, error) {
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
	if f.GroupID != "" {
		where = append(where, "h.group_id = ?")
		args = append(args, f.GroupID)
	}
	if f.Tag != "" {
		where = append(where, "EXISTS (SELECT 1 FROM host_tags t WHERE t.host_id = h.id AND t.tag = ?)")
		args = append(args, f.Tag)
	}
	if f.Enabled != nil {
		where = append(where, "h.enabled = ?")
		args = append(args, *f.Enabled)
	}
	if kw := strings.TrimSpace(f.Keyword); kw != "" {
		where = append(where, "(h.name LIKE ? OR h.address LIKE ?)")
		like := "%" + kw + "%"
		args = append(args, like, like)
	}
	cond := strings.Join(where, " AND ")

	var total int64
	if err := s.db.GetContext(ctx, &total, `SELECT count(*) FROM hosts h WHERE `+cond, args...); err != nil {
		return nil, 0, fmt.Errorf("count hosts: %w", err)
	}
	pageArgs := append([]any{}, args...)
	pageArgs = append(pageArgs, f.PageSize, (f.Page-1)*f.PageSize)
	rows := []hostRow{}
	if err := s.db.SelectContext(ctx, &rows, selectSQL+` WHERE `+cond+` ORDER BY h.name LIMIT ? OFFSET ?`, pageArgs...); err != nil {
		return nil, 0, fmt.Errorf("list hosts: %w", err)
	}
	out := make([]Host, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.toHost())
	}
	return out, total, nil
}

// Get returns one host.
func (s *Service) Get(ctx context.Context, id string) (*Host, error) {
	var r hostRow
	err := s.db.GetContext(ctx, &r, selectSQL+` WHERE h.id = ?`, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get host: %w", err)
	}
	h := r.toHost()
	return &h, nil
}

// Create inserts a host and its tags.
func (s *Service) Create(ctx context.Context, in Input) (*Host, error) {
	if in.Port == 0 {
		in.Port = 22
	}
	if err := validateInput(in); err != nil {
		return nil, err
	}
	if err := s.checkRefs(ctx, in); err != nil {
		return nil, err
	}
	id := store.NewULID()
	now := s.nowMs()
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var groupID any
	if in.GroupID != "" {
		groupID = in.GroupID
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO hosts
		(id, name, address, port, group_id, credential_id, enabled, note, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, strings.TrimSpace(in.Name), strings.TrimSpace(in.Address), in.Port, groupID,
		in.CredentialID, in.Enabled, in.Note, now, now)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return nil, invalid("主机名称已存在")
		}
		return nil, fmt.Errorf("insert host: %w", err)
	}
	if err := s.insertTags(ctx, tx, id, in.Tags); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit host: %w", err)
	}
	return s.Get(ctx, id)
}

// Update replaces host fields and tags.
func (s *Service) Update(ctx context.Context, id string, in Input) (*Host, error) {
	if _, err := s.Get(ctx, id); err != nil {
		return nil, err
	}
	if in.Port == 0 {
		in.Port = 22
	}
	if err := validateInput(in); err != nil {
		return nil, err
	}
	if err := s.checkRefs(ctx, in); err != nil {
		return nil, err
	}
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var groupID any
	if in.GroupID != "" {
		groupID = in.GroupID
	}
	_, err = tx.ExecContext(ctx, `UPDATE hosts SET name = ?, address = ?, port = ?, group_id = ?,
		credential_id = ?, enabled = ?, note = ?, updated_at = ? WHERE id = ?`,
		strings.TrimSpace(in.Name), strings.TrimSpace(in.Address), in.Port, groupID,
		in.CredentialID, in.Enabled, in.Note, s.nowMs(), id)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return nil, invalid("主机名称已存在")
		}
		return nil, fmt.Errorf("update host: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM host_tags WHERE host_id = ?`, id); err != nil {
		return nil, fmt.Errorf("clear tags: %w", err)
	}
	if err := s.insertTags(ctx, tx, id, in.Tags); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit host: %w", err)
	}
	return s.Get(ctx, id)
}

// Delete removes a host; tags cascade.
func (s *Service) Delete(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM hosts WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete host: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// SetFingerprint explicitly trusts a host key (admin action).
func (s *Service) SetFingerprint(ctx context.Context, id, fingerprint, keyType string) (*Host, error) {
	if strings.TrimSpace(fingerprint) == "" {
		return nil, invalid("指纹不能为空")
	}
	if strings.TrimSpace(keyType) == "" {
		return nil, invalid("密钥类型不能为空")
	}
	res, err := s.db.ExecContext(ctx, `UPDATE hosts SET fingerprint = ?, fingerprint_type = ?, updated_at = ? WHERE id = ?`,
		fingerprint, keyType, s.nowMs(), id)
	if err != nil {
		return nil, fmt.Errorf("update fingerprint: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, ErrNotFound
	}
	return s.Get(ctx, id)
}

func (s *Service) markTest(ctx context.Context, id string, ok bool) {
	okInt := 0
	if ok {
		okInt = 1
	}
	_, _ = s.db.ExecContext(ctx, `UPDATE hosts SET last_test_at = ?, last_test_ok = ? WHERE id = ?`, s.nowMs(), okInt, id)
}

func (s *Service) saveProbe(ctx context.Context, id string, res sshx.ProbeResult, fingerprint, keyType string) error {
	b, err := json.Marshal(res)
	if err != nil {
		return fmt.Errorf("marshal capability: %w", err)
	}
	_, err = s.db.ExecContext(ctx, `UPDATE hosts SET capability_json = ?, fingerprint = ?, fingerprint_type = ?,
		last_test_at = ?, last_test_ok = 1 WHERE id = ?`,
		string(b), fingerprint, keyType, s.nowMs(), id)
	if err != nil {
		return fmt.Errorf("save probe: %w", err)
	}
	return nil
}

// Test connects to the host, probes capabilities and records the fingerprint on
// first trust. HostKeyError is returned unchanged so the API can show details.
func (s *Service) Test(ctx context.Context, id string) (*TestResult, error) {
	h, err := s.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	authType, username, secret, passphrase, err := s.creds.Open(ctx, h.CredentialID)
	if err != nil {
		s.markTest(ctx, id, false)
		return nil, err
	}
	res, err := s.prober.Probe(ctx, sshx.Target{
		Address:             h.Address,
		Port:                h.Port,
		Username:            username,
		AuthType:            authType,
		Secret:              secret,
		Passphrase:          passphrase,
		ExpectedFingerprint: h.Fingerprint,
		Policy:              s.policy,
	})
	if err != nil {
		s.markTest(ctx, id, false)
		return nil, err
	}
	fingerprint, keyType, recorded := h.Fingerprint, h.FingerprintType, false
	if fingerprint == "" {
		fingerprint, keyType, recorded = res.Fingerprint, res.KeyType, true
	}
	if err := s.saveProbe(ctx, id, res, fingerprint, keyType); err != nil {
		return nil, err
	}
	return &TestResult{
		OK:                  true,
		LatencyMS:           res.LatencyMS,
		Fingerprint:         fingerprint,
		KeyType:             keyType,
		FingerprintRecorded: recorded,
		Capability:          res,
	}, nil
}
