package exec

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/Hopetree/spool/internal/credential"
	"github.com/Hopetree/spool/internal/store"
)

// Engine errors.
var (
	ErrNotFound = errors.New("execution not found")
	ErrNoHosts  = errors.New("no enabled hosts selected")
)

// Execution is the aggregate view of one submitted run.
type Execution struct {
	ID                 string `db:"id" json:"id"`
	Kind               string `db:"kind" json:"kind"`
	Source             string `db:"source" json:"source"`
	RefID              string `db:"ref_id" json:"ref_id"`
	ScheduleID         string `db:"schedule_id" json:"schedule_id"`
	NameSnapshot       string `db:"name_snapshot" json:"name"`
	ParamsJSON         string `db:"params_json" json:"-"`
	SensitiveParamsEnc []byte `db:"sensitive_params_enc" json:"-"`
	PlanJSON           string `db:"plan_json" json:"-"`
	HostTotal          int    `db:"host_total" json:"host_total"`
	HostSuccess        int    `db:"host_success" json:"host_success"`
	HostFailed         int    `db:"host_failed" json:"host_failed"`
	HostCancelled      int    `db:"host_cancelled" json:"host_cancelled"`
	Status             string `db:"status" json:"status"`
	FailureReason      string `db:"failure_reason" json:"failure_reason"`
	ErrorSummary       string `db:"error_summary" json:"error_summary"`
	CancelRequested    bool   `db:"cancel_requested" json:"cancel_requested"`
	CreatedBy          string `db:"created_by" json:"created_by"`
	CreatedAt          int64  `db:"created_at" json:"created_at"`
	StartedAt          *int64 `db:"started_at" json:"started_at"`
	FinishedAt         *int64 `db:"finished_at" json:"finished_at"`
}

// HostRun is one host's progress within an execution.
type HostRun struct {
	ID            string `db:"id" json:"id"`
	ExecutionID   string `db:"execution_id" json:"execution_id"`
	HostID        string `db:"host_id" json:"host_id"`
	HostName      string `db:"host_name_snapshot" json:"host_name"`
	Status        string `db:"status" json:"status"`
	FailureReason string `db:"failure_reason" json:"failure_reason"`
	ErrorMessage  string `db:"error_message" json:"error_message"`
	StartedAt     *int64 `db:"started_at" json:"started_at"`
	FinishedAt    *int64 `db:"finished_at" json:"finished_at"`
}

// StepRun is one script execution inside a host run.
type StepRun struct {
	ID            string `db:"id" json:"id"`
	HostRunID     string `db:"host_run_id" json:"host_run_id"`
	StageIdx      int    `db:"stage_idx" json:"stage_idx"`
	StepIdx       int    `db:"step_idx" json:"step_idx"`
	Name          string `db:"name_snapshot" json:"name"`
	ToolID        string `db:"tool_id" json:"tool_id"`
	ToolVersionID string `db:"tool_version_id" json:"tool_version_id"`
	ToolVersionNo int    `db:"tool_version_no" json:"tool_version_no"`
	ContentHash   string `db:"content_hash" json:"content_hash"`
	Interpreter   string `db:"interpreter" json:"interpreter"`
	Sudo          bool   `db:"sudo" json:"sudo"`
	Status        string `db:"status" json:"status"`
	FailureReason string `db:"failure_reason" json:"failure_reason"`
	ExitCode      *int   `db:"exit_code" json:"exit_code"`
	ErrorMessage  string `db:"error_message" json:"error_message"`
	Attempt       int    `db:"attempt" json:"attempt"`
	StartedAt     *int64 `db:"started_at" json:"started_at"`
	FinishedAt    *int64 `db:"finished_at" json:"finished_at"`
	LogEvents     int    `db:"log_events" json:"log_events"`
	LogBytes      int64  `db:"log_bytes" json:"log_bytes"`
	Truncated     bool   `db:"truncated" json:"truncated"`
	DroppedBytes  int64  `db:"dropped_bytes" json:"dropped_bytes"`
}

// Config tunes the execution engine.
type Config struct {
	MaxConcurrentSteps    int
	MaxConcurrentHostRuns int
	DefaultStepTimeoutSec int
	OutputLimitBytes      int64
	RemoteBaseDir         string
	LogsDir               string
}

