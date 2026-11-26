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
	err := s.db.SelectContext(ctx, &rows, `SELECT id, ts, user_id, username_snapshot, action, resource_type,
		resource_id, resource_name, summary_json, ip, user_agent
		FROM audit_logs WHERE `+cond+` ORDER BY ts DESC, id DESC LIMIT ? OFFSET ?`, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list audit: %w", err)
	}
	return rows, total, nil
}
