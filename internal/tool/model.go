package tool

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Errors returned by the tool service.
var (
	ErrNotFound          = errors.New("tool not found")
	ErrHasHistory        = errors.New("tool is used by flows or execution history; disable it instead")
	ErrCurrentVersion    = errors.New("cannot delete the current version")
	ErrVersionHasHistory = errors.New("version has execution history")
)

// ValidationError marks invalid input.
type ValidationError struct{ Message string }

func (e *ValidationError) Error() string { return e.Message }

func invalid(msg string) error { return &ValidationError{Message: msg} }

var envNameRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// Param declares one environment-variable input.
type Param struct {
	Name        string   `json:"name"`
	Label       string   `json:"label"`
	Type        string   `json:"type"`
	Required    bool     `json:"required"`
	Default     string   `json:"default"`
	Options     []string `json:"options"`
	Description string   `json:"description"`
	Sensitive   bool     `json:"sensitive"`
}

// Tool is the list/summary view.
type Tool struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Category    string  `json:"category"`
	Description string  `json:"description"`
	Enabled     bool    `json:"enabled"`
	VersionNo   int     `json:"version_no"`
	Type        string  `json:"type"`
	Interpreter string  `json:"interpreter"`
	TimeoutSec  int     `json:"timeout_sec"`
	Sudo        bool    `json:"sudo"`
	ContentHash string  `json:"content_hash"`
	ParamCount  int     `json:"param_count"`
	Params      []Param `json:"params"`
	CreatedAt   int64   `json:"created_at"`
	UpdatedAt   int64   `json:"updated_at"`
}

// Version is an immutable tool revision.
type Version struct {
	ID             string  `json:"id"`
	ToolID         string  `json:"tool_id"`
	VersionNo      int     `json:"version_no"`
	Type           string  `json:"type"`
	Interpreter    string  `json:"interpreter"`
	Content        string  `json:"content"`
	Params         []Param `json:"params"`
	PrepareCommand string  `json:"prepare_command"`
	TimeoutSec     int     `json:"timeout_sec"`
	Sudo           bool    `json:"sudo"`
	ContentHash    string  `json:"content_hash"`
	ChangeNote     string  `json:"change_note"`
	CreatedBy      string  `json:"created_by"`
	CreatedByName  string  `json:"created_by_name"`
	CreatedAt      int64   `json:"created_at"`
}

// Detail is a tool plus its current version.
type Detail struct {
	Tool
	Version Version `json:"version"`
}

// Input carries create/update fields.
type Input struct {
	Name           string
	Category       string
	Description    string
	Type           string
	Interpreter    string
	Content        string
	PrepareCommand string
	Params         []Param
	TimeoutSec     int
	Sudo           bool
	ChangeNote     string
}

// ListFilter narrows tool list queries.
type ListFilter struct {
	Keyword       string
	Category      string
	Uncategorized bool
	Enabled       *bool
	Page          int
	PageSize      int
}

// CategoryCount is one category path with its tool count.
type CategoryCount struct {
	Path  string `json:"path"`
	Count int    `json:"count"`
}

// HashContent returns the sha256 hex digest of a script body.
func HashContent(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}

