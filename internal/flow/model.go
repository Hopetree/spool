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

var envNameRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// Mapping maps one tool parameter to a flow parameter or a literal value.
type Mapping struct {
	Source string `json:"source"` // flow | literal
	Value  string `json:"value"`
}

// Step is one tool invocation inside a stage.
type Step struct {
	ID            string             `json:"id"`
	Index         int                `json:"index"`
	Name          string             `json:"name"`
	ToolID        string             `json:"tool_id"`
	ToolName      string             `json:"tool_name"`
	ToolType      string             `json:"tool_type"`
	Mapping       map[string]Mapping `json:"mapping"`
	TimeoutSec    int                `json:"timeout_sec"`
	RetryCount    int                `json:"retry_count"`
	RetryDelaySec int                `json:"retry_delay_sec"`
	Enabled       bool               `json:"enabled"`
}

// Stage groups steps that run in parallel.
type Stage struct {
	ID        string `json:"id"`
	Index     int    `json:"index"`
	Name      string `json:"name"`
	AlwaysRun bool   `json:"always_run"`
	Steps     []Step `json:"steps"`
}

// Flow is the summary view.
type Flow struct {
	ID             string       `json:"id"`
	Name           string       `json:"name"`
	Description    string       `json:"description"`
	Params         []tool.Param `json:"params"`
	FailurePolicy  string       `json:"failure_policy"`
	MaxDurationSec int          `json:"max_duration_sec"`
	Enabled        bool         `json:"enabled"`
	StageCount     int          `json:"stage_count"`
	StepCount      int          `json:"step_count"`
	CreatedAt      int64        `json:"created_at"`
	UpdatedAt      int64        `json:"updated_at"`
}

// Detail is a flow with its stages and steps.
type Detail struct {
	Flow
	Stages []Stage `json:"stages"`
}

// StageInput carries create/update stage fields.
type StageInput struct {
	Name      string
	AlwaysRun bool
	Steps     []StepInput
}

// StepInput carries create/update step fields.
type StepInput struct {
	Name          string
	ToolID        string
	Mapping       map[string]Mapping
	TimeoutSec    int
	RetryCount    int
	RetryDelaySec int
	Enabled       bool
}

// Input carries create/update fields.
type Input struct {
	Name           string
	Description    string
	Params         []tool.Param
	FailurePolicy  string
	MaxDurationSec int
	Stages         []StageInput
}

// ListFilter narrows flow list queries.
type ListFilter struct {
	Keyword  string
	Enabled  *bool
	Page     int
	PageSize int
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
	if in.Params == nil {
		in.Params = []tool.Param{}
	}
	if err := tool.ValidateParams(in.Params); err != nil {
		return invalid(err.Error())
	}
	if in.FailurePolicy == "" {
		in.FailurePolicy = "abort"
	}
	if in.FailurePolicy != "abort" && in.FailurePolicy != "continue" {
		return invalid("失败策略必须是 abort 或 continue")
	}
	if in.MaxDurationSec == 0 {
		in.MaxDurationSec = 3600
	}
	if in.MaxDurationSec < 1 || in.MaxDurationSec > 86400 {
		return invalid("流程最大时长必须在 1-86400 秒之间")
	}
	if len(in.Stages) == 0 {
		return invalid("流程至少需要一个阶段")
	}
	if len(in.Stages) > 20 {
		return invalid("阶段最多 20 个")
	}
	totalSteps := 0
	for si := range in.Stages {
		stage := &in.Stages[si]
		stage.Name = strings.TrimSpace(stage.Name)
		if stage.Name == "" {
			stage.Name = fmt.Sprintf("阶段 %d", si+1)
		}
		if len(stage.Name) > 64 {
			return invalid("阶段名称不能超过 64 个字符")
		}
		if len(stage.Steps) == 0 {
			return invalid(fmt.Sprintf("阶段 %q 至少需要一个步骤", stage.Name))
		}
		if len(stage.Steps) > 20 {
			return invalid(fmt.Sprintf("阶段 %q 的步骤最多 20 个", stage.Name))
		}
		totalSteps += len(stage.Steps)
		for i := range stage.Steps {
			step := &stage.Steps[i]
			step.Name = strings.TrimSpace(step.Name)
			if step.Name == "" {
				return invalid(fmt.Sprintf("阶段 %q 的第 %d 个步骤缺少名称", stage.Name, i+1))
			}
			if len(step.Name) > 64 {
				return invalid("步骤名称不能超过 64 个字符")
			}
			if step.ToolID == "" {
				return invalid(fmt.Sprintf("步骤 %q 未选择工具", step.Name))
			}
			if step.TimeoutSec < 0 || step.TimeoutSec > 86400 {
				return invalid(fmt.Sprintf("步骤 %q 的超时取值范围为 0-86400 秒", step.Name))
			}
			if step.RetryCount < 0 || step.RetryCount > 3 {
				return invalid(fmt.Sprintf("步骤 %q 的重试次数为 0-3", step.Name))
			}
			if step.RetryDelaySec < 0 || step.RetryDelaySec > 300 {
				return invalid(fmt.Sprintf("步骤 %q 的重试间隔为 0-300 秒", step.Name))
			}
			if step.Mapping == nil {
				step.Mapping = map[string]Mapping{}
			}
			for key, m := range step.Mapping {
				switch m.Source {
				case "flow":
					if !envNameRe.MatchString(m.Value) {
						return invalid(fmt.Sprintf("步骤 %q 的参数 %q 引用了非法的流程参数", step.Name, key))
					}
				case "literal":
					if m.Value == "" {
						return invalid(fmt.Sprintf("步骤 %q 的参数 %q 固定值不能为空", step.Name, key))
					}
				default:
					return invalid(fmt.Sprintf("步骤 %q 的参数 %q 映射来源必须是 flow 或 literal", step.Name, key))
				}
			}
		}
	}
	if totalSteps > 50 {
		return invalid("流程步骤最多 50 个")
	}
	return nil
}
