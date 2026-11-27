package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/Hopetree/spool/internal/audit"
	"github.com/Hopetree/spool/internal/exec"
	"github.com/Hopetree/spool/internal/tool"
)

type runInput struct {
	Kind       string            `json:"kind"`
	RefID      string            `json:"ref_id"`
	Params     map[string]string `json:"params"`
	HostIDs    []string          `json:"host_ids"`
	Command    string            `json:"command"`
	TimeoutSec int               `json:"timeout_sec"`
	Sudo       bool              `json:"sudo"`
}

type executionDTO struct {
	exec.Execution
	Params        map[string]string `json:"params"`
	Command       string            `json:"command,omitempty"`
	ToolVersionNo int               `json:"tool_version_no,omitempty"`
	HostRuns      []exec.HostRun    `json:"host_runs,omitempty"`
	StepRuns      []exec.StepRun    `json:"step_runs,omitempty"`
	Graph         *executionGraph   `json:"graph,omitempty"`
}

// executionListDTO is the row shape for execution listings. Ad-hoc runs
// expose their command snippet so history panels do not just repeat "临时命令".
type executionListDTO struct {
	exec.Execution
	Command       string `json:"command,omitempty"`
	ToolVersionNo int    `json:"tool_version_no,omitempty"`
}

type executionGraph struct {
	Nodes []executionGraphNode `json:"nodes"`
	Edges []executionGraphEdge `json:"edges"`
}

type executionGraphNode struct {
	ID            string  `json:"id"`
	Type          string  `json:"type"`
	Name          string  `json:"name"`
	ToolID        string  `json:"tool_id,omitempty"`
	ToolName      string  `json:"tool_name,omitempty"`
	ToolType      string  `json:"tool_type,omitempty"`
	ToolVersionNo int     `json:"tool_version_no,omitempty"`
	X             float64 `json:"x"`
	Y             float64 `json:"y"`
}

type executionGraphEdge struct {
	From      string `json:"from"`
	To        string `json:"to"`
	Condition string `json:"condition"`
	Value     string `json:"value,omitempty"`
}

// executionGraphOf extracts the graph snapshot stored with a flow execution.
func executionGraphOf(planJSON string) *executionGraph {
	if planJSON == "" {
		return nil
	}
	var plan exec.Plan
	if err := json.Unmarshal([]byte(planJSON), &plan); err != nil || plan.Graph == nil {
		return nil
	}
	out := &executionGraph{
		Nodes: make([]executionGraphNode, 0, len(plan.Graph.Nodes)),
		Edges: make([]executionGraphEdge, 0, len(plan.Graph.Edges)),
	}
	for _, n := range plan.Graph.Nodes {
		out.Nodes = append(out.Nodes, executionGraphNode{
			ID: n.ID, Type: n.Type, Name: n.Name, ToolID: n.ToolID, ToolName: n.ToolName,
			ToolType: n.ToolType, ToolVersionNo: n.ToolVersionNo, X: n.X, Y: n.Y,
		})
	}
	for _, e := range plan.Graph.Edges {
		out.Edges = append(out.Edges, executionGraphEdge{
			From: e.From, To: e.To, Condition: e.Condition, Value: e.Value,
		})
	}
	return out
}

func executionToolVersion(planJSON, kind string) int {
	if kind != "tool" || planJSON == "" {
		return 0
	}
	var plan exec.Plan
	if err := json.Unmarshal([]byte(planJSON), &plan); err != nil {
		return 0
	}
	for _, stage := range plan.Stages {
		for _, step := range stage.Steps {
			if step.ToolVersionNo > 0 {
				return step.ToolVersionNo
			}
		}
	}
	if plan.Graph != nil {
		for _, node := range plan.Graph.Nodes {
			if node.Type == "tool" && node.ToolVersionNo > 0 {
				return node.ToolVersionNo
			}
		}
	}
	return 0
}

func splitSensitive(params []tool.Param, values map[string]string) (plain, sensitive map[string]string) {
	plain = map[string]string{}
	sensitive = map[string]string{}
	for _, p := range params {
		v, ok := values[p.Name]
		if !ok {
			continue
		}
		if p.Sensitive {
			sensitive[p.Name] = v
		} else {
			plain[p.Name] = v
		}
	}
	return plain, sensitive
}

