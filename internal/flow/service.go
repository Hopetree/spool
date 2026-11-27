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
	Category       string `db:"category"`
	Description    string `db:"description"`
	ParamsJSON     string `db:"params_json"`
	MaxDurationSec int    `db:"max_duration_sec"`
	Enabled        bool   `db:"enabled"`
	NodeCount      int    `db:"node_count"`
	EdgeCount      int    `db:"edge_count"`
	CreatedAt      int64  `db:"created_at"`
	UpdatedAt      int64  `db:"updated_at"`
}

type nodeRow struct {
	ID            string  `db:"id"`
	FlowID        string  `db:"flow_id"`
	NodeKey       string  `db:"node_key"`
	Type          string  `db:"type"`
	Name          string  `db:"name"`
	ToolID        string  `db:"tool_id"`
	MappingJSON   string  `db:"mapping_json"`
	TimeoutSec    int     `db:"timeout_sec"`
	RetryCount    int     `db:"retry_count"`
	RetryDelaySec int     `db:"retry_delay_sec"`
	X             float64 `db:"x"`
	Y             float64 `db:"y"`
	ToolName      string  `db:"tool_name"`
	ToolType      string  `db:"tool_type"`
}

type edgeRow struct {
	ID             string `db:"id"`
	FlowID         string `db:"flow_id"`
	FromKey        string `db:"from_key"`
	ToKey          string `db:"to_key"`
	Condition      string `db:"condition"`
	ConditionValue string `db:"condition_value"`
}

const flowSelect = `SELECT f.id, f.name, f.category, f.description, f.params_json, f.max_duration_sec, f.enabled,
	f.created_at, f.updated_at,
	(SELECT count(*) FROM flow_nodes n WHERE n.flow_id = f.id AND n.type = 'tool') AS node_count,
	(SELECT count(*) FROM flow_edges e WHERE e.flow_id = f.id) AS edge_count
	FROM flows f`

// Service manages flow graphs.
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

