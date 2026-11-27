package flow

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/Hopetree/spool/internal/tool"
)

// Errors returned by the flow service.
var (
	ErrNotFound = errors.New("flow not found")
	ErrDisabled = errors.New("flow is disabled")
)

// ValidationError marks invalid input.
type ValidationError struct{ Message string }

func (e *ValidationError) Error() string { return e.Message }

func invalid(msg string) error { return &ValidationError{Message: msg} }

// Node types and edge conditions.
const (
	NodeStart = "start"
	NodeTool  = "tool"

	CondAlways            = "always"
	CondSuccess           = "success"
	CondFailure           = "failure"
	CondOutputContains    = "output_contains"
	CondOutputNotContains = "output_not_contains"
)

var (
	nodeIDRe  = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)
	envNameRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
)

// Mapping describes where a node parameter value comes from.
type Mapping struct {
	Source         string `json:"source"`                    // literal | input | node
	Value          string `json:"value,omitempty"`           // literal value
	InputName      string `json:"input_name,omitempty"`      // execution input name
	InputLabel     string `json:"input_label,omitempty"`     // execution input display label
	InputSensitive bool   `json:"input_sensitive,omitempty"` // legacy flow parameter sensitivity
	RefNodeID      string `json:"ref_node_id,omitempty"`     // referenced upstream node
	RefParam       string `json:"ref_param,omitempty"`       // referenced parameter name
}

// Node is a start marker or a tool invocation in the flow graph.
type Node struct {
	ID            string             `json:"id"`
	Type          string             `json:"type"` // start | tool
	Name          string             `json:"name"`
	ToolID        string             `json:"tool_id"`
	ToolName      string             `json:"tool_name"`
	ToolType      string             `json:"tool_type"`
	Mapping       map[string]Mapping `json:"mapping"`
	TimeoutSec    int                `json:"timeout_sec"`
	RetryCount    int                `json:"retry_count"`
	RetryDelaySec int                `json:"retry_delay_sec"`
	X             float64            `json:"x"`
	Y             float64            `json:"y"`
}

// Edge connects two nodes with an activation condition.
type Edge struct {
	ID        string `json:"id"`
	From      string `json:"from"`
	To        string `json:"to"`
	Condition string `json:"condition"`
	Value     string `json:"value"`
}

// Flow is the summary view.
type Flow struct {
	ID             string       `json:"id"`
	Name           string       `json:"name"`
	Category       string       `json:"category"`
	Description    string       `json:"description"`
	Params         []tool.Param `json:"params"`
	MaxDurationSec int          `json:"max_duration_sec"`
	Enabled        bool         `json:"enabled"`
	NodeCount      int          `json:"node_count"`
	EdgeCount      int          `json:"edge_count"`
	CreatedAt      int64        `json:"created_at"`
	UpdatedAt      int64        `json:"updated_at"`
}

// Detail is a flow with its graph.
type Detail struct {
	Flow
	Nodes []Node `json:"nodes"`
	Edges []Edge `json:"edges"`
}

// NodeInput carries create/update node fields.
type NodeInput struct {
	ID            string
	Type          string
	Name          string
	ToolID        string
	Mapping       map[string]Mapping
	TimeoutSec    int
	RetryCount    int
	RetryDelaySec int
	X             float64
	Y             float64
}

// EdgeInput carries create/update edge fields.
type EdgeInput struct {
	From      string
	To        string
	Condition string
	Value     string
}

// Input carries create/update fields.
type Input struct {
	Name           string
	Category       string
	Description    string
	Params         []tool.Param
	MaxDurationSec int
	Nodes          []NodeInput
	Edges          []EdgeInput
}

// ListFilter narrows flow list queries.
type ListFilter struct {
	Keyword       string
	Category      string
	Uncategorized bool
	Enabled       *bool
	Page          int
	PageSize      int
}

func validCondition(cond string) bool {
	switch cond {
	case CondAlways, CondSuccess, CondFailure, CondOutputContains, CondOutputNotContains:
		return true
	}
	return false
}