func toolSnapshot(detail *tool.Detail) exec.ToolVersionSnapshot {
	return exec.ToolVersionSnapshot{
		ToolID:         detail.ID,
		VersionID:      detail.Version.ID,
		VersionNo:      detail.Version.VersionNo,
		ContentHash:    detail.Version.ContentHash,
		Type:           detail.Version.Type,
		Interpreter:    detail.Version.Interpreter,
		Content:        detail.Version.Content,
		PrepareCommand: detail.Version.PrepareCommand,
		Sudo:           detail.Version.Sudo,
		TimeoutSec:     detail.Version.TimeoutSec,
	}
}

func listExecutionsHandler(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
		pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
		if page < 1 {
			page = 1
		}
		if pageSize < 1 || pageSize > 100 {
			pageSize = 20
		}
		where := []string{"1=1"}
		args := []any{}
		if v := c.Query("kind"); v != "" {
			where = append(where, "e.kind = ?")
			args = append(args, v)
		}
		if v := c.Query("status"); v != "" {
			where = append(where, "e.status = ?")
			args = append(args, v)
		}
		if v := c.Query("ref_id"); v != "" {
			where = append(where, "e.ref_id = ?")
			args = append(args, v)
		}
		if v := c.Query("schedule_id"); v != "" {
			where = append(where, "e.schedule_id = ?")
			args = append(args, v)
		}
		if v := c.Query("scheduled"); v == "true" || v == "1" {
			where = append(where, "e.schedule_id <> ''")
		}
		if v := c.Query("created_by"); v != "" {
			where = append(where, "e.created_by = ?")
			args = append(args, v)
		}
		if v := c.Query("host_id"); v != "" {
			where = append(where, "EXISTS (SELECT 1 FROM host_runs h WHERE h.execution_id = e.id AND h.host_id = ?)")
			args = append(args, v)
		}
		if v := c.Query("tool_id"); v != "" {
			where = append(where, "EXISTS (SELECT 1 FROM step_runs s JOIN host_runs h ON h.id = s.host_run_id WHERE h.execution_id = e.id AND s.tool_id = ?)")
			args = append(args, v)
		}
		if v := c.Query("from"); v != "" {
			if ms, err := strconv.ParseInt(v, 10, 64); err == nil {
				where = append(where, "e.created_at >= ?")
				args = append(args, ms)
			}
		}
		if v := c.Query("to"); v != "" {
			if ms, err := strconv.ParseInt(v, 10, 64); err == nil {
				where = append(where, "e.created_at <= ?")
				args = append(args, ms)
			}
		}
		if v := strings.TrimSpace(c.Query("keyword")); v != "" {
			like := "%" + v + "%"
			// Ad-hoc executions all share the same name; search their command
			// text as well so the history search is useful.
			where = append(where, "(e.name_snapshot LIKE ? OR (e.kind = 'adhoc' AND e.params_json LIKE ?))")
			args = append(args, like, like)
		}
		cond := strings.Join(where, " AND ")
		var total int64
		if err := d.Store.DB.GetContext(c.Request.Context(), &total, `SELECT count(*) FROM executions e WHERE `+cond, args...); err != nil {
			fail(c, http.StatusInternalServerError, "internal_error", "查询失败", nil)
			return
		}
		pageArgs := append([]any{}, args...)
		pageArgs = append(pageArgs, pageSize, (page-1)*pageSize)
		rows := []exec.Execution{}
		if err := d.Store.DB.SelectContext(c.Request.Context(), &rows,
			`SELECT e.*, COALESCE(u.username, NULLIF(u.display_name, ''),
				(SELECT a.username_snapshot FROM audit_logs a WHERE a.resource_id = e.id
				 AND a.action IN ('execution.start', 'execution.rerun') AND a.username_snapshot <> ''
				 ORDER BY a.ts DESC LIMIT 1),
				(SELECT a.username_snapshot FROM audit_logs a WHERE a.action = 'schedule.trigger'
				 AND a.summary_json LIKE '%"execution_id":"' || e.id || '"%' AND a.username_snapshot <> ''
				 ORDER BY a.ts DESC LIMIT 1), e.created_by) AS created_by_name
			FROM executions e LEFT JOIN users u ON u.id = e.created_by
			WHERE `+cond+` ORDER BY e.created_at DESC LIMIT ? OFFSET ?`, pageArgs...); err != nil {
			fail(c, http.StatusInternalServerError, "internal_error", "查询失败", nil)
			return
		}
		items := make([]executionListDTO, 0, len(rows))
		for _, row := range rows {
			item := executionListDTO{Execution: row, ToolVersionNo: executionToolVersion(row.PlanJSON, row.Kind)}
			if row.Kind == "adhoc" {
				params := map[string]string{}
				_ = json.Unmarshal([]byte(row.ParamsJSON), &params)
				item.Command = params["command"]
			}
			items = append(items, item)
		}
		ok(c, pageResponse{Items: items, Total: total, Page: page, PageSize: pageSize})
	}
}