func (c Config) withDefaults() Config {
	if c.MaxConcurrentSteps <= 0 {
		c.MaxConcurrentSteps = 20
	}
	if c.MaxConcurrentHostRuns <= 0 {
		c.MaxConcurrentHostRuns = 50
	}
	if c.DefaultStepTimeoutSec <= 0 {
		c.DefaultStepTimeoutSec = 300
	}
	if c.OutputLimitBytes <= 0 {
		c.OutputLimitBytes = 20 * 1024 * 1024
	}
	if c.RemoteBaseDir == "" {
		c.RemoteBaseDir = "/tmp"
	}
	return c
}

// SubmitRequest carries a resolved execution.
type SubmitRequest struct {
	Kind       string
	Source     string
	RefID      string
	ScheduleID string
	Name       string
	Plan       Plan
	Params     map[string]string
	Sensitive  map[string]string
	HostIDs    []string
	ActorID    string
}

type capability struct {
	BashPath   string `json:"bash_path"`
	PythonPath string `json:"python_path"`
	Setsid     bool   `json:"setsid"`
	Timeout    bool   `json:"timeout"`
	ProbedAt   int64  `json:"probed_at"`
}

// Engine runs plans on hosts.
type Engine struct {
	db      *sqlx.DB
	creds   *credential.Service
	factory RunnerFactory
	cfg     Config
	hub     *Hub

	stepSem chan struct{}
	hostSem chan struct{}

	hostLocks sync.Map

	baseCtx   context.Context
	cancelAll context.CancelFunc
	wg        sync.WaitGroup

	mu          sync.Mutex
	execCancels map[string]context.CancelFunc
	hostCancels map[string]context.CancelFunc
}

// New builds an Engine.
func New(db *sqlx.DB, creds *credential.Service, factory RunnerFactory, cfg Config) *Engine {
	cfg = cfg.withDefaults()
	baseCtx, cancelAll := context.WithCancel(context.Background())
	return &Engine{
		db: db, creds: creds, factory: factory, cfg: cfg, hub: NewHub(),
		stepSem: make(chan struct{}, cfg.MaxConcurrentSteps),
		hostSem: make(chan struct{}, cfg.MaxConcurrentHostRuns),
		baseCtx: baseCtx, cancelAll: cancelAll,
		execCancels: make(map[string]context.CancelFunc),
		hostCancels: make(map[string]context.CancelFunc),
	}
}

// Hub exposes the SSE event hub.
func (e *Engine) Hub() *Hub { return e.hub }

// LogPath returns the JSONL log path for an execution.
func (e *Engine) LogPath(execID string) string {
	return filepath.Join(e.cfg.LogsDir, execID+".jsonl")
}

// Start marks unfinished runs as interrupted, then enables execution.
func (e *Engine) Start(ctx context.Context) error {
	now := time.Now().UnixMilli()
	if _, err := e.db.ExecContext(ctx, `UPDATE step_runs SET status='interrupted', error_message='service restarted' WHERE status IN ('queued','running')`); err != nil {
		return fmt.Errorf("recover steps: %w", err)
	}
	if _, err := e.db.ExecContext(ctx, `UPDATE host_runs SET status='interrupted', error_message='service restarted', finished_at=? WHERE status IN ('queued','running')`, now); err != nil {
		return fmt.Errorf("recover host runs: %w", err)
	}
	if _, err := e.db.ExecContext(ctx, `UPDATE executions SET status='interrupted', error_summary='service restarted', finished_at=? WHERE status IN ('queued','running')`, now); err != nil {
		return fmt.Errorf("recover executions: %w", err)
	}
	return nil
}

// Stop cancels all running work and waits for goroutines.
func (e *Engine) Stop() {
	e.cancelAll()
	e.wg.Wait()
}

func (e *Engine) cancelStatus() string {
	if e.baseCtx.Err() != nil {
		return "interrupted"
	}
	return "cancelled"
}

func (e *Engine) hostLock(hostID string) *sync.Mutex {
	v, _ := e.hostLocks.LoadOrStore(hostID, &sync.Mutex{})
	return v.(*sync.Mutex)
}

func (e *Engine) nowMs() int64 { return time.Now().UnixMilli() }

// GetExecution loads one execution.
func (e *Engine) GetExecution(ctx context.Context, id string) (*Execution, error) {
	var ex Execution
	err := e.db.GetContext(ctx, &ex, `SELECT * FROM executions WHERE id = ?`, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get execution: %w", err)
	}
	return &ex, nil
}