func validateInput(in *Input) error {
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		return invalid("流程名称不能为空")
	}
	if len(in.Name) > 64 {
		return invalid("流程名称不能超过 64 个字符")
	}
	if len(in.Description) > 500 {
		return invalid("描述不能超过 500 个字符")
	}
	category, err := tool.NormalizeCategory(in.Category)
	if err != nil {
		return invalid(err.Error())
	}
	in.Category = category
	if in.Params == nil {
		in.Params = []tool.Param{}
	}
	if in.MaxDurationSec == 0 {
		in.MaxDurationSec = 3600
	}
	if in.MaxDurationSec < 1 || in.MaxDurationSec > 86400 {
		return invalid("流程最大时长必须在 1-86400 秒之间")
	}
	if len(in.Nodes) == 0 {
		return invalid("流程至少需要一个节点")
	}
	if len(in.Nodes) > 50 {
		return invalid("节点最多 50 个")
	}
	if len(in.Edges) > 100 {
		return invalid("连线最多 100 条")
	}

	ids := map[string]NodeInput{}
	startCount := 0
	toolCount := 0
	for i := range in.Nodes {
		n := &in.Nodes[i]
		n.ID = strings.TrimSpace(n.ID)
		if !nodeIDRe.MatchString(n.ID) {
			return invalid("节点 ID 必须是 1-64 位字母、数字、下划线或短横线")
		}
		if _, exists := ids[n.ID]; exists {
			return invalid(fmt.Sprintf("节点 ID %q 重复", n.ID))
		}
		ids[n.ID] = *n
		switch n.Type {
		case NodeStart:
			startCount++
			if n.ID != NodeStart {
				return invalid("开始节点的 ID 必须是 start")
			}
			if n.ToolID != "" {
				return invalid("开始节点不能绑定工具")
			}
		case NodeTool:
			toolCount++
			n.Name = strings.TrimSpace(n.Name)
			if n.Name == "" {
				return invalid("工具节点必须有名称")
			}
			if len(n.Name) > 64 {
				return invalid("节点名称不能超过 64 个字符")
			}
			if n.ToolID == "" {
				return invalid(fmt.Sprintf("节点 %q 未选择工具", n.Name))
			}
			if n.TimeoutSec < 0 || n.TimeoutSec > 86400 {
				return invalid(fmt.Sprintf("节点 %q 的超时取值范围为 0-86400 秒", n.Name))
			}
			if n.RetryCount < 0 || n.RetryCount > 3 {
				return invalid(fmt.Sprintf("节点 %q 的重试次数为 0-3", n.Name))
			}
			if n.RetryDelaySec < 0 || n.RetryDelaySec > 300 {
				return invalid(fmt.Sprintf("节点 %q 的重试间隔为 0-300 秒", n.Name))
			}
			if n.Mapping == nil {
				n.Mapping = map[string]Mapping{}
			}
			for key, m := range n.Mapping {
				switch m.Source {
				case "literal":
					m.Value = strings.TrimSpace(m.Value)
					if m.Value == "" {
						return invalid(fmt.Sprintf("节点 %q 的参数 %q 固定值不能为空", n.Name, key))
					}
				case "input":
					m.InputName = strings.TrimSpace(m.InputName)
					m.InputLabel = strings.TrimSpace(m.InputLabel)
					if !envNameRe.MatchString(m.InputName) {
						return invalid(fmt.Sprintf("节点 %q 的参数 %q 输入名不合法", n.Name, key))
					}
					if strings.HasPrefix(strings.ToUpper(m.InputName), "SPOOL_") {
						return invalid(fmt.Sprintf("节点 %q 的参数 %q 输入名不能以 SPOOL_ 开头", n.Name, key))
					}
				case "node":
					m.RefNodeID = strings.TrimSpace(m.RefNodeID)
					m.RefParam = strings.TrimSpace(m.RefParam)
					if m.RefNodeID == "" || m.RefParam == "" {
						return invalid(fmt.Sprintf("节点 %q 的参数 %q 未选择引用节点或参数", n.Name, key))
					}
					if m.RefNodeID == n.ID {
						return invalid(fmt.Sprintf("节点 %q 的参数 %q 不能引用自己", n.Name, key))
					}
				default:
					return invalid(fmt.Sprintf("节点 %q 的参数 %q 来源必须是 literal、input 或 node", n.Name, key))
				}
				n.Mapping[key] = m
			}
		default:
			return invalid("节点类型必须是 start 或 tool")
		}
	}
	if startCount != 1 {
		return invalid("流程必须有且只有一个开始节点")
	}
	if toolCount == 0 {
		return invalid("流程至少需要一个工具节点")
	}

	outgoing := map[string][]EdgeInput{}
	seenPairs := map[string]bool{}
	for _, e := range in.Edges {
		if _, ok := ids[e.From]; !ok {
			return invalid(fmt.Sprintf("连线的起点 %q 不存在", e.From))
		}
		if _, ok := ids[e.To]; !ok {
			return invalid(fmt.Sprintf("连线的终点 %q 不存在", e.To))
		}
		if e.From == e.To {
			return invalid("连线不能指向自己")
		}
		if !validCondition(e.Condition) {
			return invalid("连线条件必须是 always/success/failure/output_contains/output_not_contains")
		}
		if (e.Condition == CondOutputContains || e.Condition == CondOutputNotContains) && e.Value == "" {
			return invalid("输出条件必须填写匹配文本")
		}
		key := e.From + "\x00" + e.To
		if seenPairs[key] {
			return invalid(fmt.Sprintf("节点 %q 与 %q 之间存在重复连线", e.From, e.To))
		}
		seenPairs[key] = true
		outgoing[e.From] = append(outgoing[e.From], e)
	}
	if len(outgoing[NodeStart]) == 0 {
		return invalid("开始节点至少要有一条出边")
	}

	// Reachability from start.
	visited := map[string]bool{NodeStart: true}
	queue := []string{NodeStart}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, e := range outgoing[cur] {
			if !visited[e.To] {
				visited[e.To] = true
				queue = append(queue, e.To)
			}
		}
	}
	for id, n := range ids {
		if !visited[id] {
			return invalid(fmt.Sprintf("节点 %q 无法从开始节点到达", n.Name))
		}
	}

	// Cycle detection (DFS colors).
	state := map[string]int{} // 0 unvisited, 1 visiting, 2 done
	var dfs func(string) error
	dfs = func(id string) error {
		switch state[id] {
		case 1:
			return invalid("流程图中存在环，请调整连线")
		case 2:
			return nil
		}
		state[id] = 1
		for _, e := range outgoing[id] {
			if err := dfs(e.To); err != nil {
				return err
			}
		}
		state[id] = 2
		return nil
	}
	if err := dfs(NodeStart); err != nil {
		return err
	}
	return nil
}
