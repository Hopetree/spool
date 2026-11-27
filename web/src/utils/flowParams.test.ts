import { describe, expect, it } from "vitest";
import type { ToolParam, ToolSummary } from "../api/tools";
import {
  ancestorIDs,
  compatibleUpstreamParams,
  deriveInputs,
  inputConflicts,
  resolveBinding,
  topologicalOrder,
  type ParamEdge,
  type ParamFlowNode,
} from "./flowParams";

function tool(id: string, name: string, params: ToolParam[]): ToolSummary {
  return {
    id,
    name,
    category: "",
    description: "",
    enabled: true,
    version_no: 1,
    type: "shell",
    interpreter: "/bin/bash",
    timeout_sec: 300,
    sudo: false,
    content_hash: "",
    param_count: params.length,
    params,
    host_param: { label: "执行主机", default_host_ids: ["h1"] },
    created_at: 0,
    updated_at: 0,
  };
}

const params: ToolParam[] = [
  { name: "pkg_path", label: "包路径", type: "string", required: true, default: "", options: [], description: "", sensitive: false },
  { name: "work_dir", label: "工作目录", type: "string", required: false, default: "/tmp", options: [], description: "", sensitive: false },
  { name: "mode", label: "模式", type: "select", required: false, default: "", options: ["fast", "safe"], description: "", sensitive: false },
];

const tools = new Map([
  ["t1", tool("t1", "解压", params)],
  ["t2", tool("t2", "安装", params)],
]);

const nodes: ParamFlowNode[] = [
  { id: "start", kind: "start", name: "开始", toolID: "", mapping: {} },
  {
    id: "a",
    kind: "tool",
    name: "A",
    toolID: "t1",
    mapping: {
      pkg_path: { source: "input", input_name: "pkg_path", input_label: "巡检包路径" },
      mode: { source: "input", input_name: "mode", input_label: "执行模式" },
    },
  },
  {
    id: "b",
    kind: "tool",
    name: "B",
    toolID: "t2",
    mapping: {
      pkg_path: { source: "node", ref_node_id: "a", ref_param: "pkg_path" },
      work_dir: { source: "node", ref_node_id: "a", ref_param: "work_dir" },
      mode: { source: "node", ref_node_id: "a", ref_param: "mode" },
    },
  },
];

const edges: ParamEdge[] = [
  { from: "start", to: "a" },
  { from: "a", to: "b" },
];

describe("flow parameter helpers", () => {
  it("orders nodes and finds ancestors", () => {
    expect(topologicalOrder(nodes, edges).map((node) => node.id)).toEqual(["start", "a", "b"]);
    expect(ancestorIDs("b", edges)).toEqual(new Set(["a", "start"]));
    expect(ancestorIDs("a", edges)).toEqual(new Set(["start"]));
  });

  it("derives flow inputs from node bindings", () => {
    const inputs = deriveInputs(nodes, edges, tools);
    expect(inputs.map((input) => input.name)).toEqual(["pkg_path", "mode"]);
    expect(inputs[0]).toMatchObject({ label: "巡检包路径", type: "string", required: true, sensitive: false });
    expect(inputs[1]).toMatchObject({ label: "执行模式", type: "select", required: false });
    expect(inputs[1].options).toEqual(["fast", "safe"]);
    expect(inputs[0].usedBy).toEqual(["A", "B"]);
  });

  it("resolves node references to their root input", () => {
    const resolved = resolveBinding("b", "pkg_path", nodes, edges, tools);
    expect(resolved).toMatchObject({ kind: "input", inputName: "pkg_path", inputLabel: "巡检包路径" });
    const fixed = resolveBinding("b", "work_dir", nodes, edges, tools);
    expect(fixed).toMatchObject({ kind: "default", value: "/tmp" });
  });

  it("only offers type-compatible upstream parameters", () => {
    const mode = params.find((param) => param.name === "mode")!;
    const compatible = compatibleUpstreamParams("b", mode, nodes, edges, tools);
    expect(compatible.map((item) => item.param.name)).toEqual(["mode"]);
    expect(compatible[0].nodeName).toBe("A");
  });

  it("detects duplicate execution input names and labels", () => {
    expect(inputConflicts(nodes, tools)).toEqual([]);

    const duplicatedName = JSON.parse(JSON.stringify(nodes)) as ParamFlowNode[];
    duplicatedName[2].mapping.pkg_path = {
      source: "input",
      input_name: "pkg_path",
      input_label: "另一个包路径",
    };
    const nameConflicts = inputConflicts(duplicatedName, tools);
    expect(nameConflicts).toHaveLength(1);
    expect(nameConflicts[0]).toMatchObject({ kind: "name", value: "pkg_path" });

    const duplicatedLabel = JSON.parse(JSON.stringify(nodes)) as ParamFlowNode[];
    duplicatedLabel[2].mapping.pkg_path = {
      source: "input",
      input_name: "other_path",
      input_label: "巡检包路径",
    };
    const labelConflicts = inputConflicts(duplicatedLabel, tools);
    expect(labelConflicts).toHaveLength(1);
    expect(labelConflicts[0]).toMatchObject({ kind: "label", value: "巡检包路径" });
  });

  it("rejects sibling references", () => {
    const siblingEdges: ParamEdge[] = [
      { from: "start", to: "a" },
      { from: "start", to: "b" },
    ];
    const resolved = resolveBinding("b", "pkg_path", nodes, siblingEdges, tools);
    expect(resolved.error).toBe("只能引用上游节点");
  });
});