func getExecutionHandler(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		ex, err := d.Engine.GetExecution(ctx, c.Param("id"))
		if err != nil {
			if errors.Is(err, exec.ErrNotFound) {
				fail(c, http.StatusNotFound, "not_found", "执行记录不存在", nil)
				return
			}
			fail(c, http.StatusInternalServerError, "internal_error", "查询失败", nil)
			return
		}
		hostRuns, _ := d.Engine.HostRuns(ctx, ex.ID)
		steps, _ := d.Engine.StepRuns(ctx, ex.ID)
		params := map[string]string{}
		_ = json.Unmarshal([]byte(ex.ParamsJSON), &params)
		command := ""
		if ex.Kind == "adhoc" {
			command = params["command"]
			delete(params, "command")
		}
		toolVersionNo := executionToolVersion(ex.PlanJSON, ex.Kind)
		if toolVersionNo == 0 && ex.Kind == "tool" {
			for _, step := range steps {
				if step.ToolVersionNo > 0 {
					toolVersionNo = step.ToolVersionNo
					break
				}
			}
		}
		ok(c, executionDTO{
			Execution: *ex, Params: params, Command: command, ToolVersionNo: toolVersionNo,
			HostRuns: hostRuns, StepRuns: steps, Graph: executionGraphOf(ex.PlanJSON),
		})
	}
}

func createExecutionHandler(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		var in runInput
		if err := c.ShouldBindJSON(&in); err != nil {
			fail(c, http.StatusBadRequest, "validation_failed", "请求格式不正确", nil)
			return
		}
		if len(in.HostIDs) == 0 || (in.Kind != "adhoc" && in.RefID == "") {
			fail(c, http.StatusBadRequest, "validation_failed", "必须选择执行对象和主机", nil)
			return
		}
		ctx := c.Request.Context()
		var (
			name      string
			refID     string
			plan      exec.Plan
			plain     map[string]string
			sensitive map[string]string
		)
		switch in.Kind {
		case "tool":
			detail, err := d.Tools.Get(ctx, in.RefID)
			if err != nil {
				writeToolError(c, err)
				return
			}
			if !detail.Enabled {
				fail(c, http.StatusConflict, "tool_disabled", "工具已停用，不能执行", nil)
				return
			}
			values, err := tool.ValidateValues(detail.Version.Params, in.Params)
			if err != nil {
				msg := "参数不正确"
				var ve *tool.ValidationError
				if errors.As(err, &ve) {
					msg = ve.Message
				}
				fail(c, http.StatusBadRequest, "validation_failed", msg, nil)
				return
			}
			plain, sensitive = splitSensitive(detail.Version.Params, values)
			sensitiveRefs := make(map[string]string, len(sensitive))
			for k := range sensitive {
				sensitiveRefs[k] = k
			}
			plan = exec.BuildToolPlan(detail.Name, toolSnapshot(detail), plain, sensitiveRefs)
			name, refID = detail.Name, detail.ID
		case "flow":
			detail, err := d.Flows.Get(ctx, in.RefID)
			if err != nil {
				writeFlowError(c, err)
				return
			}
			if !detail.Enabled {
				fail(c, http.StatusConflict, "flow_disabled", "流程已停用，不能执行", nil)
				return
			}
			res, err := d.Flows.Compile(ctx, in.RefID, in.Params)
			if err != nil {
				writeFlowError(c, err)
				return
			}
			plan, plain, sensitive = res.Plan, res.Plain, res.Sensitive
			name, refID = detail.Name, detail.ID
		case "adhoc":
			command := strings.TrimSpace(in.Command)
			if command == "" {
				fail(c, http.StatusBadRequest, "validation_failed", "命令不能为空", nil)
				return
			}
			if len(command) > 20000 {
				fail(c, http.StatusBadRequest, "validation_failed", "命令不能超过 20000 个字符", nil)
				return
			}
			timeout := in.TimeoutSec
			if timeout == 0 {
				timeout = 300
			}
			if timeout < 1 || timeout > 86400 {
				fail(c, http.StatusBadRequest, "validation_failed", "超时必须在 1-86400 秒之间", nil)
				return
			}
			plan = exec.BuildAdhocPlan(command, timeout, in.Sudo)
			plain = map[string]string{
				"command": command, "timeout_sec": strconv.Itoa(timeout), "sudo": strconv.FormatBool(in.Sudo),
			}
			sensitive = map[string]string{}
			name, refID = "临时命令", ""
		default:
			fail(c, http.StatusBadRequest, "unsupported_kind", "仅支持执行工具或流程", nil)
			return
		}
		u := currentUser(c)
		ex, err := d.Engine.Submit(ctx, exec.SubmitRequest{
			Kind: in.Kind, Source: "manual", RefID: refID, Name: name,
			Plan: plan, Params: plain, Sensitive: sensitive, HostIDs: in.HostIDs, ActorID: u.ID,
		})
		if err != nil {
			if errors.Is(err, exec.ErrNoHosts) {
				fail(c, http.StatusBadRequest, "no_hosts", "所选主机不可用或已停用", nil)
				return
			}
			fail(c, http.StatusInternalServerError, "internal_error", "提交执行失败", nil)
			return
		}
		summary := map[string]any{"kind": in.Kind, "ref_id": refID, "hosts": ex.HostTotal}
		if in.Kind == "adhoc" {
			preview := in.Command
			if len(preview) > 200 {
				preview = preview[:200]
			}
			summary["command"] = preview
		}
		writeAudit(c, d, audit.Entry{
			ActorID: u.ID, ActorName: u.Username, Action: "execution.start",
			ResourceType: "execution", ResourceID: ex.ID, ResourceName: ex.NameSnapshot,
			Summary: summary,
		})
		c.JSON(http.StatusCreated, ex)
	}
}

