package flow

import (
	"fmt"
	"strings"

	"github.com/Hopetree/spool/internal/tool"
)

// resolvedParam is the result of following a node parameter through its
// binding and any node references. Input means the value comes from a flow
// execution input; literal/default carry a concrete value.
type resolvedParam struct {
	Kind       string // default | literal | input
	Value      string
	InputName  string
	InputLabel string
	Sensitive  bool
	Required   bool
	RootParam  tool.Param
}

type paramResolver struct {
	nodes      map[string]NodeInput
	toolParams map[string][]tool.Param
	params     map[string]map[string]tool.Param
	ancestors  map[string]map[string]bool
	order      []string
	memo       map[string]resolvedParam
	visiting   map[string]bool
}

func newParamResolver(nodes []NodeInput, edges []EdgeInput, toolsByNode map[string]*tool.Detail) (*paramResolver, error) {
	r := &paramResolver{
		nodes:      make(map[string]NodeInput, len(nodes)),
		toolParams: map[string][]tool.Param{},
		params:     map[string]map[string]tool.Param{},
		ancestors:  map[string]map[string]bool{},
		memo:       map[string]resolvedParam{},
		visiting:   map[string]bool{},
	}
	for _, node := range nodes {
		r.nodes[node.ID] = node
		if node.Type != NodeTool {
			continue
		}
		td := toolsByNode[node.ID]
		if td == nil {
			return nil, invalid(fmt.Sprintf("节点 %q 的工具信息缺失", node.Name))
		}
		byName := make(map[string]tool.Param, len(td.Version.Params))
		for _, p := range td.Version.Params {
			byName[p.Name] = p
		}
		r.toolParams[node.ID] = td.Version.Params
		r.params[node.ID] = byName
	}
	order, err := topologicalOrder(nodes, edges)
	if err != nil {
		return nil, err
	}
	r.order = order
	r.ancestors = buildAncestors(nodes, edges)
	return r, nil
}

func paramKey(nodeID, paramName string) string {
	return nodeID + "\x00" + paramName
}