// HostRuns returns host runs of an execution.
func (e *Engine) HostRuns(ctx context.Context, execID string) ([]HostRun, error) {
	out := []HostRun{}
	err := e.db.SelectContext(ctx, &out, `SELECT * FROM host_runs WHERE execution_id = ? ORDER BY host_name_snapshot`, execID)
	if err != nil {
		return nil, fmt.Errorf("host runs: %w", err)
	}
	return out, nil
}

// StepRuns returns all step runs of an execution.
func (e *Engine) StepRuns(ctx context.Context, execID string) ([]StepRun, error) {
	out := []StepRun{}
	err := e.db.SelectContext(ctx, &out, `SELECT s.* FROM step_runs s
		JOIN host_runs h ON h.id = s.host_run_id WHERE h.execution_id = ?
		ORDER BY h.host_name_snapshot, s.stage_idx, s.step_idx`, execID)
	if err != nil {
		return nil, fmt.Errorf("step runs: %w", err)
	}
	return out, nil
}

// DecryptSensitive returns the execution's sensitive parameters.
func (e *Engine) DecryptSensitive(ex *Execution) (map[string]string, error) {
	out := map[string]string{}
	if len(ex.SensitiveParamsEnc) == 0 {
		return out, nil
	}
	b, err := e.creds.DecryptSensitive(ex.SensitiveParamsEnc, ex.ID)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, err
	}
	return out, nil
}

type hostTarget struct {
	ref HostRef
	cap capability
}

func (e *Engine) loadHost(ctx context.Context, id string) (hostTarget, error) {
	var row struct {
		ID              string `db:"id"`
		Name            string `db:"name"`
		Address         string `db:"address"`
		Port            int    `db:"port"`
		CredentialID    string `db:"credential_id"`
		Fingerprint     string `db:"fingerprint"`
		FingerprintType string `db:"fingerprint_type"`
		CapabilityJSON  string `db:"capability_json"`
		Enabled         bool   `db:"enabled"`
	}
	err := e.db.GetContext(ctx, &row, `SELECT id, name, address, port, credential_id, fingerprint,
		fingerprint_type, capability_json, enabled FROM hosts WHERE id = ?`, id)
	if errors.Is(err, sql.ErrNoRows) {
		return hostTarget{}, errors.New("host no longer exists")
	}
	if err != nil {
		return hostTarget{}, err
	}
	if !row.Enabled {
		return hostTarget{}, errors.New("host is disabled")
	}
	var cap capability
	if row.CapabilityJSON != "" {
		_ = json.Unmarshal([]byte(row.CapabilityJSON), &cap)
	}
	return hostTarget{
		ref: HostRef{ID: row.ID, Name: row.Name, Address: row.Address, Port: row.Port,
			CredentialID: row.CredentialID, Fingerprint: row.Fingerprint, FingerprintType: row.FingerprintType},
		cap: cap,
	}, nil
}

type hostRow struct {
	ID              string `db:"id"`
	Name            string `db:"name"`
	Address         string `db:"address"`
	Port            int    `db:"port"`
	CredentialID    string `db:"credential_id"`
	Fingerprint     string `db:"fingerprint"`
	FingerprintType string `db:"fingerprint_type"`
	Enabled         bool   `db:"enabled"`
}