func cancelExecutionHandler(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		u := currentUser(c)
		if err := d.Engine.Cancel(c.Request.Context(), c.Param("id")); err != nil {
			if errors.Is(err, exec.ErrNotFound) {
				fail(c, http.StatusNotFound, "not_found", "执行记录不存在", nil)
				return
			}
			fail(c, http.StatusInternalServerError, "internal_error", "取消失败", nil)
			return
		}
		writeAudit(c, d, audit.Entry{
			ActorID: u.ID, ActorName: u.Username, Action: "execution.cancel",
			ResourceType: "execution", ResourceID: c.Param("id"),
		})
		c.Status(http.StatusNoContent)
	}
}

func cancelHostRunHandler(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		var n int
		if err := d.Store.DB.GetContext(c.Request.Context(), &n,
			`SELECT count(*) FROM host_runs WHERE id = ? AND execution_id = ?`, c.Param("hostRunId"), c.Param("id")); err != nil || n == 0 {
			fail(c, http.StatusNotFound, "not_found", "主机执行记录不存在", nil)
			return
		}
		if err := d.Engine.CancelHostRun(c.Request.Context(), c.Param("hostRunId")); err != nil {
			fail(c, http.StatusInternalServerError, "internal_error", "取消失败", nil)
			return
		}
		u := currentUser(c)
		writeAudit(c, d, audit.Entry{
			ActorID: u.ID, ActorName: u.Username, Action: "execution.cancel_host",
			ResourceType: "execution", ResourceID: c.Param("id"),
			Summary: map[string]any{"host_run": c.Param("hostRunId")},
		})
		c.Status(http.StatusNoContent)
	}
}

