package schedule

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/robfig/cron/v3"

	"github.com/Hopetree/spool/internal/credential"
	"github.com/Hopetree/spool/internal/exec"
	"github.com/Hopetree/spool/internal/flow"
	"github.com/Hopetree/spool/internal/notify"
	"github.com/Hopetree/spool/internal/store"
	"github.com/Hopetree/spool/internal/tool"
)

// Errors returned by the schedule service.
var ErrNotFound = errors.New("schedule not found")

// ValidationError marks invalid input.
type ValidationError struct{ Message string }

func (e *ValidationError) Error() string { return e.Message }

func invalid(msg string) error { return &ValidationError{Message: msg} }

// Schedule is the public view of a scheduled job.
type Schedule struct {
	ID                string            `json:"id"`
	Name              string            `json:"name"`
	TargetKind        string            `json:"target_kind"`
	TargetRefID       string            `json:"target_ref_id"`
	TargetName        string            `json:"target_name"`
	Params            map[string]string `json:"params"`
	CronExpr          string            `json:"cron_expr"`
	ConcurrencyPolicy string            `json:"concurrency_policy"`
	HostIDs           []string          `json:"host_ids"`
	NotifyChannelIDs  []string          `json:"notify_channel_ids"`
	NotifyEvents      string            `json:"notify_events"`
	Enabled           bool              `json:"enabled"`
	LastRunAt         *int64            `json:"last_run_at"`
	NextRunAt         *int64            `json:"next_run_at"`
	CreatedAt         int64             `json:"created_at"`
	UpdatedAt         int64             `json:"updated_at"`
}

// Input carries create/update fields.
type Input struct {
	Name              string
	TargetKind        string
	TargetRefID       string
	Params            map[string]string
	HostIDs           []string
	CronExpr          string
	ConcurrencyPolicy string
	NotifyChannelIDs  []string
	NotifyEvents      string
	Enabled           bool
}

// ListFilter narrows schedule list queries.
type ListFilter struct {
	Keyword  string
	Enabled  *bool
	Page     int
	PageSize int
}

type row struct {
	ID                   string         `db:"id"`
	Name                 string         `db:"name"`
	TargetKind           string         `db:"target_kind"`
	TargetRefID          string         `db:"target_ref_id"`
	TargetName           sql.NullString `db:"target_name"`
	ParamsJSON           string         `db:"params_json"`
	SensitiveEnc         []byte         `db:"sensitive_params_enc"`
	HostIDsJSON          string         `db:"host_ids_json"`
	CronExpr             string         `db:"cron_expr"`
	ConcurrencyPolicy    string         `db:"concurrency_policy"`
	NotifyChannelIDsJSON string         `db:"notify_channel_ids_json"`
	NotifyEvents         string         `db:"notify_events"`
	Enabled              bool           `db:"enabled"`
	CreatedBy            string         `db:"created_by"`
	LastRunAt            *int64         `db:"last_run_at"`
	NextRunAt            *int64         `db:"next_run_at"`
	CreatedAt            int64          `db:"created_at"`
	UpdatedAt            int64          `db:"updated_at"`
}