// ValidateParams checks parameter declarations.
func ValidateParams(params []Param) error {
	if len(params) > 50 {
		return invalid("参数最多 50 个")
	}
	for i := range params {
		if params[i].Options == nil {
			params[i].Options = []string{}
		}
	}
	seen := map[string]bool{}
	for _, p := range params {
		if !envNameRe.MatchString(p.Name) {
			return invalid(fmt.Sprintf("参数名 %q 必须是合法的环境变量名", p.Name))
		}
		if strings.HasPrefix(strings.ToUpper(p.Name), "SPOOL_") {
			return invalid("参数名不能以 SPOOL_ 开头")
		}
		if seen[p.Name] {
			return invalid(fmt.Sprintf("参数名 %q 重复", p.Name))
		}
		seen[p.Name] = true
		switch p.Type {
		case "string", "int", "bool", "select":
		default:
			return invalid(fmt.Sprintf("参数 %q 的类型必须是 string/int/bool/select", p.Name))
		}
		if len(p.Label) > 64 {
			return invalid(fmt.Sprintf("参数 %q 的显示名不能超过 64 个字符", p.Name))
		}
		if len(p.Description) > 200 {
			return invalid(fmt.Sprintf("参数 %q 的说明不能超过 200 个字符", p.Name))
		}
		if p.Type == "select" && len(p.Options) == 0 {
			return invalid(fmt.Sprintf("参数 %q 是 select，必须提供选项", p.Name))
		}
		if p.Default != "" {
			if err := validateValue(p, p.Default); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateValue(p Param, value string) error {
	switch p.Type {
	case "int":
		if _, err := strconv.Atoi(value); err != nil {
			return invalid(fmt.Sprintf("参数 %q 必须是整数", p.Name))
		}
	case "bool":
		if value != "true" && value != "false" {
			return invalid(fmt.Sprintf("参数 %q 必须是 true 或 false", p.Name))
		}
	case "select":
		for _, opt := range p.Options {
			if opt == value {
				return nil
			}
		}
		return invalid(fmt.Sprintf("参数 %q 的值不在候选项中", p.Name))
	}
	return nil
}

// ValidateValues applies defaults, rejects unknown keys and returns the final
// values that will be injected as environment variables.
func ValidateValues(params []Param, values map[string]string) (map[string]string, error) {
	known := make(map[string]bool, len(params))
	out := make(map[string]string, len(params))
	for _, p := range params {
		known[p.Name] = true
		value := values[p.Name]
		if value == "" {
			value = p.Default
		}
		if value == "" {
			if p.Required {
				return nil, invalid(fmt.Sprintf("参数 %q 不能为空", p.Name))
			}
			out[p.Name] = ""
			continue
		}
		if err := validateValue(p, value); err != nil {
			return nil, err
		}
		out[p.Name] = value
	}
	for name := range values {
		if !known[name] {
			return nil, invalid(fmt.Sprintf("未声明的参数 %q", name))
		}
	}
	return out, nil
}

// NormalizeCategory validates and normalizes a slash-separated category path.
// It is exported so other asset types (flows) share the same rules.
func NormalizeCategory(raw string) (string, error) {
	return normalizeCategory(raw)
}

func normalizeCategory(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	parts := strings.Split(raw, "/")
	if len(parts) > 5 {
		return "", invalid("分类最多 5 层")
	}
	cleaned := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			return "", invalid("分类的每一层都不能为空")
		}
		if len(part) > 32 {
			return "", invalid("分类每层不能超过 32 个字符")
		}
		if part == "." || part == ".." {
			return "", invalid("分类不能包含 . 或 ..")
		}
		if strings.ContainsAny(part, "\t\r\n") {
			return "", invalid("分类不能包含控制字符")
		}
		cleaned = append(cleaned, part)
	}
	out := strings.Join(cleaned, "/")
	if len(out) > 128 {
		return "", invalid("分类总长度不能超过 128 个字符")
	}
	return out, nil
}

func validateInput(in *Input) error {
	in.Name = strings.TrimSpace(in.Name)
	category, err := normalizeCategory(in.Category)
	if err != nil {
		return err
	}
	in.Category = category
	if in.Name == "" {
		return invalid("工具名称不能为空")
	}
	if len(in.Name) > 64 {
		return invalid("工具名称不能超过 64 个字符")
	}
	if len(in.Description) > 500 {
		return invalid("描述不能超过 500 个字符")
	}
	if in.Type != "shell" && in.Type != "python" {
		return invalid("类型必须是 shell 或 python")
	}
	in.Interpreter = strings.TrimSpace(in.Interpreter)
	if in.Interpreter == "" {
		if in.Type == "python" {
			in.Interpreter = "python3"
		} else {
			in.Interpreter = "/bin/bash"
		}
	}
	if strings.ContainsAny(in.Interpreter, " \t\r\n") {
		return invalid("解释器必须是单个可执行文件路径")
	}
	if in.Content == "" {
		return invalid("脚本内容不能为空")
	}
	if len(in.Content) > 1024*1024 {
		return invalid("脚本内容不能超过 1MB")
	}
	if len(in.PrepareCommand) > 4000 {
		return invalid("准备命令不能超过 4000 个字符")
	}
	if in.TimeoutSec == 0 {
		in.TimeoutSec = 300
	}
	if in.TimeoutSec < 1 || in.TimeoutSec > 86400 {
		return invalid("超时必须在 1-86400 秒之间")
	}
	if len(in.ChangeNote) > 200 {
		return invalid("变更说明不能超过 200 个字符")
	}
	if in.Params == nil {
		in.Params = []Param{}
	}
	return ValidateParams(in.Params)
}
