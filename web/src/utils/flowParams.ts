import type { FlowMapping } from "../api/flows";
import type { ParamType, ToolParam, ToolSummary } from "../api/tools";

export interface ParamFlowNode {
  id: string;
  kind: "start" | "tool";
  name: string;
  toolID: string;
  mapping: Record<string, FlowMapping>;
}

export interface ParamEdge {
  from: string;
  to: string;
}

export interface ResolvedBinding {
  kind: "default" | "literal" | "input";
  value?: string;
  inputName?: string;
  inputLabel?: string;
  rootParam?: ToolParam;
  sensitive: boolean;
  required: boolean;
  error?: string;
}

export interface FlowInputSummary {
  name: string;
  label: string;
  type: ParamType;
  required: boolean;
  sensitive: boolean;
  default: string;
  options: string[];
  description: string;
  usedBy: string[];
}

export interface CompatibleParam {
  nodeID: string;
  nodeName: string;
  param: ToolParam;
}

export function topologicalOrder(nodes: ParamFlowNode[], edges: ParamEdge[]): ParamFlowNode[] {
  const index = new Map(nodes.map((node, i) => [node.id, i]));
  const indegree = new Map<string, number>();
  const outgoing = new Map<string, string[]>();
  nodes.forEach((node) => {
    indegree.set(node.id, 0);
    outgoing.set(node.id, []);
  });
  edges.forEach((edge) => {
    if (!index.has(edge.from) || !index.has(edge.to)) return;
    indegree.set(edge.to, (indegree.get(edge.to) ?? 0) + 1);
    outgoing.get(edge.from)?.push(edge.to);
  });
  const ready = nodes.filter((node) => (indegree.get(node.id) ?? 0) === 0).map((node) => node.id);
  const order: string[] = [];
  while (ready.length > 0) {
    const id = ready.shift()!;
    order.push(id);
    (outgoing.get(id) ?? []).forEach((next) => {
      const left = (indegree.get(next) ?? 0) - 1;
      indegree.set(next, left);
      if (left === 0) ready.push(next);
    });
  }
  if (order.length !== nodes.length) return nodes;
  const byID = new Map(nodes.map((node) => [node.id, node]));
  return order.map((id) => byID.get(id)!).filter(Boolean);
}

export function ancestorIDs(nodeID: string, edges: ParamEdge[]): Set<string> {
  const incoming = new Map<string, string[]>();
  edges.forEach((edge) => {
    const list = incoming.get(edge.to) ?? [];
    list.push(edge.from);
    incoming.set(edge.to, list);
  });
  const visited = new Set<string>();
  const stack = [...(incoming.get(nodeID) ?? [])];
  while (stack.length > 0) {
    const id = stack.pop()!;
    if (visited.has(id) || id === nodeID) continue;
    visited.add(id);
    stack.push(...(incoming.get(id) ?? []));
  }
  return visited;
}

export function toolFlowParams(tool: ToolSummary | undefined): ToolParam[] {
  if (!tool) return [];
  const host = tool.host_param;
  const hostParam: ToolParam = {
    name: "SPOOL_HOST",
    label: host?.label || "执行主机",
    type: "host",
    required: true,
    default: JSON.stringify(host?.default_host_ids ?? []),
    options: [],
    description: "",
    sensitive: false,
  };
  return [hostParam, ...(tool.params ?? [])];
}

function toolParams(tool: ToolSummary | undefined): ToolParam[] {
  return toolFlowParams(tool);
}

function optionsCover(current: string[], target: string[]): boolean {
  if (target.length === 0) return false;
  const allowed = new Set(current);
  return target.every((option) => allowed.has(option));
}

function validateLiteral(param: ToolParam, value: string): string | undefined {
  if (!value) return "固定值不能为空";
  if (param.type === "int" && !/^-?\d+$/.test(value)) return "必须是整数";
  if (param.type === "bool" && value !== "true" && value !== "false") return "必须是 true 或 false";
  if (param.type === "select" && !param.options.includes(value)) return "值不在候选项中";
  return undefined;
}