func (r row) toSchedule() Schedule {
	params := map[string]string{}
	_ = json.Unmarshal([]byte(r.ParamsJSON), &params)
	hostIDs := []string{}
	_ = json.Unmarshal([]byte(r.HostIDsJSON), &hostIDs)
	channelIDs := []string{}
	_ = json.Unmarshal([]byte(r.NotifyChannelIDsJSON), &channelIDs)
	return Schedule{
		ID: r.ID, Name: r.Name, TargetKind: r.TargetKind, TargetRefID: r.TargetRefID,
		TargetName: r.TargetName.String, Params: params, CronExpr: r.CronExpr,
		ConcurrencyPolicy: r.ConcurrencyPolicy, HostIDs: hostIDs,
		NotifyChannelIDs: channelIDs, NotifyEvents: r.NotifyEvents, Enabled: r.Enabled,
		LastRunAt: r.LastRunAt, NextRunAt: r.NextRunAt, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
}

const selectSQL = `SELECT s.*, COALESCE(t.name, f.name, '') AS target_name
	FROM schedules s
	LEFT JOIN tools t ON s.target_kind = 'tool' AND t.id = s.target_ref_id
	LEFT JOIN flows f ON s.target_kind = 'flow' AND f.id = s.target_ref_id`

// Service manages schedules and runs the cron scheduler.
type Service struct {
	db       *sqlx.DB
	tools    *tool.Service
	flows    *flow.Service
	creds    *credential.Service
	engine   *exec.Engine
	notifier *notify.Service
	loc      *time.Location
	parser   cron.Parser
	cron     *cron.Cron

	mu      sync.Mutex
	entries map[string]cron.EntryID
}

// NewService builds a schedule Service.
func NewService(db *sqlx.DB, tools *tool.Service, flows *flow.Service, creds *credential.Service,
	engine *exec.Engine, notifier *notify.Service, loc *time.Location) *Service {
	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)
	return &Service{
		db: db, tools: tools, flows: flows, creds: creds, engine: engine,
		notifier: notifier, loc: loc, parser: parser,
		cron:    cron.New(cron.WithLocation(loc), cron.WithParser(parser), cron.WithChain(cron.Recover(cron.DefaultLogger))),
		entries: make(map[string]cron.EntryID),
	}
}

func (s *Service) nowMs() int64 { return time.Now().UnixMilli() }

// Start loads enabled schedules and starts the cron loop.
func (s *Service) Start(ctx context.Context) error {
	rows := []row{}
	if err := s.db.SelectContext(ctx, &rows, selectSQL+` WHERE s.enabled = 1`); err != nil {
		return fmt.Errorf("load schedules: %w", err)
	}
	s.cron.Start()
	for _, r := range rows {
		sch := r.toSchedule()
		if err := s.addJob(sch); err != nil {
			slog.Error("schedule: add job failed", "id", sch.ID, "err", err)
		}
	}
	return nil
}

// Stop stops the cron loop and waits for running jobs.
func (s *Service) Stop() {
	<-s.cron.Stop().Done()
}

func (s *Service) addJob(sch Schedule) error {
	id, err := s.cron.AddFunc(sch.CronExpr, func() { s.fire(sch.ID) })
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.entries[sch.ID] = id
	s.mu.Unlock()
	parsed, err := s.parser.Parse(sch.CronExpr)
	if err != nil {
		return err
	}
	next := parsed.Next(time.Now().In(s.loc))
	if next.IsZero() {
		_, _ = s.db.Exec(`UPDATE schedules SET next_run_at = NULL WHERE id = ?`, sch.ID)
		return nil
	}
	_, _ = s.db.Exec(`UPDATE schedules SET next_run_at = ? WHERE id = ?`, next.UnixMilli(), sch.ID)
	return nil
}

func (s *Service) removeJob(id string) {
	s.mu.Lock()
	entryID, ok := s.entries[id]
	if ok {
		delete(s.entries, id)
	}
	s.mu.Unlock()
	if ok {
		s.cron.Remove(entryID)
	}
}

func (s *Service) syncJob(ctx context.Context, id string) error {
	sch, err := s.Get(ctx, id)
	if err != nil {
		return err
	}
	s.removeJob(id)
	if !sch.Enabled {
		_, _ = s.db.Exec(`UPDATE schedules SET next_run_at = NULL WHERE id = ?`, id)
		return nil
	}
	return s.addJob(*sch)
}

// List returns a page of schedules.
func (s *Service) List(ctx context.Context, f ListFilter) ([]Schedule, int64, error) {
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
		where = append(where, "s.name LIKE ?")
		args = append(args, "%"+kw+"%")
	}
	if f.Enabled != nil {
		where = append(where, "s.enabled = ?")
		args = append(args, *f.Enabled)
	}
	cond := strings.Join(where, " AND ")
	var total int64
	if err := s.db.GetContext(ctx, &total, `SELECT count(*) FROM schedules s WHERE `+cond, args...); err != nil {
		return nil, 0, fmt.Errorf("count schedules: %w", err)
	}
	pageArgs := append([]any{}, args...)
	pageArgs = append(pageArgs, f.PageSize, (f.Page-1)*f.PageSize)
	rows := []row{}
	if err := s.db.SelectContext(ctx, &rows, selectSQL+` WHERE `+cond+` ORDER BY s.name LIMIT ? OFFSET ?`, pageArgs...); err != nil {
		return nil, 0, fmt.Errorf("list schedules: %w", err)
	}
	out := make([]Schedule, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.toSchedule())
	}
	return out, total, nil
}