func rerunExecutionHandler(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		old, err := d.Engine.GetExecution(ctx, c.Param("id"))
		if err != nil {
			fail(c, http.StatusNotFound, "not_found", "执行记录不存在", nil)
			return
		}
		stored := map[string]string{}
		_ = json.Unmarshal([]byte(old.ParamsJSON), &stored)
		sensitiveOld, err := d.Engine.DecryptSensitive(old)
		if err != nil {
			fail(c, http.StatusInternalServerError, "master_key_mismatch", "无法解密原执行参数", nil)
			return
		}
		var (
			name      string
			plan      exec.Plan
			plain     map[string]string
			sensitive map[string]string
		)
		switch old.Kind {
		case "tool":
			detail, err := d.Tools.Get(ctx, old.RefID)
			if err != nil {
				writeToolError(c, err)
				return
			}
			if !detail.Enabled {
				fail(c, http.StatusConflict, "tool_disabled", "工具已停用，不能执行", nil)
				return
			}
			for k, v := range sensitiveOld {
				stored[k] = v
			}
			values, err := tool.ValidateValues(detail.Version.Params, stored)
			if err != nil {
				fail(c, http.StatusBadRequest, "validation_failed", "原参数已不适用于当前工具版本", nil)
				return
			}
			plain, sensitive = splitSensitive(detail.Version.Params, values)
			sensitiveRefs := make(map[string]string, len(sensitive))
			for k := range sensitive {
				sensitiveRefs[k] = k
			}
			plan = exec.BuildToolPlan(detail.Name, toolSnapshot(detail), plain, sensitiveRefs)
			name = detail.Name
		case "adhoc":
			command := stored["command"]
			if command == "" {
				fail(c, http.StatusBadRequest, "validation_failed", "原执行没有可重跑的命令", nil)
				return
			}
			timeout, _ := strconv.Atoi(stored["timeout_sec"])
			sudo := stored["sudo"] == "true"
			plan = exec.BuildAdhocPlan(command, timeout, sudo)
			plain = map[string]string{
				"command": command, "timeout_sec": strconv.Itoa(timeout), "sudo": strconv.FormatBool(sudo),
			}
			sensitive = map[string]string{}
			name = "临时命令"
		case "flow":
			detail, err := d.Flows.Get(ctx, old.RefID)
			if err != nil {
				writeFlowError(c, err)
				return
			}
			if !detail.Enabled {
				fail(c, http.StatusConflict, "flow_disabled", "流程已停用，不能执行", nil)
				return
			}
			allowed := map[string]bool{}
			for _, p := range detail.Params {
				allowed[p.Name] = true
			}
			for k, v := range sensitiveOld {
				if allowed[k] {
					stored[k] = v
				}
			}
			res, err := d.Flows.Compile(ctx, detail.ID, stored)
			if err != nil {
				writeFlowError(c, err)
				return
			}
			plan, plain, sensitive = res.Plan, res.Plain, res.Sensitive
			name = detail.Name
		default:
			fail(c, http.StatusBadRequest, "unsupported_kind", "当前执行类型不支持重跑", nil)
			return
		}
		hosts, _ := d.Engine.HostRuns(ctx, old.ID)
		hostIDs := make([]string, 0, len(hosts))
		for _, h := range hosts {
			if h.HostID != "" {
				hostIDs = append(hostIDs, h.HostID)
			}
		}
		u := currentUser(c)
		ex, err := d.Engine.Submit(ctx, exec.SubmitRequest{
			Kind: old.Kind, Source: "manual", RefID: old.RefID, Name: name,
			Plan: plan, Params: plain, Sensitive: sensitive, HostIDs: hostIDs, ActorID: u.ID,
		})
		if err != nil {
			fail(c, http.StatusInternalServerError, "internal_error", "提交执行失败", nil)
			return
		}
		writeAudit(c, d, audit.Entry{
			ActorID: u.ID, ActorName: u.Username, Action: "execution.rerun",
			ResourceType: "execution", ResourceID: ex.ID, ResourceName: ex.NameSnapshot,
			Summary: map[string]any{"source_execution": old.ID, "kind": old.Kind, "hosts": len(hostIDs)},
		})
		c.JSON(http.StatusCreated, ex)
	}
}