func (r *paramResolver) resolve(nodeID, paramName string) (resolvedParam, error) {
	key := paramKey(nodeID, paramName)
	if res, ok := r.memo[key]; ok {
		return res, nil
	}
	if r.visiting[key] {
		node := r.nodes[nodeID]
		return resolvedParam{}, invalid(fmt.Sprintf("节点 %q 的参数 %q 存在循环引用", node.Name, paramName))
	}
	r.visiting[key] = true
	defer delete(r.visiting, key)

	node, ok := r.nodes[nodeID]
	if !ok {
		return resolvedParam{}, invalid("引用的节点不存在")
	}
	p, ok := r.params[nodeID][paramName]
	if !ok {
		return resolvedParam{}, invalid(fmt.Sprintf("节点 %q 不存在参数 %q", node.Name, paramName))
	}
	m, has := node.Mapping[paramName]
	if !has || m.Source == "" || m.Source == "default" {
		res := resolvedParam{
			Kind: "default", Value: p.Default, Sensitive: p.Sensitive, Required: p.Required, RootParam: p,
		}
		if err := validateResolvedValue(p, res.Value); err != nil {
			return resolvedParam{}, nodeParamError(node, p, err)
		}
		r.memo[key] = res
		return res, nil
	}

	var res resolvedParam
	switch m.Source {
	case "literal":
		if p.Sensitive {
			return resolvedParam{}, invalid(fmt.Sprintf("节点 %q 的敏感参数 %q 不能使用固定值", node.Name, p.Name))
		}
		value := strings.TrimSpace(m.Value)
		if value == "" {
			return resolvedParam{}, invalid(fmt.Sprintf("节点 %q 的参数 %q 固定值不能为空", node.Name, p.Name))
		}
		if err := validateResolvedValue(p, value); err != nil {
			return resolvedParam{}, nodeParamError(node, p, err)
		}
		res = resolvedParam{
			Kind: "literal", Value: value, Sensitive: p.Sensitive, Required: p.Required, RootParam: p,
		}
	case "input":
		name := strings.TrimSpace(m.InputName)
		if !envNameRe.MatchString(name) {
			return resolvedParam{}, invalid(fmt.Sprintf("节点 %q 的参数 %q 输入名 %q 不合法", node.Name, p.Name, name))
		}
		if strings.HasPrefix(strings.ToUpper(name), "SPOOL_") {
			return resolvedParam{}, invalid(fmt.Sprintf("节点 %q 的参数 %q 输入名不能以 SPOOL_ 开头", node.Name, p.Name))
		}
		res = resolvedParam{
			Kind: "input", InputName: name, InputLabel: strings.TrimSpace(m.InputLabel),
			Sensitive: p.Sensitive || m.InputSensitive, Required: p.Required, RootParam: p,
		}
	case "node":
		target, ok := r.nodes[m.RefNodeID]
		if !ok {
			return resolvedParam{}, invalid(fmt.Sprintf("节点 %q 的参数 %q 引用的节点不存在", node.Name, p.Name))
		}
		if target.Type != NodeTool {
			return resolvedParam{}, invalid(fmt.Sprintf("节点 %q 的参数 %q 只能引用工具节点", node.Name, p.Name))
		}
		if m.RefNodeID == nodeID {
			return resolvedParam{}, invalid(fmt.Sprintf("节点 %q 的参数 %q 不能引用自己", node.Name, p.Name))
		}
		if !r.ancestors[nodeID][m.RefNodeID] {
			return resolvedParam{}, invalid(fmt.Sprintf(
				"节点 %q 的参数 %q 只能引用上游节点，%q 不是上游节点", node.Name, p.Name, target.Name))
		}
		targetParam, ok := r.params[m.RefNodeID][m.RefParam]
		if !ok {
			return resolvedParam{}, invalid(fmt.Sprintf(
				"节点 %q 的参数 %q 引用了节点 %q 不存在的参数 %q", node.Name, p.Name, target.Name, m.RefParam))
		}
		if targetParam.Type != p.Type {
			return resolvedParam{}, invalid(fmt.Sprintf(
				"节点 %q 的参数 %q 不能引用节点 %q 的参数 %q：类型不一致（%s / %s）",
				node.Name, p.Name, target.Name, targetParam.Name, p.Type, targetParam.Type))
		}
		if p.Type == "select" && !optionsCover(p.Options, targetParam.Options) {
			return resolvedParam{}, invalid(fmt.Sprintf(
				"节点 %q 的参数 %q 的候选项未覆盖节点 %q 的参数 %q",
				node.Name, p.Name, target.Name, targetParam.Name))
		}
		targetRes, err := r.resolve(m.RefNodeID, m.RefParam)
		if err != nil {
			return resolvedParam{}, err
		}
		targetSensitive := targetRes.Sensitive
		res = targetRes
		res.Sensitive = res.Sensitive || p.Sensitive
		res.Required = res.Required || p.Required
		if p.Sensitive && res.Kind == "literal" {
			return resolvedParam{}, invalid(fmt.Sprintf(
				"节点 %q 的敏感参数 %q 不能引用固定值来源", node.Name, p.Name))
		}
		if p.Sensitive && res.Kind == "default" && !targetSensitive {
			return resolvedParam{}, invalid(fmt.Sprintf(
				"节点 %q 的敏感参数 %q 不能引用非敏感参数 %q 的默认值",
				node.Name, p.Name, targetParam.Name))
		}
		if res.Kind != "input" {
			if err := validateResolvedValue(p, res.Value); err != nil {
				return resolvedParam{}, nodeParamError(node, p, err)
			}
		}
	default:
		return resolvedParam{}, invalid(fmt.Sprintf("节点 %q 的参数 %q 来源不合法", node.Name, p.Name))
	}
	r.memo[key] = res
	return res, nil
}

func nodeParamError(node NodeInput, p tool.Param, err error) error {
	return invalid(fmt.Sprintf("节点 %q 的参数 %q：%s", node.Name, p.Name, err.Error()))
}

func validateResolvedValue(p tool.Param, value string) error {
	_, err := tool.ValidateValues([]tool.Param{p}, map[string]string{p.Name: value})
	return err
}

func optionsCover(current, target []string) bool {
	if len(target) == 0 {
		return false
	}
	set := make(map[string]bool, len(current))
	for _, o := range current {
		set[o] = true
	}
	for _, o := range target {
		if !set[o] {
			return false
		}
	}
	return true
}