// Get returns one schedule.
func (s *Service) Get(ctx context.Context, id string) (*Schedule, error) {
	var r row
	err := s.db.GetContext(ctx, &r, selectSQL+` WHERE s.id = ?`, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get schedule: %w", err)
	}
	sch := r.toSchedule()
	return &sch, nil
}

func (s *Service) decryptSensitive(sch *Schedule) (map[string]string, error) {
	out := map[string]string{}
	var r row
	if err := s.db.GetContext(context.Background(), &r, `SELECT sensitive_params_enc FROM schedules WHERE id = ?`, sch.ID); err != nil {
		return out, err
	}
	if len(r.SensitiveEnc) == 0 {
		return out, nil
	}
	b, err := s.creds.DecryptSensitive(r.SensitiveEnc, sch.ID)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Service) declarations(ctx context.Context, kind, refID string) ([]tool.Param, string, bool, error) {
	switch kind {
	case "tool":
		d, err := s.tools.Get(ctx, refID)
		if err != nil {
			return nil, "", false, err
		}
		return d.Version.Params, d.Name, d.Enabled, nil
	case "flow":
		d, err := s.flows.Get(ctx, refID)
		if err != nil {
			return nil, "", false, err
		}
		return d.Params, d.Name, d.Enabled, nil
	default:
		return nil, "", false, invalid("目标类型必须是 tool 或 flow")
	}
}

func dedupe(ids []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out
}

func (s *Service) validate(ctx context.Context, in *Input, existingSensitive map[string]string) (map[string]string, map[string]string, string, error) {
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || len(in.Name) > 64 {
		return nil, nil, "", invalid("任务名称不能为空且不能超过 64 个字符")
	}
	if in.TargetKind != "tool" && in.TargetKind != "flow" {
		return nil, nil, "", invalid("目标类型必须是 tool 或 flow")
	}
	decls, targetName, enabled, err := s.declarations(ctx, in.TargetKind, in.TargetRefID)
	if errors.Is(err, tool.ErrNotFound) || errors.Is(err, flow.ErrNotFound) {
		return nil, nil, "", invalid("执行目标不存在")
	}
	if err != nil {
		return nil, nil, "", err
	}
	if !enabled {
		return nil, nil, "", invalid(fmt.Sprintf("执行目标 %q 已停用", targetName))
	}
	if _, err := s.parser.Parse(in.CronExpr); err != nil {
		return nil, nil, "", invalid("cron 表达式不合法：" + err.Error())
	}
	if in.ConcurrencyPolicy == "" {
		in.ConcurrencyPolicy = "skip"
	}
	if in.ConcurrencyPolicy != "skip" && in.ConcurrencyPolicy != "queue" {
		return nil, nil, "", invalid("并发策略必须是 skip 或 queue")
	}
	if in.NotifyEvents == "" {
		in.NotifyEvents = "failure"
	}
	if in.NotifyEvents != "failure" && in.NotifyEvents != "always" {
		return nil, nil, "", invalid("通知时机必须是 failure 或 always")
	}
	in.HostIDs = dedupe(in.HostIDs)
	needsHosts := in.TargetKind == "tool"
	if needsHosts && len(in.HostIDs) == 0 {
		return nil, nil, "", invalid("请选择至少一台默认主机")
	}
	if len(in.HostIDs) > 0 {
		q, args, err := sqlx.In(`SELECT count(*) FROM hosts WHERE id IN (?)`, in.HostIDs)
		if err != nil {
			return nil, nil, "", err
		}
		var hosts int
		if err := s.db.GetContext(ctx, &hosts, s.db.Rebind(q), args...); err != nil {
			return nil, nil, "", err
		}
		if hosts != len(in.HostIDs) {
			return nil, nil, "", invalid("包含不存在的主机")
		}
	}
	in.NotifyChannelIDs = dedupe(in.NotifyChannelIDs)
	if len(in.NotifyChannelIDs) > 0 {
		q, args, err := sqlx.In(`SELECT count(*) FROM notify_channels WHERE id IN (?)`, in.NotifyChannelIDs)
		if err != nil {
			return nil, nil, "", err
		}
		var channels int
		if err := s.db.GetContext(ctx, &channels, s.db.Rebind(q), args...); err != nil {
			return nil, nil, "", err
		}
		if channels != len(in.NotifyChannelIDs) {
			return nil, nil, "", invalid("包含不存在的通知渠道")
		}
	}

	merged := map[string]string{}
	if in.Params == nil {
		in.Params = map[string]string{}
	}
	for k, v := range in.Params {
		merged[k] = v
	}
	for k, v := range existingSensitive {
		if merged[k] == "" {
			merged[k] = v
		}
	}
	if in.TargetKind == "flow" {
		// Flow inputs are derived from the current graph; ignore values left
		// over from parameters that no longer exist.
		filtered := make(map[string]string, len(decls))
		for _, p := range decls {
			if v, ok := merged[p.Name]; ok {
				filtered[p.Name] = v
			}
		}
		merged = filtered
	}
	values, err := tool.ValidateValues(decls, merged)
	if err != nil {
		return nil, nil, "", invalid(err.Error())
	}
	plain := map[string]string{}
	sensitive := map[string]string{}
	for _, p := range decls {
		if p.Sensitive {
			sensitive[p.Name] = values[p.Name]
			continue
		}
		plain[p.Name] = values[p.Name]
	}
	return plain, sensitive, targetName, nil
}