func executionEventsHandler(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		ex, err := d.Engine.GetExecution(c.Request.Context(), c.Param("id"))
		if err != nil {
			fail(c, http.StatusNotFound, "not_found", "执行记录不存在", nil)
			return
		}
		after, _ := strconv.ParseInt(c.DefaultQuery("after_seq", "0"), 10, 64)
		limit, _ := strconv.Atoi(c.DefaultQuery("limit", "500"))
		events, err := exec.ReadEvents(d.Engine.LogPath(ex.ID), after, limit)
		if err != nil {
			fail(c, http.StatusInternalServerError, "internal_error", "读取日志失败", nil)
			return
		}
		hostRunID := c.Query("host_run")
		stepRunID := c.Query("step_run")
		if hostRunID != "" || stepRunID != "" {
			filtered := make([]exec.Event, 0, len(events))
			for _, ev := range events {
				if ev.Type == "log" {
					if hostRunID != "" && ev.HostRunID != hostRunID {
						continue
					}
					if stepRunID != "" && ev.StepRunID != stepRunID {
						continue
					}
				}
				filtered = append(filtered, ev)
			}
			events = filtered
		}
		ok(c, gin.H{"items": events, "status": ex.Status})
	}
}

func streamExecutionHandler(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		ex, err := d.Engine.GetExecution(c.Request.Context(), c.Param("id"))
		if err != nil {
			fail(c, http.StatusNotFound, "not_found", "执行记录不存在", nil)
			return
		}
		after := int64(0)
		if v := c.GetHeader("Last-Event-ID"); v != "" {
			after, _ = strconv.ParseInt(v, 10, 64)
		}
		if v := c.Query("after_seq"); v != "" {
			after, _ = strconv.ParseInt(v, 10, 64)
		}
		ch, unsubscribe := d.Engine.Hub().Subscribe(ex.ID)
		defer unsubscribe()

		events, err := exec.ReadEvents(d.Engine.LogPath(ex.ID), after, 0)
		if err != nil {
			fail(c, http.StatusInternalServerError, "internal_error", "读取日志失败", nil)
			return
		}

		c.Header("Content-Type", "text/event-stream")
		c.Header("Cache-Control", "no-cache")
		c.Header("Connection", "keep-alive")
		c.Header("X-Accel-Buffering", "no")

		filterHost := c.Query("host_run")
		filterStep := c.Query("step_run")
		last := after
		write := func(ev exec.Event) {
			if ev.Seq <= last {
				return
			}
			if ev.Type == "log" {
				if filterHost != "" && ev.HostRunID != filterHost {
					return
				}
				if filterStep != "" && ev.StepRunID != filterStep {
					return
				}
			}
			b, err := json.Marshal(ev)
			if err != nil {
				return
			}
			last = ev.Seq
			fmt.Fprintf(c.Writer, "id: %d\nevent: %s\ndata: %s\n\n", ev.Seq, ev.Type, b)
		}
		for _, ev := range events {
			write(ev)
		}
		c.Writer.Flush()
		if isTerminalStatus(ex.Status) {
			return
		}

		heartbeat := time.NewTicker(20 * time.Second)
		defer heartbeat.Stop()
		for {
			select {
			case ev, ok := <-ch:
				if !ok {
					return
				}
				write(ev)
				c.Writer.Flush()
			case <-heartbeat.C:
				fmt.Fprint(c.Writer, ": ping\n\n")
				c.Writer.Flush()
			case <-c.Request.Context().Done():
				return
			}
		}
	}
}

func isTerminalStatus(status string) bool {
	switch status {
	case "success", "partial", "failed", "cancelled", "interrupted", "skipped":
		return true
	}
	return false
}

func downloadExecutionLogHandler(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		ex, err := d.Engine.GetExecution(c.Request.Context(), c.Param("id"))
		if err != nil {
			fail(c, http.StatusNotFound, "not_found", "执行记录不存在", nil)
			return
		}
		hostRunID := c.Query("host_run")
		stepRunID := c.Query("step_run")
		events, err := exec.ReadEvents(d.Engine.LogPath(ex.ID), 0, 0)
		if err != nil {
			fail(c, http.StatusInternalServerError, "internal_error", "读取日志失败", nil)
			return
		}
		text := exec.RenderText(events, hostRunID, stepRunID)
		name := strings.NewReplacer(" ", "_", "/", "_", "\\", "_").Replace(ex.NameSnapshot)
		c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s-%s.log"`, name, ex.ID))
		c.Data(http.StatusOK, "text/plain; charset=utf-8", []byte(text))
		u := currentUser(c)
		writeAudit(c, d, audit.Entry{
			ActorID: u.ID, ActorName: u.Username, Action: "execution.download",
			ResourceType: "execution", ResourceID: ex.ID, ResourceName: ex.NameSnapshot,
			Summary: map[string]any{"host_run": hostRunID, "step_run": stepRunID},
		})
	}
}
