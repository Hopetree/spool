package flow

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/Hopetree/spool/internal/exec"
	"github.com/Hopetree/spool/internal/store"
	"github.com/Hopetree/spool/internal/tool"
)

type flowRow struct {
	ID             string `db:"id"`
	Name           string `db:"name"`
	Description    string `db:"description"`
	ParamsJSON     string `db:"params_json"`
	FailurePolicy  string `db:"failure_policy"`
	MaxDurationSec int    `db:"max_duration_sec"`
	Enabled        bool   `db:"enabled"`
	StageCount     int    `db:"stage_count"`
	StepCount      int    `db:"step_count"`
	CreatedAt      int64  `db:"created_at"`
	UpdatedAt      int64  `db:"updated_at"`
}

type stageRow struct {
	ID        string `db:"id"`
	FlowID    string `db:"flow_id"`
	Index     int    `db:"idx"`
	Name      string `db:"name"`
	AlwaysRun bool   `db:"always_run"`
}

type stepRow struct {
	ID            string `db:"id"`
	StageID       string `db:"stage_id"`
	Index         int    `db:"idx"`
	Name          string `db:"name"`
	ToolID        string `db:"tool_id"`
	MappingJSON   string `db:"param_mapping_json"`
	TimeoutSec    int    `db:"timeout_sec"`
	RetryCount    int    `db:"retry_count"`
	RetryDelaySec int    `db:"retry_delay_sec"`
	Enabled       bool   `db:"enabled"`
	ToolName      string `db:"tool_name"`
	ToolType      string `db:"tool_type"`
}

func (r flowRow) toFlow() Flow {
	params := []tool.Param{}
	if r.ParamsJSON != "" {
		_ = json.Unmarshal([]byte(r.ParamsJSON), &params)
	}
	return Flow{
		ID: r.ID, Name: r.Name, Description: r.Description, Params: params,
		FailurePolicy: r.FailurePolicy, MaxDurationSec: r.MaxDurationSec,
		Enabled: r.Enabled, StageCount: r.StageCount, StepCount: r.StepCount,
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
}

const flowSelect = `SELECT f.id, f.name, f.description, f.params_json, f.failure_policy, f.max_duration_sec,
	f.enabled, f.created_at, f.updated_at,
	(SELECT count(*) FROM flow_stages s WHERE s.flow_id = f.id) AS stage_count,
	(SELECT count(*) FROM flow_steps st JOIN flow_stages s ON s.id = st.stage_id WHERE s.flow_id = f.id) AS step_count
	FROM flows f`

// Service manages flows.
type Service struct {
	db    *sqlx.DB
	tools *tool.Service
	now   func() time.Time
}

// NewService builds a flow Service.
func NewService(db *sqlx.DB, tools *tool.Service) *Service {
	return &Service{db: db, tools: tools, now: time.Now}
}

func (s *Service) nowMs() int64 { return s.now().UnixMilli() }

// List returns a page of flows and the total count.
func (s *Service) List(ctx context.Context, f ListFilter) ([]Flow, int64, error) {
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
		where = append(where, "(f.name LIKE ? OR f.description LIKE ?)")
		like := "%" + kw + "%"
		args = append(args, like, like)
	}
	if f.Enabled != nil {
		where = append(where, "f.enabled = ?")
		args = append(args, *f.Enabled)
	}
	cond := strings.Join(where, " AND ")
	var total int64
	if err := s.db.GetContext(ctx, &total, `SELECT count(*) FROM flows f WHERE `+cond, args...); err != nil {
		return nil, 0, fmt.Errorf("count flows: %w", err)
	}
	pageArgs := append([]any{}, args...)
	pageArgs = append(pageArgs, f.PageSize, (f.Page-1)*f.PageSize)
	rows := []flowRow{}
	if err := s.db.SelectContext(ctx, &rows, flowSelect+` WHERE `+cond+` ORDER BY f.name LIMIT ? OFFSET ?`, pageArgs...); err != nil {
		return nil, 0, fmt.Errorf("list flows: %w", err)
	}
	out := make([]Flow, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.toFlow())
	}
	return out, total, nil
}