func marshal(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

// Create inserts a schedule and registers its cron job.
func (s *Service) Create(ctx context.Context, in Input, actorID string) (*Schedule, error) {
	plain, sensitive, _, err := s.validate(ctx, &in, nil)
	if err != nil {
		return nil, err
	}
	id := store.NewULID()
	var sensitiveEnc []byte
	if len(sensitive) > 0 {
		sensitiveEnc, err = s.creds.EncryptSensitive([]byte(marshal(sensitive)), id)
		if err != nil {
			return nil, err
		}
	}
	now := s.nowMs()
	_, err = s.db.ExecContext(ctx, `INSERT INTO schedules
		(id, name, target_kind, target_ref_id, params_json, sensitive_params_enc, host_ids_json, cron_expr,
		 concurrency_policy, notify_channel_ids_json, notify_events, enabled, created_by, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, in.Name, in.TargetKind, in.TargetRefID, marshal(plain), sensitiveEnc, marshal(in.HostIDs),
		in.CronExpr, in.ConcurrencyPolicy, marshal(in.NotifyChannelIDs), in.NotifyEvents, in.Enabled,
		actorID, now, now)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return nil, invalid("任务名称已存在")
		}
		return nil, fmt.Errorf("insert schedule: %w", err)
	}
	if err := s.syncJob(ctx, id); err != nil {
		return nil, fmt.Errorf("register job: %w", err)
	}
	return s.Get(ctx, id)
}

// Update replaces a schedule and re-registers its job.
func (s *Service) Update(ctx context.Context, id string, in Input) (*Schedule, error) {
	existing, err := s.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	oldSensitive, err := s.decryptSensitive(existing)
	if err != nil {
		return nil, err
	}
	plain, sensitive, _, err := s.validate(ctx, &in, oldSensitive)
	if err != nil {
		return nil, err
	}
	var sensitiveEnc []byte
	if len(sensitive) > 0 {
		sensitiveEnc, err = s.creds.EncryptSensitive([]byte(marshal(sensitive)), id)
		if err != nil {
			return nil, err
		}
	}
	_, err = s.db.ExecContext(ctx, `UPDATE schedules SET name = ?, target_kind = ?, target_ref_id = ?,
		params_json = ?, sensitive_params_enc = ?, host_ids_json = ?, cron_expr = ?, concurrency_policy = ?,
		notify_channel_ids_json = ?, notify_events = ?, enabled = ?, updated_at = ? WHERE id = ?`,
		in.Name, in.TargetKind, in.TargetRefID, marshal(plain), sensitiveEnc, marshal(in.HostIDs),
		in.CronExpr, in.ConcurrencyPolicy, marshal(in.NotifyChannelIDs), in.NotifyEvents, in.Enabled, s.nowMs(), id)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return nil, invalid("任务名称已存在")
		}
		return nil, fmt.Errorf("update schedule: %w", err)
	}
	if err := s.syncJob(ctx, id); err != nil {
		return nil, fmt.Errorf("register job: %w", err)
	}
	return s.Get(ctx, id)
}

// SetEnabled toggles a schedule.
func (s *Service) SetEnabled(ctx context.Context, id string, enabled bool) error {
	res, err := s.db.ExecContext(ctx, `UPDATE schedules SET enabled = ?, updated_at = ? WHERE id = ?`, enabled, s.nowMs(), id)
	if err != nil {
		return fmt.Errorf("set enabled: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return s.syncJob(ctx, id)
}

// Delete removes a schedule and its cron job.
func (s *Service) Delete(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM schedules WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete schedule: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	s.removeJob(id)
	return nil
}

// NextRuns previews the next triggers.
func (s *Service) NextRuns(ctx context.Context, id string, count int) ([]int64, error) {
	sch, err := s.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	return s.Preview(sch.CronExpr, count)
}

// Preview computes the next count trigger times for a cron expression.
func (s *Service) Preview(expr string, count int) ([]int64, error) {
	if count < 1 || count > 20 {
		count = 5
	}
	parsed, err := s.parser.Parse(expr)
	if err != nil {
		return nil, invalid("cron 表达式不合法：" + err.Error())
	}
	out := make([]int64, 0, count)
	next := time.Now().In(s.loc)
	for i := 0; i < count; i++ {
		next = parsed.Next(next)
		if next.IsZero() {
			break
		}
		out = append(out, next.UnixMilli())
	}
	return out, nil
}

func (s *Service) submit(ctx context.Context, sch *Schedule, source, actorID string) (*exec.Execution, error) {
	sensitiveOld, err := s.decryptSensitive(sch)
	if err != nil {
		return nil, err
	}
	values := map[string]string{}
	for k, v := range sch.Params {
		values[k] = v
	}
	for k, v := range sensitiveOld {
		values[k] = v
	}

	var (
		plan      exec.Plan
		plain     map[string]string
		sensitive map[string]string
		name      string
	)
	switch sch.TargetKind {
	case "tool":
		detail, err := s.tools.Get(ctx, sch.TargetRefID)
		if err != nil {
			return nil, fmt.Errorf("工具不可用: %w", err)
		}
		if !detail.Enabled {
			return nil, invalid(fmt.Sprintf("工具 %q 已停用", detail.Name))
		}
		validated, err := tool.ValidateValues(detail.Version.Params, values)
		if err != nil {
			return nil, err
		}
		plain = map[string]string{}
		sensitive = map[string]string{}
		for _, p := range detail.Version.Params {
			if p.Sensitive {
				sensitive[p.Name] = validated[p.Name]
				continue
			}
			plain[p.Name] = validated[p.Name]
		}
		refs := make(map[string]string, len(sensitive))
		for k := range sensitive {
			refs[k] = k
		}
		plan = exec.BuildToolPlan(detail.Name, exec.ToolVersionSnapshot{
			ToolID: detail.ID, VersionID: detail.Version.ID, VersionNo: detail.Version.VersionNo,
			ContentHash: detail.Version.ContentHash, Type: detail.Version.Type,
			Interpreter: detail.Version.Interpreter, Content: detail.Version.Content,
			PrepareCommand: detail.Version.PrepareCommand, Sudo: detail.Version.Sudo,
			TimeoutSec: detail.Version.TimeoutSec,
		}, plain, refs)
		name = detail.Name
	case "flow":
		res, err := s.flows.Compile(ctx, sch.TargetRefID, values)
		if err != nil {
			return nil, err
		}
		plan, plain, sensitive = res.Plan, res.Plain, res.Sensitive
		name = plan.Name
	default:
		return nil, invalid("目标类型必须是 tool 或 flow")
	}
	ex, err := s.engine.Submit(ctx, exec.SubmitRequest{
		Kind: sch.TargetKind, Source: source, RefID: sch.TargetRefID, ScheduleID: sch.ID,
		Name: name, Plan: plan, Params: plain, Sensitive: sensitive,
		HostIDs: sch.HostIDs, ActorID: actorID,
	})
	if err != nil {
		return nil, err
	}
	return ex, nil
}

// RunNow triggers a schedule immediately (no skip check).
func (s *Service) RunNow(ctx context.Context, id, actorID string) (*exec.Execution, error) {
	sch, err := s.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	return s.submit(ctx, sch, "manual", actorID)
}

func (s *Service) running(ctx context.Context, scheduleID string) bool {
	var n int
	err := s.db.GetContext(ctx, &n, `SELECT count(*) FROM executions WHERE schedule_id = ? AND status IN ('queued','running')`, scheduleID)
	return err == nil && n > 0
}

func (s *Service) recordNoRun(ctx context.Context, sch *Schedule, status, summary string) {
	id := store.NewULID()
	now := s.nowMs()
	_, err := s.db.ExecContext(ctx, `INSERT INTO executions
		(id, kind, source, ref_id, schedule_id, name_snapshot, plan_json, host_total, status, error_summary, created_by, created_at, finished_at)
		VALUES (?, ?, 'schedule', ?, ?, ?, '{}', 0, ?, ?, 'scheduler', ?, ?)`,
		id, sch.TargetKind, sch.TargetRefID, sch.ID, sch.TargetName, status, summary, now, now)
	if err != nil {
		slog.Error("schedule: record no-run failed", "id", sch.ID, "err", err)
		return
	}
	if s.notifier != nil {
		s.notifier.NotifyExecution(ctx, notify.ExecutionNotice{
			ID: id, Name: sch.TargetName, Status: status, ScheduleID: sch.ID,
			ErrorSummary: summary, FinishedAt: &now,
		})
	}
}

func (s *Service) fire(id string) {
	log := slog.With("schedule", id)
	defer func() {
		if r := recover(); r != nil {
			log.Error("schedule: panic", "panic", r)
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	sch, err := s.Get(ctx, id)
	if err != nil || !sch.Enabled {
		return
	}
	if sch.ConcurrencyPolicy == "skip" && s.running(ctx, id) {
		s.recordNoRun(ctx, sch, "skipped", "上一次执行仍在进行，本次跳过")
		log.Info("skipped: previous run still active")
		return
	}
	ex, err := s.submit(ctx, sch, "schedule", "scheduler")
	if err != nil {
		s.recordNoRun(ctx, sch, "failed", "触发失败："+err.Error())
		log.Error("trigger failed", "err", err)
		return
	}
	_, _ = s.db.ExecContext(ctx, `UPDATE schedules SET last_run_at = ? WHERE id = ?`, s.nowMs(), id)
	log.Info("triggered", "execution", ex.ID)
}

// HandleExecutionFinished notifies channels for scheduled executions.
func (s *Service) HandleExecutionFinished(ctx context.Context, execID string) {
	ex, err := s.engine.GetExecution(ctx, execID)
	if err != nil || ex.ScheduleID == "" || s.notifier == nil {
		return
	}
	s.notifier.NotifyExecution(ctx, notify.ExecutionNotice{
		ID: ex.ID, Name: ex.NameSnapshot, Status: ex.Status, FailureReason: ex.FailureReason,
		ErrorSummary: ex.ErrorSummary, ScheduleID: ex.ScheduleID,
		HostTotal: ex.HostTotal, HostSuccess: ex.HostSuccess, HostFailed: ex.HostFailed,
		HostCancelled: ex.HostCancelled, StartedAt: ex.StartedAt, FinishedAt: ex.FinishedAt,
	})
}