function resolveInternal(
  nodeID: string,
  paramName: string,
  nodesByID: Map<string, ParamFlowNode>,
  toolsByID: Map<string, ToolSummary>,
  ancestors: (id: string) => Set<string>,
  visiting: Set<string>,
): ResolvedBinding {
  const key = `${nodeID}\u0000${paramName}`;
  if (visiting.has(key)) {
    return { kind: "default", sensitive: false, required: false, error: "参数引用存在循环" };
  }
  const node = nodesByID.get(nodeID);
  const tool = node ? toolsByID.get(node.toolID) : undefined;
  const param = toolParams(tool).find((p) => p.name === paramName);
  if (!node || !tool || !param) {
    return { kind: "default", sensitive: false, required: false, error: "工具参数不存在" };
  }
  const mapping = node.mapping[paramName];
  if (!mapping) {
    return {
      kind: "default",
      value: param.default,
      sensitive: param.sensitive,
      required: param.required,
      error: param.required && !param.default ? "必填参数未配置来源" : undefined,
    };
  }
  if (mapping.source === "literal") {
    if (param.sensitive) {
      return { kind: "literal", sensitive: true, required: param.required, error: "敏感参数不能使用固定值" };
    }
    const error = validateLiteral(param, mapping.value);
    return {
      kind: "literal",
      value: mapping.value,
      sensitive: param.sensitive,
      required: param.required,
      error,
    };
  }
  if (mapping.source === "input") {
    if (!mapping.input_name) {
      return { kind: "input", sensitive: param.sensitive, required: param.required, error: "请填写输入名" };
    }
    if (!/^[A-Za-z_][A-Za-z0-9_]*$/.test(mapping.input_name)) {
      return { kind: "input", sensitive: param.sensitive, required: param.required, error: "输入名格式不合法" };
    }
    if (mapping.input_name.toUpperCase().startsWith("SPOOL_")) {
      return { kind: "input", sensitive: param.sensitive, required: param.required, error: "输入名不能以 SPOOL_ 开头" };
    }
    return {
      kind: "input",
      inputName: mapping.input_name,
      inputLabel: mapping.input_label ?? "",
      rootParam: param,
      sensitive: param.sensitive || !!mapping.input_sensitive,
      required: param.required,
    };
  }
  const target = nodesByID.get(mapping.ref_node_id);
  if (!target) {
    return { kind: "default", sensitive: param.sensitive, required: param.required, error: "引用的节点不存在" };
  }
  if (target.kind !== "tool") {
    return { kind: "default", sensitive: param.sensitive, required: param.required, error: "只能引用工具节点" };
  }
  if (!ancestors(nodeID).has(mapping.ref_node_id)) {
    return { kind: "default", sensitive: param.sensitive, required: param.required, error: "只能引用上游节点" };
  }
  const targetTool = toolsByID.get(target.toolID);
  const targetParam = toolParams(targetTool).find((p) => p.name === mapping.ref_param);
  if (!targetParam) {
    return { kind: "default", sensitive: param.sensitive, required: param.required, error: "引用的参数不存在" };
  }
  if (targetParam.type !== param.type) {
    return { kind: "default", sensitive: param.sensitive, required: param.required, error: "引用参数类型不一致" };
  }
  if (param.type === "select" && !optionsCover(param.options, targetParam.options)) {
    return { kind: "default", sensitive: param.sensitive, required: param.required, error: "候选项未覆盖上游参数" };
  }
  visiting.add(key);
  const resolved = resolveInternal(
    mapping.ref_node_id,
    mapping.ref_param,
    nodesByID,
    toolsByID,
    ancestors,
    visiting,
  );
  visiting.delete(key);
  const targetSensitive = resolved.sensitive;
  const sensitive = targetSensitive || param.sensitive;
  if (param.sensitive && resolved.kind === "literal") {
    return { ...resolved, sensitive, required: resolved.required || param.required, error: "敏感参数不能引用固定值" };
  }
  if (param.sensitive && resolved.kind === "default" && !targetSensitive) {
    return { ...resolved, sensitive, required: resolved.required || param.required, error: "敏感参数不能引用非敏感默认值" };
  }
  return {
    ...resolved,
    sensitive,
    required: resolved.required || param.required,
  };
}

export function resolveBinding(
  nodeID: string,
  paramName: string,
  nodes: ParamFlowNode[],
  edges: ParamEdge[],
  tools: Map<string, ToolSummary>,
): ResolvedBinding {
  const nodesByID = new Map(nodes.map((node) => [node.id, node]));
  const cache = new Map<string, Set<string>>();
  const ancestors = (id: string) => {
    let cached = cache.get(id);
    if (!cached) {
      cached = ancestorIDs(id, edges);
      cache.set(id, cached);
    }
    return cached;
  };
  return resolveInternal(nodeID, paramName, nodesByID, tools, ancestors, new Set());
}

