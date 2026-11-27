package exec

// Plan is the immutable execution snapshot compiled at submit time.
type Plan struct {
	Kind           string     `json:"kind"`
	Name           string     `json:"name"`
	FailurePolicy  string     `json:"failure_policy,omitempty"` // abort | continue
	MaxDurationSec int        `json:"max_duration_sec,omitempty"`
	DefaultHostIDs []string   `json:"default_host_ids,omitempty"`
	Stages         []Stage    `json:"stages,omitempty"`
	Graph          *GraphPlan `json:"graph,omitempty"`
	// PerHostGraph preserves the legacy behavior for flows whose nodes do not
	// select hosts: every selected host runs the whole graph independently.
	PerHostGraph bool `json:"per_host_graph,omitempty"`
}

// GraphPlan is a DAG execution plan: nodes run at most once per host, edges
// activate target nodes when their condition matches.
type GraphPlan struct {
	Nodes []GraphNode `json:"nodes"`
	Edges []GraphEdge `json:"edges"`
}

// GraphNode is one start marker or tool invocation in a graph plan.
type GraphNode struct {
	ID              string            `json:"id"`
	Type            string            `json:"type"` // start | tool
	Name            string            `json:"name"`
	ToolID          string            `json:"tool_id,omitempty"`
	ToolName        string            `json:"tool_name,omitempty"`
	ToolVersionID   string            `json:"tool_version_id,omitempty"`
	ToolVersionNo   int               `json:"tool_version_no,omitempty"`
	HostIDs         []string          `json:"host_ids,omitempty"`
	ContentHash     string            `json:"content_hash,omitempty"`
	ToolType        string            `json:"tool_type,omitempty"`
	Interpreter     string            `json:"interpreter,omitempty"`
	Content         string            `json:"content,omitempty"`
	PrepareCommand  string            `json:"prepare_command,omitempty"`
	Sudo            bool              `json:"sudo,omitempty"`
	TimeoutSec      int               `json:"timeout_sec,omitempty"`
	RetryCount      int               `json:"retry_count,omitempty"`
	RetryDelaySec   int               `json:"retry_delay_sec,omitempty"`
	Params          map[string]string `json:"params,omitempty"`
	SensitiveParams map[string]string `json:"sensitive_params,omitempty"`
	// X and Y keep the editor layout so execution history can be drawn.
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// GraphEdge activates To when From's result matches Condition.
type GraphEdge struct {
	From      string `json:"from"`
	To        string `json:"to"`
	Condition string `json:"condition"`
	Value     string `json:"value,omitempty"`
}

// Stage groups steps that run in parallel.
type Stage struct {
	Index     int    `json:"index"`
	Name      string `json:"name"`
	AlwaysRun bool   `json:"always_run"`
	Steps     []Step `json:"steps"`
}

// Step is one script execution on one host.
type Step struct {
	Index          int               `json:"index"`
	Name           string            `json:"name"`
	ToolID         string            `json:"tool_id,omitempty"`
	ToolVersionID  string            `json:"tool_version_id,omitempty"`
	ToolVersionNo  int               `json:"tool_version_no,omitempty"`
	ContentHash    string            `json:"content_hash,omitempty"`
	Type           string            `json:"type"`
	Interpreter    string            `json:"interpreter"`
	Content        string            `json:"content"`
	PrepareCommand string            `json:"prepare_command,omitempty"`
	Sudo           bool              `json:"sudo"`
	TimeoutSec     int               `json:"timeout_sec"`
	RetryCount     int               `json:"retry_count,omitempty"`
	RetryDelaySec  int               `json:"retry_delay_sec,omitempty"`
	Params         map[string]string `json:"params,omitempty"`
	// SensitiveParams maps env var names to keys in the execution's encrypted
	// sensitive value map. Plan JSON never contains the secret values.
	SensitiveParams map[string]string `json:"sensitive_params,omitempty"`
}

// BuildToolPlan turns one tool version into a single-stage plan.
func BuildToolPlan(name string, v ToolVersionSnapshot, params, sensitiveParams map[string]string) Plan {
	return Plan{
		Kind: "tool",
		Name: name,
		Stages: []Stage{{
			Index: 1,
			Name:  "执行",
			Steps: []Step{{
				Index:           1,
				Name:            name,
				ToolID:          v.ToolID,
				ToolVersionID:   v.VersionID,
				ToolVersionNo:   v.VersionNo,
				ContentHash:     v.ContentHash,
				Type:            v.Type,
				Interpreter:     v.Interpreter,
				Content:         v.Content,
				PrepareCommand:  v.PrepareCommand,
				Sudo:            v.Sudo,
				TimeoutSec:      v.TimeoutSec,
				Params:          params,
				SensitiveParams: sensitiveParams,
			}},
		}},
	}
}

// BuildAdhocPlan wraps an inline shell command as a single-step plan.
func BuildAdhocPlan(command string, timeoutSec int, sudo bool) Plan {
	if timeoutSec <= 0 {
		timeoutSec = 300
	}
	return Plan{
		Kind: "adhoc", Name: "临时命令",
		Stages: []Stage{{Index: 1, Name: "执行", Steps: []Step{{
			Index: 1, Name: "临时命令", Type: "shell", Interpreter: "/bin/bash",
			Content: command, TimeoutSec: timeoutSec, Sudo: sudo,
		}}}},
	}
}

// ToolVersionSnapshot is the subset of a tool version needed to build a plan.
type ToolVersionSnapshot struct {
	ToolID         string
	VersionID      string
	VersionNo      int
	ContentHash    string
	Type           string
	Interpreter    string
	Content        string
	PrepareCommand string
	Sudo           bool
	TimeoutSec     int
}