// deriveInputs walks every node parameter, resolves references and returns the
// execution inputs derived from source=input bindings. When enforceUnique is
// set, every source=input binding must use a distinct input name and label.
func deriveInputs(r *paramResolver, enforceUnique bool) ([]tool.Param, error) {
	type inputAccumulator struct {
		name         string
		params       []tool.Param
		labels       []string
		descriptions []string
		defaults     []string
		required     bool
		sensitive    bool
	}
	accs := map[string]*inputAccumulator{}
	order := []string{}
	declaredNames := map[string]string{}
	declaredLabels := map[string]string{}
	for _, nodeID := range r.order {
		if r.nodes[nodeID].Type != NodeTool {
			continue
		}
		for _, p := range r.toolParams[nodeID] {
			res, err := r.resolve(nodeID, p.Name)
			if err != nil {
				return nil, err
			}
			if res.Kind != "input" {
				continue
			}
			if mapping, ok := r.nodes[nodeID].Mapping[p.Name]; ok && mapping.Source == "input" {
				label := firstNonEmpty(res.InputLabel, res.RootParam.Label, res.InputName)
				owner := fmt.Sprintf("节点 %q 的参数 %q", r.nodes[nodeID].Name, p.Name)
				if enforceUnique {
					if prev, exists := declaredNames[res.InputName]; exists {
						return nil, invalid(fmt.Sprintf(
							"流程输入名 %q 重复：%s 与 %s 冲突", res.InputName, prev, owner))
					}
					if prev, exists := declaredLabels[label]; exists {
						return nil, invalid(fmt.Sprintf(
							"流程输入显示标题 %q 重复：%s 与 %s 冲突", label, prev, owner))
					}
				}
				if _, exists := declaredNames[res.InputName]; !exists {
					declaredNames[res.InputName] = owner
				}
				if _, exists := declaredLabels[label]; !exists {
					declaredLabels[label] = owner
				}
			}
			acc := accs[res.InputName]
			if acc == nil {
				acc = &inputAccumulator{name: res.InputName}
				accs[res.InputName] = acc
				order = append(order, res.InputName)
			}
			acc.params = append(acc.params, res.RootParam)
			if label := strings.TrimSpace(res.InputLabel); label != "" {
				acc.labels = append(acc.labels, label)
			}
			if label := strings.TrimSpace(res.RootParam.Label); label != "" {
				acc.labels = append(acc.labels, label)
			}
			if desc := strings.TrimSpace(res.RootParam.Description); desc != "" {
				acc.descriptions = append(acc.descriptions, desc)
			}
			if def := strings.TrimSpace(res.RootParam.Default); def != "" {
				acc.defaults = append(acc.defaults, def)
			}
			acc.required = acc.required || res.Required
			acc.sensitive = acc.sensitive || res.Sensitive
		}
	}

	out := make([]tool.Param, 0, len(order))
	for _, name := range order {
		acc := accs[name]
		base := acc.params[0]
		for _, p := range acc.params[1:] {
			if p.Type != base.Type {
				return nil, invalid(fmt.Sprintf("流程输入 %q 被不同类型的参数使用（%s / %s）", name, base.Type, p.Type))
			}
		}
		base.Name = name
		base.Label = firstNonEmpty(append(acc.labels, base.Label, name)...)
		base.Description = firstNonEmpty(append(acc.descriptions, base.Description)...)
		base.Required = acc.required
		base.Sensitive = acc.sensitive
		base.Options = intersectOptions(base.Type, acc.params)
		if base.Type == "select" && len(base.Options) == 0 {
			return nil, invalid(fmt.Sprintf("流程输入 %q 的候选项没有交集", name))
		}
		base.Default = chooseDefault(base, acc.defaults)
		out = append(out, base)
	}
	if err := tool.ValidateParams(out); err != nil {
		return nil, invalid("流程输入定义不合法：" + err.Error())
	}
	return out, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if v := strings.TrimSpace(value); v != "" {
			return v
		}
	}
	return ""
}

func intersectOptions(paramType string, params []tool.Param) []string {
	if paramType != "select" || len(params) == 0 {
		return nil
	}
	allowed := map[string]bool{}
	for _, opt := range params[0].Options {
		allowed[opt] = true
	}
	for _, p := range params[1:] {
		next := map[string]bool{}
		for _, opt := range p.Options {
			if allowed[opt] {
				next[opt] = true
			}
		}
		allowed = next
	}
	out := make([]string, 0, len(allowed))
	for _, opt := range params[0].Options {
		if allowed[opt] {
			out = append(out, opt)
		}
	}
	return out
}

func chooseDefault(p tool.Param, defaults []string) string {
	unique := []string{}
	seen := map[string]bool{}
	for _, def := range defaults {
		if def == "" || seen[def] {
			continue
		}
		seen[def] = true
		unique = append(unique, def)
	}
	if len(unique) != 1 {
		return ""
	}
	candidate := unique[0]
	if _, err := tool.ValidateValues([]tool.Param{p}, map[string]string{p.Name: candidate}); err != nil {
		return ""
	}
	return candidate
}