export function deriveInputs(
  nodes: ParamFlowNode[],
  edges: ParamEdge[],
  tools: Map<string, ToolSummary>,
): FlowInputSummary[] {
  const order = topologicalOrder(nodes, edges);
  const accs = new Map<
    string,
    {
      labels: string[];
      params: ToolParam[];
      descriptions: string[];
      required: boolean;
      sensitive: boolean;
      usedBy: string[];
    }
  >();
  for (const node of order) {
    if (node.kind !== "tool") continue;
    const tool = tools.get(node.toolID);
    for (const param of toolParams(tool)) {
      const res = resolveBinding(node.id, param.name, nodes, edges, tools);
      if (res.kind !== "input" || !res.inputName || res.error) continue;
      const acc = accs.get(res.inputName) ?? {
        labels: [],
        params: [],
        descriptions: [],
        required: false,
        sensitive: false,
        usedBy: [],
      };
      if (res.inputLabel) acc.labels.push(res.inputLabel);
      if (res.rootParam?.label) acc.labels.push(res.rootParam.label);
      if (res.rootParam) acc.params.push(res.rootParam);
      if (res.rootParam?.description) acc.descriptions.push(res.rootParam.description);
      acc.required = acc.required || res.required;
      acc.sensitive = acc.sensitive || res.sensitive;
      if (!acc.usedBy.includes(node.name)) acc.usedBy.push(node.name);
      accs.set(res.inputName, acc);
    }
  }

  return Array.from(accs.entries()).map(([name, acc]) => {
    const base = acc.params[0];
    return {
      name,
      label: firstNonEmpty(...acc.labels, base?.label ?? "", name),
      type: base?.type ?? "string",
      required: acc.required,
      sensitive: acc.sensitive,
      default: "",
      options: intersectOptions(base?.type ?? "string", acc.params),
      description: firstNonEmpty(...acc.descriptions, base?.description ?? ""),
      usedBy: acc.usedBy,
    };
  });
}

function firstNonEmpty(...values: string[]): string {
  const found = values.find((value) => value.trim() !== "");
  return found ? found.trim() : "";
}

function intersectOptions(type: ParamType, params: ToolParam[]): string[] {
  if (type !== "select" || params.length === 0) return [];
  let allowed = new Set(params[0].options ?? []);
  for (const param of params.slice(1)) {
    const next = new Set(param.options ?? []);
    allowed = new Set([...allowed].filter((option) => next.has(option)));
  }
  return (params[0].options ?? []).filter((option) => allowed.has(option));
}


export interface InputConflict {
  kind: "name" | "label";
  value: string;
  message: string;
}

/** Returns conflicts between explicit execution inputs. Inputs created by
 *  downstream node references are not declarations and do not count. */
export function inputConflicts(nodes: ParamFlowNode[], tools: Map<string, ToolSummary>): InputConflict[] {
  const byName = new Map<string, string>();
  const byLabel = new Map<string, string>();
  const conflicts: InputConflict[] = [];
  for (const node of nodes) {
    if (node.kind !== "tool") continue;
    const tool = tools.get(node.toolID);
    for (const param of toolParams(tool)) {
      const mapping = node.mapping[param.name];
      if (mapping?.source !== "input") continue;
      const name = (mapping.input_name ?? "").trim();
      if (!name) continue;
      const label = firstNonEmpty(mapping.input_label ?? "", param.label, name);
      const owner = `${node.name || node.id} / ${param.name}`;
      const previousName = byName.get(name);
      if (previousName) {
        conflicts.push({
          kind: "name",
          value: name,
          message: `输入名 "${name}" 重复：${previousName} 与 ${owner} 冲突`,
        });
      } else {
        byName.set(name, owner);
      }
      const previousLabel = byLabel.get(label);
      if (previousLabel) {
        conflicts.push({
          kind: "label",
          value: label,
          message: `显示标题 "${label}" 重复：${previousLabel} 与 ${owner} 冲突`,
        });
      } else {
        byLabel.set(label, owner);
      }
    }
  }
  return conflicts;
}


export function compatibleUpstreamParams(
  nodeID: string,
  param: ToolParam,
  nodes: ParamFlowNode[],
  edges: ParamEdge[],
  tools: Map<string, ToolSummary>,
): CompatibleParam[] {
  const ancestors = ancestorIDs(nodeID, edges);
  const out: CompatibleParam[] = [];
  for (const node of nodes) {
    if (node.kind !== "tool" || !ancestors.has(node.id)) continue;
    const tool = tools.get(node.toolID);
    for (const candidate of toolParams(tool)) {
      if (candidate.type !== param.type) continue;
      if (param.type === "select" && !optionsCover(param.options, candidate.options)) continue;
      out.push({ nodeID: node.id, nodeName: node.name, param: candidate });
    }
  }
  return out;
}