// Get returns a flow with its definition.
func (s *Service) Get(ctx context.Context, id string) (*Detail, error) {
	var fr flowRow
	err := s.db.GetContext(ctx, &fr, flowSelect+` WHERE f.id = ?`, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get flow: %w", err)
	}
	stages := []stageRow{}
	if err := s.db.SelectContext(ctx, &stages, `SELECT id, flow_id, idx, name, always_run FROM flow_stages WHERE flow_id = ? ORDER BY idx`, id); err != nil {
		return nil, fmt.Errorf("get stages: %w", err)
	}
	detail := Detail{Flow: fr.toFlow(), Stages: make([]Stage, 0, len(stages))}
	for _, sr := range stages {
		steps := []stepRow{}
		err := s.db.SelectContext(ctx, &steps, `SELECT st.id, st.stage_id, st.idx, st.name, st.tool_id, st.param_mapping_json,
			st.timeout_sec, st.retry_count, st.retry_delay_sec, st.enabled,
			COALESCE(t.name, '') AS tool_name, COALESCE(v.type, '') AS tool_type
			FROM flow_steps st
			LEFT JOIN tools t ON t.id = st.tool_id
			LEFT JOIN tool_versions v ON v.id = t.current_version_id
			WHERE st.stage_id = ? ORDER BY st.idx`, sr.ID)
		if err != nil {
			return nil, fmt.Errorf("get steps: %w", err)
		}
		stage := Stage{ID: sr.ID, Index: sr.Index, Name: sr.Name, AlwaysRun: sr.AlwaysRun, Steps: make([]Step, 0, len(steps))}
		for _, row := range steps {
			mapping := map[string]Mapping{}
			if row.MappingJSON != "" {
				_ = json.Unmarshal([]byte(row.MappingJSON), &mapping)
			}
			stage.Steps = append(stage.Steps, Step{
				ID: row.ID, Index: row.Index, Name: row.Name, ToolID: row.ToolID,
				ToolName: row.ToolName, ToolType: row.ToolType, Mapping: mapping,
				TimeoutSec: row.TimeoutSec, RetryCount: row.RetryCount,
				RetryDelaySec: row.RetryDelaySec, Enabled: row.Enabled,
			})
		}
		detail.Stages = append(detail.Stages, stage)
	}
	return &detail, nil
}

func (s *Service) validateTools(ctx context.Context, in Input) error {
	flowParams := map[string]tool.Param{}
	for _, p := range in.Params {
		flowParams[p.Name] = p
	}
	for _, stage := range in.Stages {
		for _, step := range stage.Steps {
			td, err := s.tools.Get(ctx, step.ToolID)
			if errors.Is(err, tool.ErrNotFound) {
				return invalid(fmt.Sprintf("步骤 %q 引用的工具不存在", step.Name))
			}
			if err != nil {
				return err
			}
			if !td.Enabled {
				return invalid(fmt.Sprintf("步骤 %q 引用的工具 %q 已停用", step.Name, td.Name))
			}
			declared := map[string]tool.Param{}
			for _, p := range td.Version.Params {
				declared[p.Name] = p
			}
			for key := range step.Mapping {
				if _, ok := declared[key]; !ok {
					return invalid(fmt.Sprintf("步骤 %q 映射了工具未声明的参数 %q", step.Name, key))
				}
			}
			for _, p := range td.Version.Params {
				m, ok := step.Mapping[p.Name]
				if ok {
					switch m.Source {
					case "flow":
						fp, exists := flowParams[m.Value]
						if !exists {
							return invalid(fmt.Sprintf("步骤 %q 引用了不存在的流程参数 %q", step.Name, m.Value))
						}
						if p.Sensitive && !fp.Sensitive {
							return invalid(fmt.Sprintf("步骤 %q 的工具参数 %q 是敏感参数，必须引用敏感的流程参数", step.Name, p.Name))
						}
					case "literal":
						if p.Sensitive {
							return invalid(fmt.Sprintf("步骤 %q 的敏感参数 %q 不能使用固定值", step.Name, p.Name))
						}
						if _, err := tool.ValidateValues([]tool.Param{p}, map[string]string{p.Name: m.Value}); err != nil {
							return invalid(fmt.Sprintf("步骤 %q 的参数 %q：%s", step.Name, p.Name, err.Error()))
						}
					}
					continue
				}
				if p.Required && p.Default == "" {
					return invalid(fmt.Sprintf("步骤 %q 的工具参数 %q 必填，请配置映射或默认值", step.Name, p.Name))
				}
			}
		}
	}
	return nil
}

func marshalParams(params []tool.Param) string {
	if params == nil {
		params = []tool.Param{}
	}
	b, _ := json.Marshal(params)
	return string(b)
}