func topologicalOrder(nodes []NodeInput, edges []EdgeInput) ([]string, error) {
	index := make(map[string]int, len(nodes))
	for i, node := range nodes {
		index[node.ID] = i
	}
	indegree := make(map[string]int, len(nodes))
	outgoing := make(map[string][]string, len(nodes))
	for _, node := range nodes {
		indegree[node.ID] = 0
	}
	for _, edge := range edges {
		if _, ok := index[edge.From]; !ok {
			continue
		}
		if _, ok := index[edge.To]; !ok {
			continue
		}
		indegree[edge.To]++
		outgoing[edge.From] = append(outgoing[edge.From], edge.To)
	}
	ready := make([]string, 0, len(nodes))
	for _, node := range nodes {
		if indegree[node.ID] == 0 {
			ready = append(ready, node.ID)
		}
	}
	order := make([]string, 0, len(nodes))
	for len(ready) > 0 {
		id := ready[0]
		ready = ready[1:]
		order = append(order, id)
		for _, next := range outgoing[id] {
			indegree[next]--
			if indegree[next] == 0 {
				ready = append(ready, next)
			}
		}
	}
	if len(order) != len(nodes) {
		return nil, invalid("流程图中存在环，无法确定参数顺序")
	}
	return order, nil
}

func buildAncestors(nodes []NodeInput, edges []EdgeInput) map[string]map[string]bool {
	incoming := make(map[string][]string, len(nodes))
	for _, edge := range edges {
		incoming[edge.To] = append(incoming[edge.To], edge.From)
	}
	out := make(map[string]map[string]bool, len(nodes))
	for _, node := range nodes {
		visited := map[string]bool{}
		stack := append([]string{}, incoming[node.ID]...)
		for len(stack) > 0 {
			id := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if visited[id] || id == node.ID {
				continue
			}
			visited[id] = true
			stack = append(stack, incoming[id]...)
		}
		out[node.ID] = visited
	}
	return out
}

// normalizeLegacyInputMappings converts old flow-parameter mappings to input
// bindings. Parameter definition lists are checked in order; the first
// definition for a name wins.
func normalizeLegacyInputMappings(nodes []NodeInput, paramLists ...[]tool.Param) {
	byName := map[string]tool.Param{}
	for _, list := range paramLists {
		for _, p := range list {
			if _, exists := byName[p.Name]; !exists {
				byName[p.Name] = p
			}
		}
	}
	for i := range nodes {
		for key, m := range nodes[i].Mapping {
			if m.Source != "flow" {
				continue
			}
			label := ""
			sensitive := false
			if p, ok := byName[m.Value]; ok {
				label = p.Label
				sensitive = p.Sensitive
			}
			nodes[i].Mapping[key] = Mapping{
				Source: "input", InputName: m.Value, InputLabel: label, InputSensitive: sensitive,
			}
		}
	}
}

// normalizeLegacyNodeMappings converts old flow mappings in a detail response
// and filters stored flow parameters down to inputs that are still referenced.
func normalizeLegacyNodeMappings(detail *Detail) {
	byName := map[string]tool.Param{}
	for _, p := range detail.Params {
		byName[p.Name] = p
	}
	used := map[string]bool{}
	for i := range detail.Nodes {
		node := &detail.Nodes[i]
		for key, m := range node.Mapping {
			switch m.Source {
			case "flow":
				label := ""
				sensitive := false
				if p, ok := byName[m.Value]; ok {
					label = p.Label
					sensitive = p.Sensitive
				}
				node.Mapping[key] = Mapping{
					Source: "input", InputName: m.Value, InputLabel: label, InputSensitive: sensitive,
				}
				used[m.Value] = true
			case "input":
				used[m.InputName] = true
			}
		}
	}
	filtered := make([]tool.Param, 0, len(used))
	for _, p := range detail.Params {
		if used[p.Name] {
			filtered = append(filtered, p)
		}
	}
	detail.Params = filtered
}

func nodeInputsFromDetail(nodes []Node) []NodeInput {
	out := make([]NodeInput, 0, len(nodes))
	for _, n := range nodes {
		out = append(out, NodeInput{
			ID: n.ID, Type: n.Type, Name: n.Name, ToolID: n.ToolID, HostIDs: n.HostIDs, Mapping: n.Mapping,
			TimeoutSec: n.TimeoutSec, RetryCount: n.RetryCount, RetryDelaySec: n.RetryDelaySec, X: n.X, Y: n.Y,
		})
	}
	return out
}

func edgeInputsFromDetail(edges []Edge) []EdgeInput {
	out := make([]EdgeInput, 0, len(edges))
	for _, e := range edges {
		out = append(out, EdgeInput{From: e.From, To: e.To, Condition: e.Condition, Value: e.Value})
	}
	return out
}