func (r flowRow) toFlow() Flow {
	params := []tool.Param{}
	if r.ParamsJSON != "" {
		_ = json.Unmarshal([]byte(r.ParamsJSON), &params)
	}
	return Flow{
		ID: r.ID, Name: r.Name, Category: r.Category, Description: r.Description, Params: params,
		MaxDurationSec: r.MaxDurationSec, Enabled: r.Enabled,
		NodeCount: r.NodeCount, EdgeCount: r.EdgeCount,
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
}

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
	if f.Uncategorized {
		where = append(where, "f.category = ''")
	} else if f.Category != "" {
		where = append(where, "(f.category = ? OR f.category LIKE ?)")
		args = append(args, f.Category, f.Category+"/%")
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

// Get returns a flow with its graph.
func (s *Service) Get(ctx context.Context, id string) (*Detail, error) {
	var fr flowRow
	err := s.db.GetContext(ctx, &fr, flowSelect+` WHERE f.id = ?`, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get flow: %w", err)
	}
	rows := []nodeRow{}
	err = s.db.SelectContext(ctx, &rows, `SELECT n.id, n.flow_id, n.node_key, n.type, n.name, n.tool_id, n.mapping_json,
		n.timeout_sec, n.retry_count, n.retry_delay_sec, n.x, n.y,
		COALESCE(t.name, '') AS tool_name, COALESCE(v.type, '') AS tool_type
		FROM flow_nodes n
		LEFT JOIN tools t ON t.id = n.tool_id
		LEFT JOIN tool_versions v ON v.id = t.current_version_id
		WHERE n.flow_id = ?
		ORDER BY CASE n.type WHEN 'start' THEN 0 ELSE 1 END, n.name`, id)
	if err != nil {
		return nil, fmt.Errorf("get nodes: %w", err)
	}
	edges := []edgeRow{}
	err = s.db.SelectContext(ctx, &edges, `SELECT id, flow_id, from_key, to_key, condition, condition_value
		FROM flow_edges WHERE flow_id = ? ORDER BY rowid`, id)
	if err != nil {
		return nil, fmt.Errorf("get edges: %w", err)
	}
	detail := Detail{Flow: fr.toFlow(), Nodes: make([]Node, 0, len(rows)), Edges: make([]Edge, 0, len(edges))}
	for _, r := range rows {
		mapping := map[string]Mapping{}
		if r.MappingJSON != "" {
			_ = json.Unmarshal([]byte(r.MappingJSON), &mapping)
		}
		detail.Nodes = append(detail.Nodes, Node{
			ID: r.NodeKey, Type: r.Type, Name: r.Name, ToolID: r.ToolID,
			ToolName: r.ToolName, ToolType: r.ToolType, Mapping: mapping,
			TimeoutSec: r.TimeoutSec, RetryCount: r.RetryCount, RetryDelaySec: r.RetryDelaySec,
			X: r.X, Y: r.Y,
		})
	}
	for _, e := range edges {
		detail.Edges = append(detail.Edges, Edge{
			ID: e.ID, From: e.FromKey, To: e.ToKey, Condition: e.Condition, Value: e.ConditionValue,
		})
	}
	normalizeLegacyNodeMappings(&detail)
	return &detail, nil
}

func (s *Service) loadNodeTools(ctx context.Context, nodes []NodeInput) (map[string]*tool.Detail, error) {
	out := make(map[string]*tool.Detail)
	for _, node := range nodes {
		if node.Type != NodeTool {
			continue
		}
		td, err := s.tools.Get(ctx, node.ToolID)
		if errors.Is(err, tool.ErrNotFound) {
			return nil, invalid(fmt.Sprintf("节点 %q 引用的工具不存在", node.Name))
		}
		if err != nil {
			return nil, err
		}
		if !td.Enabled {
			return nil, invalid(fmt.Sprintf("节点 %q 引用的工具 %q 已停用", node.Name, td.Name))
		}
		out[node.ID] = td
	}
	return out, nil
}

func validateAndDeriveParams(in *Input, toolsByNode map[string]*tool.Detail) error {
	for _, node := range in.Nodes {
		if node.Type != NodeTool {
			continue
		}
		td := toolsByNode[node.ID]
		flowParams := td.Version.FlowParams()
		declared := make(map[string]bool, len(flowParams))
		for _, p := range flowParams {
			declared[p.Name] = true
		}
		for key := range node.Mapping {
			if !declared[key] {
				return invalid(fmt.Sprintf("节点 %q 映射了工具未声明的参数 %q", node.Name, key))
			}
		}
	}
	resolver, err := newParamResolver(in.Nodes, in.Edges, toolsByNode)
	if err != nil {
		return err
	}
	inputs, err := deriveInputs(resolver, true)
	if err != nil {
		return err
	}
	in.Params = inputs
	return nil
}

func marshalParams(params []tool.Param) string {
	if params == nil {
		params = []tool.Param{}
	}
	b, _ := json.Marshal(params)
	return string(b)
}

func (s *Service) insertGraph(ctx context.Context, tx *sqlx.Tx, flowID string, in Input) error {
	for _, node := range in.Nodes {
		mapping, err := json.Marshal(node.Mapping)
		if err != nil {
			return fmt.Errorf("marshal mapping: %w", err)
		}
		dbID := store.NewULID()
		if node.Type == NodeStart {
			dbID = "start-" + flowID
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO flow_nodes
			(id, flow_id, node_key, type, name, tool_id, mapping_json, timeout_sec, retry_count, retry_delay_sec, enabled, x, y)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1, ?, ?)`,
			dbID, flowID, node.ID, node.Type, node.Name, node.ToolID, string(mapping),
			node.TimeoutSec, node.RetryCount, node.RetryDelaySec, node.X, node.Y)
		if err != nil {
			return fmt.Errorf("insert node: %w", err)
		}
	}
	for _, edge := range in.Edges {
		_, err := tx.ExecContext(ctx, `INSERT INTO flow_edges (id, flow_id, from_key, to_key, condition, condition_value)
			VALUES (?, ?, ?, ?, ?, ?)`,
			store.NewULID(), flowID, edge.From, edge.To, edge.Condition, edge.Value)
		if err != nil {
			return fmt.Errorf("insert edge: %w", err)
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

// Create inserts a flow graph.
func (s *Service) Create(ctx context.Context, in Input, actorID string) (*Detail, error) {
	normalizeLegacyInputMappings(in.Nodes, in.Params)
	if err := validateInput(&in); err != nil {
		return nil, err
	}
	toolsByNode, err := s.loadNodeTools(ctx, in.Nodes)
	if err != nil {
		return nil, err
	}
	if err := validateAndDeriveParams(&in, toolsByNode); err != nil {
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
		(id, name, category, description, params_json, failure_policy, max_duration_sec, enabled, created_by, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, 'continue', ?, 1, ?, ?, ?)`,
		id, in.Name, in.Category, in.Description, marshalParams(in.Params), in.MaxDurationSec, actorID, now, now)
	if err != nil {
		if ue := uniqueNameError(err); ue != nil {
			return nil, ue
		}
		return nil, fmt.Errorf("insert flow: %w", err)
	}
	if err := s.insertGraph(ctx, tx, id, in); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit flow: %w", err)
	}
	return s.Get(ctx, id)
}

// Update replaces the whole graph.
func (s *Service) Update(ctx context.Context, id string, in Input) (*Detail, error) {
	existing, err := s.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	normalizeLegacyInputMappings(in.Nodes, in.Params, existing.Params)
	if err := validateInput(&in); err != nil {
		return nil, err
	}
	toolsByNode, err := s.loadNodeTools(ctx, in.Nodes)
	if err != nil {
		return nil, err
	}
	if err := validateAndDeriveParams(&in, toolsByNode); err != nil {
		return nil, err
	}
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	_, err = tx.ExecContext(ctx, `UPDATE flows SET name = ?, category = ?, description = ?, params_json = ?,
		max_duration_sec = ?, updated_at = ? WHERE id = ?`,
		in.Name, in.Category, in.Description, marshalParams(in.Params), in.MaxDurationSec, s.nowMs(), id)
	if err != nil {
		if ue := uniqueNameError(err); ue != nil {
			return nil, ue
		}
		return nil, fmt.Errorf("update flow: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM flow_edges WHERE flow_id = ?`, id); err != nil {
		return nil, fmt.Errorf("clear edges: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM flow_nodes WHERE flow_id = ?`, id); err != nil {
		return nil, fmt.Errorf("clear nodes: %w", err)
	}
	if err := s.insertGraph(ctx, tx, id, in); err != nil {
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

// Delete removes a flow graph.
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
		Name: src.Name + "-copy", Category: src.Category, Description: src.Description, Params: src.Params,
		MaxDurationSec: src.MaxDurationSec,
	}
	for _, n := range src.Nodes {
		in.Nodes = append(in.Nodes, NodeInput{
			ID: n.ID, Type: n.Type, Name: n.Name, ToolID: n.ToolID, Mapping: n.Mapping,
			TimeoutSec: n.TimeoutSec, RetryCount: n.RetryCount, RetryDelaySec: n.RetryDelaySec,
			X: n.X, Y: n.Y,
		})
	}
	for _, e := range src.Edges {
		in.Edges = append(in.Edges, EdgeInput{From: e.From, To: e.To, Condition: e.Condition, Value: e.Value})
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

// Categories returns category paths with flow counts.
func (s *Service) Categories(ctx context.Context) ([]tool.CategoryCount, error) {
	var rows []struct {
		Category string `db:"category"`
		Count    int    `db:"count"`
	}
	if err := s.db.SelectContext(ctx, &rows, `SELECT category, count(*) AS count FROM flows GROUP BY category ORDER BY category`); err != nil {
		return nil, fmt.Errorf("list flow categories: %w", err)
	}
	out := make([]tool.CategoryCount, 0, len(rows))
	for _, r := range rows {
		out = append(out, tool.CategoryCount{Path: r.Category, Count: r.Count})
	}
	return out, nil
}

// CompileResult is a resolved execution plan plus its parameter split.
type CompileResult struct {
	Plan      exec.Plan
	Plain     map[string]string
	Sensitive map[string]string
}

// Compile resolves the latest tool versions and builds a graph execution plan.
func (s *Service) Compile(ctx context.Context, id string, values map[string]string) (*CompileResult, error) {
	detail, err := s.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if !detail.Enabled {
		return nil, ErrDisabled
	}
	nodes := nodeInputsFromDetail(detail.Nodes)
	edges := edgeInputsFromDetail(detail.Edges)
	toolsByNode, err := s.loadNodeTools(ctx, nodes)
	if err != nil {
		return nil, err
	}
	resolver, err := newParamResolver(nodes, edges, toolsByNode)
	if err != nil {
		return nil, err
	}
	inputs, err := deriveInputs(resolver, false)
	if err != nil {
		return nil, err
	}
	filtered := make(map[string]string, len(inputs))
	for _, p := range inputs {
		if v, ok := values[p.Name]; ok {
			filtered[p.Name] = v
		}
	}
	flowValues, err := tool.ValidateValues(inputs, filtered)
	if err != nil {
		return nil, err
	}
	plain := map[string]string{}
	sensitive := map[string]string{}
	inputSensitive := make(map[string]bool, len(inputs))
	for _, p := range inputs {
		v := flowValues[p.Name]
		inputSensitive[p.Name] = p.Sensitive
		if p.Sensitive {
			sensitive[p.Name] = v
			continue
		}
		plain[p.Name] = v
	}

	graph := &exec.GraphPlan{}
	for _, node := range detail.Nodes {
		if node.Type == NodeStart {
			graph.Nodes = append(graph.Nodes, exec.GraphNode{
				ID: node.ID, Type: NodeStart, Name: "开始",
				X: node.X, Y: node.Y,
			})
			continue
		}
		td := toolsByNode[node.ID]
		nodePlain := map[string]string{}
		nodeSensitive := map[string]string{}
		hostIDs := []string{}
		for _, p := range td.Version.FlowParams() {
			res, err := resolver.resolve(node.ID, p.Name)
			if err != nil {
				return nil, err
			}
			value := res.Value
			if res.Kind == "input" {
				value = flowValues[res.InputName]
				if value == "" {
					if p.Required {
						return nil, invalid(fmt.Sprintf("节点 %q 的参数 %q 不能为空", node.Name, p.Name))
					}
				} else if err := tool.ValidateValue(p, value); err != nil {
					return nil, invalid(fmt.Sprintf("节点 %q 的参数校验失败：%s", node.Name, err.Error()))
				}
			} else {
				validated, err := tool.ValidateValues([]tool.Param{p}, map[string]string{p.Name: value})
				if err != nil {
					return nil, invalid(fmt.Sprintf("节点 %q 的参数校验失败：%s", node.Name, err.Error()))
				}
				value = validated[p.Name]
			}
			if p.Type == "host" {
				ids, err := tool.ParseHostIDs(value)
				if err != nil {
					return nil, invalid(fmt.Sprintf("节点 %q 的执行主机参数校验失败：%s", node.Name, err.Error()))
				}
				if len(ids) == 0 {
					return nil, invalid(fmt.Sprintf("节点 %q 的执行主机参数不能为空", node.Name))
				}
				hostIDs = ids
				continue
			}
			secret := res.Sensitive
			if res.Kind == "input" && inputSensitive[res.InputName] {
				secret = true
			}
			if secret {
				key := fmt.Sprintf("n.%s.%s", node.ID, p.Name)
				sensitive[key] = value
				nodeSensitive[p.Name] = key
				continue
			}
			nodePlain[p.Name] = value
		}
		timeout := node.TimeoutSec
		if timeout == 0 {
			timeout = td.Version.TimeoutSec
		}
		graph.Nodes = append(graph.Nodes, exec.GraphNode{
			ID: node.ID, Type: NodeTool, Name: node.Name,
			ToolID: td.ID, ToolName: td.Name, ToolVersionID: td.Version.ID, ToolVersionNo: td.Version.VersionNo,
			ContentHash: td.Version.ContentHash, ToolType: td.Version.Type,
			Interpreter: td.Version.Interpreter, Content: td.Version.Content,
			PrepareCommand: td.Version.PrepareCommand, Sudo: td.Version.Sudo,
			TimeoutSec: timeout, RetryCount: node.RetryCount, RetryDelaySec: node.RetryDelaySec,
			HostIDs: hostIDs,
			Params:  nodePlain, SensitiveParams: nodeSensitive,
			X: node.X, Y: node.Y,
		})
	}
	for _, e := range detail.Edges {
		graph.Edges = append(graph.Edges, exec.GraphEdge{From: e.From, To: e.To, Condition: e.Condition, Value: e.Value})
	}
	plan := exec.Plan{Kind: "flow", Name: detail.Name, MaxDurationSec: detail.MaxDurationSec, Graph: graph}
	return &CompileResult{Plan: plan, Plain: plain, Sensitive: sensitive}, nil
}