// Submit persists an execution and starts it asynchronously.
func (e *Engine) Submit(ctx context.Context, req SubmitRequest) (*Execution, error) {
	hosts := []hostRow{}
	if len(req.HostIDs) > 0 {
		q, args, err := sqlx.In(`SELECT id, name, address, port, credential_id, fingerprint, fingerprint_type, enabled
			FROM hosts WHERE id IN (?) AND enabled = 1`, req.HostIDs)
		if err != nil {
			return nil, err
		}
		if err := e.db.SelectContext(ctx, &hosts, e.db.Rebind(q), args...); err != nil {
			return nil, fmt.Errorf("load hosts: %w", err)
		}
	}
	byID := make(map[string]hostRow, len(hosts))
	for _, h := range hosts {
		byID[h.ID] = h
	}
	ordered := make([]hostRow, 0, len(req.HostIDs))
	seen := make(map[string]bool, len(req.HostIDs))
	for _, id := range req.HostIDs {
		if seen[id] {
			continue
		}
		seen[id] = true
		if h, ok := byID[id]; ok {
			ordered = append(ordered, h)
		}
	}
	if len(ordered) == 0 {
		return nil, ErrNoHosts
	}

	execID := store.NewULID()
	var sensitiveEnc []byte
	if len(req.Sensitive) > 0 {
		b, err := json.Marshal(req.Sensitive)
		if err != nil {
			return nil, err
		}
		sensitiveEnc, err = e.creds.EncryptSensitive(b, execID)
		if err != nil {
			return nil, err
		}
	}
	paramsJSON, _ := json.Marshal(req.Params)
	planJSON, err := json.Marshal(req.Plan)
	if err != nil {
		return nil, fmt.Errorf("marshal plan: %w", err)
	}
	now := e.nowMs()

	tx, err := e.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	_, err = tx.ExecContext(ctx, `INSERT INTO executions
		(id, kind, source, ref_id, schedule_id, name_snapshot, params_json, sensitive_params_enc, plan_json,
		 host_total, status, created_by, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'queued', ?, ?)`,
		execID, req.Kind, req.Source, req.RefID, req.ScheduleID, req.Name, string(paramsJSON), sensitiveEnc, string(planJSON),
		len(ordered), req.ActorID, now)
	if err != nil {
		return nil, fmt.Errorf("insert execution: %w", err)
	}
	for _, h := range ordered {
		hostRunID := store.NewULID()
		_, err = tx.ExecContext(ctx, `INSERT INTO host_runs (id, execution_id, host_id, host_name_snapshot, status)
			VALUES (?, ?, ?, ?, 'queued')`, hostRunID, execID, h.ID, h.Name)
		if err != nil {
			return nil, fmt.Errorf("insert host run: %w", err)
		}
		for _, stage := range req.Plan.Stages {
			for _, st := range stage.Steps {
				_, err = tx.ExecContext(ctx, `INSERT INTO step_runs
					(id, host_run_id, stage_idx, step_idx, name_snapshot, tool_id, tool_version_id, tool_version_no,
					 content_hash, interpreter, sudo, status)
					VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'queued')`,
					store.NewULID(), hostRunID, stage.Index, st.Index, st.Name, st.ToolID, st.ToolVersionID, st.ToolVersionNo,
					st.ContentHash, st.Interpreter, st.Sudo)
				if err != nil {
					return nil, fmt.Errorf("insert step run: %w", err)
				}
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit execution: %w", err)
	}

	e.wg.Add(1)
	go func() {
		defer e.wg.Done()
		e.runExecution(execID)
	}()
	return e.GetExecution(ctx, execID)
}

// Cancel requests cancellation of a whole execution.
func (e *Engine) Cancel(ctx context.Context, execID string) error {
	if _, err := e.GetExecution(ctx, execID); err != nil {
		return err
	}
	if _, err := e.db.ExecContext(ctx, `UPDATE executions SET cancel_requested = 1 WHERE id = ?`, execID); err != nil {
		return err
	}
	e.mu.Lock()
	if cancel := e.execCancels[execID]; cancel != nil {
		cancel()
	}
	e.mu.Unlock()
	return nil
}

// CancelHostRun requests cancellation of one host run.
func (e *Engine) CancelHostRun(ctx context.Context, hostRunID string) error {
	var id string
	err := e.db.GetContext(ctx, &id, `SELECT id FROM host_runs WHERE id = ?`, hostRunID)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	e.mu.Lock()
	if cancel := e.hostCancels[hostRunID]; cancel != nil {
		cancel()
	}
	e.mu.Unlock()
	return nil
}

func (e *Engine) runExecution(execID string) {
	ctx, cancel := context.WithCancel(e.baseCtx)
	e.mu.Lock()
	e.execCancels[execID] = cancel
	e.mu.Unlock()
	defer func() {
		cancel()
		e.mu.Lock()
		delete(e.execCancels, execID)
		e.mu.Unlock()
	}()

	logPath := filepath.Join(e.cfg.LogsDir, execID+".jsonl")
	log, err := OpenEventLog(logPath, execID, e.hub)
	if err != nil {
		_, _ = e.db.Exec(`UPDATE executions SET status='failed', failure_reason='internal_error', error_summary=?, finished_at=? WHERE id=?`,
			err.Error(), e.nowMs(), execID)
		return
	}
	defer log.Close()

	ex, err := e.GetExecution(ctx, execID)
	if err != nil {
		return
	}
	var plan Plan
	if err := json.Unmarshal([]byte(ex.PlanJSON), &plan); err != nil {
		_, _ = e.db.Exec(`UPDATE executions SET status='failed', failure_reason='internal_error', error_summary='invalid plan', finished_at=? WHERE id=?`,
			e.nowMs(), execID)
		return
	}
	sensitive, _ := e.DecryptSensitive(ex)

	start := e.nowMs()
	if _, err := e.db.ExecContext(ctx, `UPDATE executions SET status='running', started_at=? WHERE id=?`, start, execID); err != nil {
		return
	}
	log.Emit(Event{Type: "state", Entity: "execution", Status: "running"})

	hostRuns, err := e.HostRuns(ctx, execID)
	if err != nil {
		return
	}
	steps, _ := e.StepRuns(ctx, execID)
	byHost := map[string][]*StepRun{}
	for i := range steps {
		byHost[steps[i].HostRunID] = append(byHost[steps[i].HostRunID], &steps[i])
	}

	var wg sync.WaitGroup
	for i := range hostRuns {
		hr := hostRuns[i]
		wg.Add(1)
		go func() {
			defer wg.Done()
			e.runHost(ctx, log, *ex, hr, byHost[hr.ID], plan, sensitive)
		}()
	}
	wg.Wait()

	finalHosts, _ := e.HostRuns(context.Background(), execID)
	statuses := make([]string, 0, len(finalHosts))
	var success, failed, cancelled int
	for _, h := range finalHosts {
		statuses = append(statuses, h.Status)
		switch h.Status {
		case "success":
			success++
		case "failed":
			failed++
		case "cancelled":
			cancelled++
		}
	}
	status := DeriveExecutionStatus(statuses)
	if e.baseCtx.Err() != nil {
		status = "interrupted"
	}
	summary := ""
	if failed > 0 {
		summary = fmt.Sprintf("%d 台主机执行失败", failed)
	}
	failureReason := ""
	for _, h := range finalHosts {
		if h.FailureReason != "" && h.FailureReason != "cancelled" && h.FailureReason != "interrupted" {
			failureReason = h.FailureReason
			break
		}
	}
	if _, err := e.db.ExecContext(context.Background(), `UPDATE executions SET status=?, host_success=?, host_failed=?,
		host_cancelled=?, finished_at=?, failure_reason=?, error_summary=? WHERE id=?`,
		status, success, failed, cancelled, e.nowMs(), failureReason, summary, execID); err != nil {
		return
	}
	log.Emit(Event{Type: "state", Entity: "execution", Status: status})
	log.Emit(Event{Type: "end", Entity: "execution", Status: status})
	e.hub.CloseExecution(execID)
}

func findPlanStep(plan Plan, stageIdx, stepIdx int) (Step, bool) {
	for _, st := range plan.Stages {
		if st.Index != stageIdx {
			continue
		}
		for _, step := range st.Steps {
			if step.Index == stepIdx {
				return step, true
			}
		}
	}
	return Step{}, false
}

func (e *Engine) runHost(ctx context.Context, log *EventLog, ex Execution, hr HostRun, steps []*StepRun, plan Plan, sensitive map[string]string) {
	select {
	case e.hostSem <- struct{}{}:
	case <-ctx.Done():
		cs := e.cancelStatus()
		e.skipQueuedSteps(log, steps, "cancelled", "执行已取消")
		e.finishHost(log, hr.ID, cs, cs, "执行已取消")
		return
	}
	defer func() { <-e.hostSem }()

	lock := e.hostLock(hr.HostID)
	lock.Lock()
	defer lock.Unlock()
	if ctx.Err() != nil {
		cs := e.cancelStatus()
		e.skipQueuedSteps(log, steps, "cancelled", "执行已取消")
		e.finishHost(log, hr.ID, cs, cs, "执行已取消")
		return
	}

	hostCtx, cancel := context.WithCancel(ctx)
	if plan.MaxDurationSec > 0 {
		var cancelTimeout context.CancelFunc
		hostCtx, cancelTimeout = context.WithTimeout(hostCtx, time.Duration(plan.MaxDurationSec)*time.Second)
		defer cancelTimeout()
	}
	e.mu.Lock()
	e.hostCancels[hr.ID] = cancel
	e.mu.Unlock()
	defer func() {
		cancel()
		e.mu.Lock()
		delete(e.hostCancels, hr.ID)
		e.mu.Unlock()
	}()

	target, err := e.loadHost(hostCtx, hr.HostID)
	if err != nil {
		e.skipQueuedSteps(log, steps, "skipped", "主机不可用，未执行")
		e.finishHost(log, hr.ID, "failed", "host_unavailable", err.Error())
		return
	}
	runner, err := e.factory.Connect(hostCtx, target.ref)
	if err != nil {
		e.skipQueuedSteps(log, steps, "skipped", "SSH 连接失败，未执行")
		e.finishHost(log, hr.ID, "failed", "ssh_error", err.Error())
		return
	}
	defer runner.Close()

	if _, err := e.db.ExecContext(hostCtx, `UPDATE host_runs SET status='running', started_at=? WHERE id=?`, e.nowMs(), hr.ID); err != nil {
		e.skipQueuedSteps(log, steps, "skipped", "内部错误，未执行")
		e.finishHost(log, hr.ID, "failed", "internal_error", err.Error())
		return
	}
	log.Emit(Event{Type: "state", Entity: "host_run", Status: "running", HostRunID: hr.ID})

	failedEarlier := false
	statuses := make([]string, 0, len(steps))
	var flowTimedOut bool
	for stageIdx, stageCfg := range plan.Stages {
		if hostCtx.Err() != nil {
			flowTimedOut = errors.Is(hostCtx.Err(), context.DeadlineExceeded)
			for _, rest := range plan.Stages[stageIdx:] {
				for _, sr := range steps {
					if sr.StageIdx == rest.Index && sr.Status == "queued" {
						e.finishStep(log, sr, "skipped", "skipped", nil, "流程超时或已取消，未执行", 0, 0, false, 0)
						statuses = append(statuses, "skipped")
					}
				}
			}
			break
		}
		stageSteps := make([]*StepRun, 0)
		for _, sr := range steps {
			if sr.StageIdx == stageCfg.Index {
				stageSteps = append(stageSteps, sr)
			}
		}
		if failedEarlier && plan.FailurePolicy != "continue" && !stageCfg.AlwaysRun {
			for _, sr := range stageSteps {
				e.finishStep(log, sr, "skipped", "", nil, "前序阶段失败，已跳过", 0, 0, false, 0)
				statuses = append(statuses, "skipped")
			}
			continue
		}
		var wg sync.WaitGroup
		for _, sr := range stageSteps {
			st, ok := findPlanStep(plan, sr.StageIdx, sr.StepIdx)
			if !ok {
				e.finishStep(log, sr, "failed", "internal_error", nil, "计划缺少步骤定义", 0, 0, false, 0)
				statuses = append(statuses, "failed")
				failedEarlier = true
				continue
			}
			wg.Add(1)
			go func(sr *StepRun, st Step) {
				defer wg.Done()
				e.runStep(hostCtx, log, ex, hr, sr, st, sensitive, target.cap, runner)
			}(sr, st)
		}
		wg.Wait()
		for _, sr := range stageSteps {
			statuses = append(statuses, sr.Status)
			if sr.Status == "failed" || sr.Status == "interrupted" {
				failedEarlier = true
			}
		}
	}

	status := DeriveHostRunStatus(statuses)
	reason, msg := "", ""
	for _, sr := range steps {
		if sr.Status == "failed" || sr.Status == "cancelled" {
			reason, msg = sr.FailureReason, sr.ErrorMessage
			break
		}
	}
	if flowTimedOut {
		status = "failed"
		reason = "flow_timeout"
		msg = "流程执行超时"
	}
	e.finishHost(log, hr.ID, status, reason, msg)
}

func (e *Engine) skipQueuedSteps(log *EventLog, steps []*StepRun, reason, msg string) {
	for _, sr := range steps {
		if sr.Status == "queued" {
			e.finishStep(log, sr, "skipped", reason, nil, msg, 0, 0, false, 0)
		}
	}
}

func (e *Engine) finishHost(log *EventLog, hostRunID, status, reason, msg string) {
	_, _ = e.db.Exec(`UPDATE host_runs SET status=?, failure_reason=?, error_message=?, finished_at=? WHERE id=?`,
		status, reason, msg, e.nowMs(), hostRunID)
	log.Emit(Event{Type: "state", Entity: "host_run", Status: status, HostRunID: hostRunID, Message: msg})
}

func (e *Engine) finishStep(log *EventLog, sr *StepRun, status, reason string, exitCode *int, msg string, logEvents int, logBytes int64, truncated bool, dropped int64) {
	sr.Status = status
	sr.FailureReason = reason
	sr.ExitCode = exitCode
	sr.ErrorMessage = msg
	sr.LogEvents = logEvents
	sr.LogBytes = logBytes
	sr.Truncated = truncated
	sr.DroppedBytes = dropped
	now := e.nowMs()
	sr.FinishedAt = &now
	_, _ = e.db.Exec(`UPDATE step_runs SET status=?, failure_reason=?, exit_code=?, error_message=?, attempt=?,
		finished_at=?, log_events=?, log_bytes=?, truncated=?, dropped_bytes=? WHERE id=?`,
		sr.Status, sr.FailureReason, sr.ExitCode, sr.ErrorMessage, sr.Attempt,
		now, sr.LogEvents, sr.LogBytes, sr.Truncated, sr.DroppedBytes, sr.ID)
	log.Emit(Event{Type: "state", Entity: "step_run", Status: status, HostRunID: sr.HostRunID, StepRunID: sr.ID, Message: msg})
}

type logSink struct {
	mu        sync.Mutex
	log       *EventLog
	hostRunID string
	stepRunID string
	limit     int64
	written   int64
	dropped   int64
	truncated bool
	events    int
}

func (s *logSink) write(stream string, data []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.written >= s.limit {
		s.dropped += int64(len(data))
		s.truncated = true
		return
	}
	if remain := s.limit - s.written; int64(len(data)) > remain {
		s.dropped += int64(len(data)) - remain
		data = data[:remain]
		s.truncated = true
	}
	s.written += int64(len(data))
	s.events++
	s.log.Emit(Event{Type: "log", HostRunID: s.hostRunID, StepRunID: s.stepRunID, Stream: stream, Data: string(data)})
}

func (s *logSink) stats() (int, int64, bool, int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.events, s.written, s.truncated, s.dropped
}

func preflightFailure(st Step, cap capability) (string, string) {
	if cap.ProbedAt == 0 {
		return "", ""
	}
	switch st.Type {
	case "python":
		if cap.PythonPath == "" {
			return "interpreter_missing", "目标主机未找到 python3，请先安装或重新探测"
		}
	default:
		if cap.BashPath == "" {
			return "interpreter_missing", "目标主机未找到 bash，请先安装或重新探测"
		}
	}
	return "", ""
}

func (e *Engine) uploadStep(ctx context.Context, runner Runner, dir string, st Step, scriptName string, env map[string]string) error {
	if err := runner.Upload(ctx, path.Join(dir, scriptName), []byte(st.Content), 0o700); err != nil {
		return err
	}
	if err := runner.Upload(ctx, path.Join(dir, "run.sh"), BuildRunScript(st, scriptName), 0o700); err != nil {
		return err
	}
	if err := runner.Upload(ctx, path.Join(dir, ".spool-env"), BuildEnvFile(env), 0o600); err != nil {
		return err
	}
	return nil
}

func (e *Engine) cleanup(runner Runner, dir string, sudo bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, _ = runner.RunStream(ctx, CleanupCommand(dir, sudo), nil, nil)
}

func (e *Engine) killRemote(runner Runner, dir string, sudo bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	b, err := runner.ReadFile(ctx, path.Join(dir, ".spool.pid"))
	if err != nil {
		return
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil || pid <= 0 {
		return
	}
	_, _ = runner.RunStream(ctx, KillCommand(pid, sudo, false), nil, nil)
	select {
	case <-time.After(10 * time.Second):
	case <-ctx.Done():
		return
	}
	_, _ = runner.RunStream(ctx, KillCommand(pid, sudo, true), nil, nil)
}

func (e *Engine) runStep(ctx context.Context, log *EventLog, ex Execution, hr HostRun, sr *StepRun, st Step, sensitive map[string]string, cap capability, runner Runner) {
	select {
	case e.stepSem <- struct{}{}:
	case <-ctx.Done():
		cs := e.cancelStatus()
		e.finishStep(log, sr, cs, cs, nil, "执行已取消", 0, 0, false, 0)
		return
	}
	defer func() { <-e.stepSem }()
	if ctx.Err() != nil {
		cs := e.cancelStatus()
		e.finishStep(log, sr, cs, cs, nil, "执行已取消", 0, 0, false, 0)
		return
	}

	timeout := st.TimeoutSec
	if timeout <= 0 {
		timeout = e.cfg.DefaultStepTimeoutSec
	}
	st.TimeoutSec = timeout
	dir := RemoteDir(e.cfg.RemoteBaseDir, sr.ID)
	scriptName := ScriptFileName(st.Type)
	start := e.nowMs()
	sr.Attempt = 1
	sr.StartedAt = &start
	_, _ = e.db.Exec(`UPDATE step_runs SET status='running', attempt=1, started_at=? WHERE id=?`, start, sr.ID)
	log.Emit(Event{Type: "state", Entity: "step_run", Status: "running", HostRunID: hr.ID, StepRunID: sr.ID})

	if reason, msg := preflightFailure(st, cap); reason != "" {
		e.finishStep(log, sr, "failed", reason, nil, msg, 0, 0, false, 0)
		return
	}

	env := make(map[string]string, len(st.Params)+len(st.SensitiveParams)+6)
	for k, v := range st.Params {
		env[k] = v
	}
	for envName, sourceKey := range st.SensitiveParams {
		if v, ok := sensitive[sourceKey]; ok {
			env[envName] = v
		}
	}
	env["SPOOL_EXECUTION_ID"] = ex.ID
	env["SPOOL_HOST_RUN_ID"] = hr.ID
	env["SPOOL_STEP_RUN_ID"] = sr.ID
	env["SPOOL_HOST_NAME"] = hr.HostName
	env["SPOOL_TOOL_NAME"] = st.Name
	env["SPOOL_TOOL_VERSION"] = strconv.Itoa(st.ToolVersionNo)

	sink := &logSink{log: log, hostRunID: hr.ID, stepRunID: sr.ID, limit: e.cfg.OutputLimitBytes}
	cmd := BuildCommand(dir, st, cap.Setsid, cap.Timeout)

	for attempt := 1; attempt <= st.RetryCount+1; attempt++ {
		sr.Attempt = attempt
		if attempt > 1 {
			delay := st.RetryDelaySec
			if delay <= 0 {
				delay = 5
			}
			select {
			case <-time.After(time.Duration(delay) * time.Second):
			case <-ctx.Done():
				cs := e.cancelStatus()
				e.finishStep(log, sr, cs, cs, nil, "执行已取消", 0, 0, false, 0)
				return
			}
		}
		if err := e.uploadStep(ctx, runner, dir, st, scriptName, env); err != nil {
			if ctx.Err() != nil {
				cs := e.cancelStatus()
				e.finishStep(log, sr, cs, cs, nil, "执行已取消", 0, 0, false, 0)
				return
			}
			if attempt <= st.RetryCount {
				continue
			}
			e.cleanup(runner, dir, st.Sudo)
			ev, written, truncated, dropped := sink.stats()
			e.finishStep(log, sr, "failed", "upload_error", nil, err.Error(), ev, written, truncated, dropped)
			return
		}

		stepCtx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
		killDone := make(chan struct{})
		go func() {
			select {
			case <-stepCtx.Done():
				if ctx.Err() != nil || errors.Is(stepCtx.Err(), context.DeadlineExceeded) {
					e.killRemote(runner, dir, st.Sudo)
				}
			case <-killDone:
			}
		}()
		code, runErr := runner.RunStream(stepCtx, cmd, sink.writeOut, sink.writeErr)
		close(killDone)
		cancel()

		if runErr == nil && code == 0 {
			e.cleanup(runner, dir, st.Sudo)
			ev, written, truncated, dropped := sink.stats()
			exit := code
			e.finishStep(log, sr, "success", "", &exit, "", ev, written, truncated, dropped)
			return
		}

		reason := "ssh_error"
		msg := ""
		var exit *int
		switch {
		case ctx.Err() != nil:
			e.cleanup(runner, dir, st.Sudo)
			ev, written, truncated, dropped := sink.stats()
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				e.finishStep(log, sr, "failed", "timeout", nil, "流程执行超时", ev, written, truncated, dropped)
				return
			}
			cs := e.cancelStatus()
			e.finishStep(log, sr, cs, cs, nil, "执行已取消", ev, written, truncated, dropped)
			return
		case errors.Is(stepCtx.Err(), context.DeadlineExceeded) || errors.Is(runErr, context.DeadlineExceeded):
			reason = "timeout"
			msg = fmt.Sprintf("执行超时（%d 秒）", timeout)
		case runErr != nil:
			reason = "ssh_error"
			msg = runErr.Error()
		default:
			reason = "exit_code"
			msg = fmt.Sprintf("退出码 %d", code)
			exit = &code
		}

		if attempt <= st.RetryCount {
			continue
		}
		e.cleanup(runner, dir, st.Sudo)
		ev, written, truncated, dropped := sink.stats()
		e.finishStep(log, sr, "failed", reason, exit, msg, ev, written, truncated, dropped)
		return
	}
}

func (s *logSink) writeOut(data []byte) { s.write("stdout", data) }
func (s *logSink) writeErr(data []byte) { s.write("stderr", data) }