func (s *Service) insertDefinition(ctx context.Context, tx *sqlx.Tx, flowID string, in Input) error {
	for si, stage := range in.Stages {
		stageID := store.NewULID()
		_, err := tx.ExecContext(ctx, `INSERT INTO flow_stages (id, flow_id, idx, name, always_run) VALUES (?, ?, ?, ?, ?)`,
			stageID, flowID, si+1, stage.Name, stage.AlwaysRun)
		if err != nil {
			return fmt.Errorf("insert stage: %w", err)
		}
		for i, step := range stage.Steps {
			mapping, err := json.Marshal(step.Mapping)
			if err != nil {
				return fmt.Errorf("marshal mapping: %w", err)
			}
			_, err = tx.ExecContext(ctx, `INSERT INTO flow_steps
				(id, stage_id, idx, name, tool_id, param_mapping_json, timeout_sec, retry_count, retry_delay_sec, enabled)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				store.NewULID(), stageID, i+1, step.Name, step.ToolID, string(mapping),
				step.TimeoutSec, step.RetryCount, step.RetryDelaySec, step.Enabled)
			if err != nil {
				return fmt.Errorf("insert step: %w", err)
			}
		}
	}
	return nil
}

func uniqueNameError(err error) error {
	if err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed") {
		return invalid("流程名称已存在")
	}
	return nil
}

// Create inserts a flow definition.
func (s *Service) Create(ctx context.Context, in Input, actorID string) (*Detail, error) {
	if err := validateInput(&in); err != nil {
		return nil, err
	}
	if err := s.validateTools(ctx, in); err != nil {
		return nil, err
	}
	id := store.NewULID()
	now := s.nowMs()
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	_, err = tx.ExecContext(ctx, `INSERT INTO flows
		(id, name, description, params_json, failure_policy, max_duration_sec, enabled, created_by, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, 1, ?, ?, ?)`,
		id, in.Name, in.Description, marshalParams(in.Params), in.FailurePolicy, in.MaxDurationSec, actorID, now, now)
	if err != nil {
		if ue := uniqueNameError(err); ue != nil {
			return nil, ue
		}
		return nil, fmt.Errorf("insert flow: %w", err)
	}
	if err := s.insertDefinition(ctx, tx, id, in); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit flow: %w", err)
	}
	return s.Get(ctx, id)
}

// Update replaces the whole definition.
func (s *Service) Update(ctx context.Context, id string, in Input) (*Detail, error) {
	if _, err := s.Get(ctx, id); err != nil {
		return nil, err
	}
	if err := validateInput(&in); err != nil {
		return nil, err
	}
	if err := s.validateTools(ctx, in); err != nil {
		return nil, err
	}
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	_, err = tx.ExecContext(ctx, `UPDATE flows SET name = ?, description = ?, params_json = ?, failure_policy = ?,
		max_duration_sec = ?, updated_at = ? WHERE id = ?`,
		in.Name, in.Description, marshalParams(in.Params), in.FailurePolicy, in.MaxDurationSec, s.nowMs(), id)
	if err != nil {
		if ue := uniqueNameError(err); ue != nil {
			return nil, ue
		}
		return nil, fmt.Errorf("update flow: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM flow_stages WHERE flow_id = ?`, id); err != nil {
		return nil, fmt.Errorf("clear stages: %w", err)
	}
	if err := s.insertDefinition(ctx, tx, id, in); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit flow: %w", err)
	}
	return s.Get(ctx, id)
}

// SetEnabled toggles whether a flow can be executed.
func (s *Service) SetEnabled(ctx context.Context, id string, enabled bool) error {
	res, err := s.db.ExecContext(ctx, `UPDATE flows SET enabled = ?, updated_at = ? WHERE id = ?`, enabled, s.nowMs(), id)
	if err != nil {
		return fmt.Errorf("set enabled: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// Delete removes a flow definition.
func (s *Service) Delete(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM flows WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete flow: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// Copy clones a flow under a new name.
func (s *Service) Copy(ctx context.Context, id, actorID string) (*Detail, error) {
	src, err := s.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	in := Input{
		Name: src.Name + "-copy", Description: src.Description, Params: src.Params,
		FailurePolicy: src.FailurePolicy, MaxDurationSec: src.MaxDurationSec,
	}
	for _, stage := range src.Stages {
		si := StageInput{Name: stage.Name, AlwaysRun: stage.AlwaysRun}
		for _, step := range stage.Steps {
			si.Steps = append(si.Steps, StepInput{
				Name: step.Name, ToolID: step.ToolID, Mapping: step.Mapping,
				TimeoutSec: step.TimeoutSec, RetryCount: step.RetryCount,
				RetryDelaySec: step.RetryDelaySec, Enabled: step.Enabled,
			})
		}
		in.Stages = append(in.Stages, si)
	}
	base := src.Name + "-copy"
	for i := 2; i < 100; i++ {
		var n int
		if err := s.db.GetContext(ctx, &n, `SELECT count(*) FROM flows WHERE name = ?`, in.Name); err != nil {
			return nil, fmt.Errorf("check name: %w", err)
		}
		if n == 0 {
			break
		}
		in.Name = fmt.Sprintf("%s-%d", base, i)
	}
	return s.Create(ctx, in, actorID)
}

// CompileResult is a resolved execution plan plus its parameter split.
type CompileResult struct {
	Plan      exec.Plan
	Plain     map[string]string
	Sensitive map[string]string
}

// Compile resolves the latest tool versions and builds an execution plan.
func (s *Service) Compile(ctx context.Context, id string, values map[string]string) (*CompileResult, error) {
	detail, err := s.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if !detail.Enabled {
		return nil, ErrDisabled
	}
	flowValues, err := tool.ValidateValues(detail.Params, values)
	if err != nil {
		return nil, err
	}
	plain := map[string]string{}
	sensitive := map[string]string{}
	flowSensitive := map[string]bool{}
	for _, p := range detail.Params {
		v := flowValues[p.Name]
		if p.Sensitive {
			flowSensitive[p.Name] = true
			sensitive[p.Name] = v
			continue
		}
		plain[p.Name] = v
	}

	plan := exec.Plan{
		Kind: "flow", Name: detail.Name, FailurePolicy: detail.FailurePolicy,
		MaxDurationSec: detail.MaxDurationSec,
	}
	stageIdx := 1
	for _, stage := range detail.Stages {
		es := exec.Stage{Index: stageIdx, Name: stage.Name, AlwaysRun: stage.AlwaysRun}
		stepIdx := 1
		for _, step := range stage.Steps {
			if !step.Enabled {
				continue
			}
			td, err := s.tools.Get(ctx, step.ToolID)
			if errors.Is(err, tool.ErrNotFound) {
				return nil, invalid(fmt.Sprintf("步骤 %q 引用的工具不存在", step.Name))
			}
			if err != nil {
				return nil, err
			}
			if !td.Enabled {
				return nil, invalid(fmt.Sprintf("步骤 %q 引用的工具 %q 已停用", step.Name, td.Name))
			}
			resolved := map[string]string{}
			for _, p := range td.Version.Params {
				if m, ok := step.Mapping[p.Name]; ok {
					if m.Source == "flow" {
						resolved[p.Name] = flowValues[m.Value]
					} else {
						resolved[p.Name] = m.Value
					}
					continue
				}
				if p.Default != "" {
					resolved[p.Name] = p.Default
				}
			}
			validated, err := tool.ValidateValues(td.Version.Params, resolved)
			if err != nil {
				return nil, invalid(fmt.Sprintf("步骤 %q 的参数校验失败：%s", step.Name, err.Error()))
			}
			stepPlain := map[string]string{}
			stepSensitive := map[string]string{}
			for _, p := range td.Version.Params {
				v := validated[p.Name]
				secret := p.Sensitive
				if m, ok := step.Mapping[p.Name]; ok && m.Source == "flow" && flowSensitive[m.Value] {
					secret = true
				}
				if secret {
					key := fmt.Sprintf("s%d.%d.%s", stageIdx, stepIdx, p.Name)
					sensitive[key] = v
					stepSensitive[p.Name] = key
					continue
				}
				stepPlain[p.Name] = v
			}
			timeout := step.TimeoutSec
			if timeout == 0 {
				timeout = td.Version.TimeoutSec
			}
			es.Steps = append(es.Steps, exec.Step{
				Index: stepIdx, Name: step.Name,
				ToolID: td.ID, ToolVersionID: td.Version.ID, ToolVersionNo: td.Version.VersionNo,
				ContentHash: td.Version.ContentHash, Type: td.Version.Type,
				Interpreter: td.Version.Interpreter, Content: td.Version.Content,
				PrepareCommand: td.Version.PrepareCommand, Sudo: td.Version.Sudo,
				TimeoutSec: timeout, RetryCount: step.RetryCount, RetryDelaySec: step.RetryDelaySec,
				Params: stepPlain, SensitiveParams: stepSensitive,
			})
			stepIdx++
		}
		if len(es.Steps) == 0 {
			continue
		}
		plan.Stages = append(plan.Stages, es)
		stageIdx++
	}
	if len(plan.Stages) == 0 {
		return nil, invalid("流程没有可执行的步骤")
	}
	return &CompileResult{Plan: plan, Plain: plain, Sensitive: sensitive}, nil
}
