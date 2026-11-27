package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/Hopetree/spool/internal/store"
)

// Entry is one audit record to append.
type Entry struct {
	ActorID      string
	ActorName    string
	Action       string
	ResourceType string
	ResourceID   string
	ResourceName string
	Summary      map[string]any
	IP           string
	UserAgent    string
}

// Record is a stored audit row.
type Record struct {
	ID               string `db:"id" json:"id"`
	TS               int64  `db:"ts" json:"ts"`
	UserID           string `db:"user_id" json:"user_id"`
	UsernameSnapshot string `db:"username_snapshot" json:"username_snapshot"`
	Action           string `db:"action" json:"action"`
	ResourceType     string `db:"resource_type" json:"resource_type"`
	ResourceID       string `db:"resource_id" json:"resource_id"`
	ResourceName     string `db:"resource_name" json:"resource_name"`
	SummaryJSON      string `db:"summary_json" json:"summary_json"`
	IP               string `db:"ip" json:"ip"`
	UserAgent        string `db:"user_agent" json:"user_agent"`
}

// Filter narrows audit list queries.
type Filter struct {
	ActorID  string
	Action   string
	FromTS   int64
	ToTS     int64
	Page     int
	PageSize int
}

// Service appends and queries audit records. There is no update or delete API.
type Service struct {
	db  *sqlx.DB
	now func() time.Time
}

// NewService builds an audit Service.
func NewService(db *sqlx.DB) *Service {
	return &Service{db: db, now: time.Now}
}

// Write appends one audit record.
func (s *Service) Write(ctx context.Context, e Entry) error {
	summary := e.Summary
	if summary == nil {
		summary = map[string]any{}
	}
	b, err := json.Marshal(summary)
	if err != nil {
		return fmt.Errorf("marshal summary: %w", err)
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO audit_logs
		(id, ts, user_id, username_snapshot, action, resource_type, resource_id, resource_name, summary_json, ip, user_agent)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		store.NewULID(), s.now().UnixMilli(), e.ActorID, e.ActorName, e.Action,
		e.ResourceType, e.ResourceID, e.ResourceName, string(b), e.IP, e.UserAgent)
	if err != nil {
		return fmt.Errorf("insert audit: %w", err)
	}
	return nil
}

// List returns a page of audit records plus the total matching count.
func (s *Service) List(ctx context.Context, f Filter) ([]Record, int64, error) {
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
	if f.ActorID != "" {
		where = append(where, "user_id = ?")
		args = append(args, f.ActorID)
	}
	if f.Action != "" {
		where = append(where, "action = ?")
		args = append(args, f.Action)
	}
	if f.FromTS > 0 {
		where = append(where, "ts >= ?")
		args = append(args, f.FromTS)
	}
	if f.ToTS > 0 {
		where = append(where, "ts <= ?")
		args = append(args, f.ToTS)
	}
	cond := strings.Join(where, " AND ")

	var total int64
	if err := s.db.GetContext(ctx, &total, "SELECT count(*) FROM audit_logs WHERE "+cond, args...); err != nil {
		return nil, 0, fmt.Errorf("count audit: %w", err)
	}

	args = append(args, f.PageSize, (f.Page-1)*f.PageSize)
	rows := []Record{}
	// Older records (especially host connectivity tests) stored only the
	// resource ID. Fall back to the current resource name when the snapshot is
	// empty so the audit page does not show opaque IDs.
	err := s.db.SelectContext(ctx, &rows, `SELECT a.id, a.ts, a.user_id, a.username_snapshot, a.action, a.resource_type,
		a.resource_id,
		COALESCE(NULLIF(a.resource_name, ''), CASE a.resource_type
			WHEN 'host' THEN (SELECT name FROM hosts WHERE id = a.resource_id)
			WHEN 'host_group' THEN (SELECT name FROM host_groups WHERE id = a.resource_id)
			WHEN 'credential' THEN (SELECT name FROM credentials WHERE id = a.resource_id)
			WHEN 'tool' THEN (SELECT name FROM tools WHERE id = a.resource_id)
			WHEN 'flow' THEN (SELECT name FROM flows WHERE id = a.resource_id)
			WHEN 'execution' THEN (SELECT name_snapshot FROM executions WHERE id = a.resource_id)
			WHEN 'schedule' THEN (SELECT name FROM schedules WHERE id = a.resource_id)
			WHEN 'notify_channel' THEN (SELECT name FROM notify_channels WHERE id = a.resource_id)
			WHEN 'user' THEN (SELECT username FROM users WHERE id = a.resource_id)
			WHEN 'transfer' THEN (SELECT file_name FROM transfers WHERE id = a.resource_id)
		END, '') AS resource_name,
		a.summary_json, a.ip, a.user_agent
		FROM audit_logs a WHERE `+cond+` ORDER BY a.ts DESC, a.id DESC LIMIT ? OFFSET ?`, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list audit: %w", err)
	}
	return rows, total, nil
}
