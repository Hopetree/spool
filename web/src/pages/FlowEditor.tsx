import { useCallback, useEffect, useMemo, useRef, useState, type MouseEvent as ReactMouseEvent } from "react";
import {
  App as AntApp,
  Alert,
  Button,
  Card,
  Drawer,
  Form,
  Input,
  InputNumber,
  Select,
  Space,
  Switch,
  Tag,
  Typography,
} from "antd";
import {
  Background,
  Controls,
  ReactFlow,
  type Connection,
  type FinalConnectionState,
  type OnConnectEnd,
  type OnEdgesChange,
  type OnNodesChange,
  type OnNodesDelete,
  type ReactFlowInstance,
} from "@xyflow/react";
import "@xyflow/react/dist/style.css";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useNavigate, useParams } from "react-router-dom";
import { ApiError } from "../api/client";
import {
  createFlow,
  getFlow,
  updateFlow,
  type EdgeCondition,
  type FlowInput,
  type FlowMapping,
} from "../api/flows";
import { listTools, type ToolSummary } from "../api/tools";
import {
  edgeConditionOptions,
  flowNodeTypes,
  flowToRFEdges,
  flowToRFNodes,
  makeRFEdge,
  restyleEdge,
  type FlowEdgeData,
  type FlowNodeData,
  type FlowRFEdge,
  type FlowRFNode,
} from "../components/FlowGraphParts";
import { useAuthStore } from "../stores/auth";

interface ParamForm {
  name: string;
  label?: string;
  type: "string" | "int" | "bool" | "select";
  required?: boolean;
  default?: string;
  optionsText?: string;
  description?: string;
  sensitive?: boolean;
}

interface FormValues {
  name: string;
  description?: string;
  max_duration_sec?: number;
  params?: ParamForm[];
}

const paramTypeOptions = [
  { value: "string", label: "字符串" },
  { value: "int", label: "整数" },
  { value: "bool", label: "布尔" },
  { value: "select", label: "枚举" },
];

function newID(prefix: string): string {
  return `${prefix}_${Math.random().toString(36).slice(2, 10)}`;
}

function toolNodeData(name = "新节点"): FlowNodeData {
  return {
    kind: "tool",
    name,
    toolID: "",
    toolName: "",
    toolType: "",
    mapping: {},
    timeoutSec: 0,
    retryCount: 0,
    retryDelaySec: 0,
  };
}

function MappingEditor({
  tool,
  mapping,
  flowParams,
  onChange,
}: {
  tool: ToolSummary | undefined;
  mapping: Record<string, FlowMapping>;
  flowParams: ParamForm[];
  onChange: (mapping: Record<string, FlowMapping>) => void;
}) {
  if (!tool) {
    return <Typography.Text type="secondary">请先选择工具</Typography.Text>;
  }
  if (tool.params.length === 0) {
    return <Typography.Text type="secondary">该工具没有参数</Typography.Text>;
  }
  const setMapping = (name: string, m?: FlowMapping) => {
    const next = { ...mapping };
    if (m) next[name] = m;
    else delete next[name];
    onChange(next);
  };
  return (
    <div>
      {tool.params.map((p) => {
        const m = mapping[p.name];
        const source: string = m?.source ?? "default";
        const eligible = flowParams.filter((fp) => !p.sensitive || fp.sensitive);
        return (
          <div
            key={p.name}
            style={{ display: "flex", gap: 8, alignItems: "center", padding: "6px 0", borderTop: "1px solid #f0f0f0" }}
          >
            <div style={{ width: 150, fontSize: 12 }}>
              {p.name}
              {p.sensitive && <Tag color="red" style={{ marginLeft: 4 }}>敏感</Tag>}
            </div>
            <Select
              size="small"
              style={{ width: 100 }}
              value={source}
              onChange={(next: string) => {
                if (next === "default") return setMapping(p.name);
                if (next === "flow") return setMapping(p.name, { source: "flow", value: eligible[0]?.name ?? "" });
                setMapping(p.name, { source: "literal", value: "" });
              }}
              options={[
                { value: "default", label: "默认值" },
                { value: "flow", label: "流程参数" },
                { value: "literal", label: "固定值", disabled: p.sensitive },
              ]}
            />
            <div style={{ flex: 1 }}>
              {source === "flow" && (
                <Select
                  size="small"
                  style={{ width: "100%" }}
                  value={m?.value || undefined}
                  placeholder="选择流程参数"
                  onChange={(v) => setMapping(p.name, { source: "flow", value: v })}
                  options={eligible.map((fp) => ({
                    value: fp.name,
                    label: `${fp.name}${fp.sensitive ? "（敏感）" : ""}`,
                  }))}
                />
              )}
              {source === "literal" && (
                <Input
                  size="small"
                  value={m?.value ?? ""}
                  onChange={(e) => setMapping(p.name, { source: "literal", value: e.target.value })}
                />
              )}
              {source === "default" && (
                <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                  {p.default ? `默认：${p.default}` : p.required ? "必填，必须映射" : "无默认值"}
                </Typography.Text>
              )}
            </div>
          </div>
        );
      })}
    </div>
  );
}

export default function FlowEditor() {
  const { id } = useParams();
  const isNew = !id;
  const qc = useQueryClient();
  const navigate = useNavigate();
  const { message } = AntApp.useApp();
  const user = useAuthStore((s) => s.user);
  const isAdmin = user?.role === "admin";

  const [form] = Form.useForm<FormValues>();
  const [nodes, setNodes] = useState<FlowRFNode[]>([]);
  const [edges, setEdges] = useState<FlowRFEdge[]>([]);
  const [selectedNodeID, setSelectedNodeID] = useState<string>();
  const [selectedEdgeID, setSelectedEdgeID] = useState<string>();
  const flowRef = useRef<ReactFlowInstance<FlowRFNode, FlowRFEdge> | null>(null);

  const { data: detail } = useQuery({
    queryKey: ["flow", id],
    queryFn: () => getFlow(id!),
    enabled: !!id,
  });
  const { data: toolPage } = useQuery({
    queryKey: ["tools", "flow-editor"],
    queryFn: () => listTools({ page: 1, page_size: 100 }),
  });
  const tools = (toolPage?.items ?? []).filter((t) => t.enabled);
  const watchedParams = Form.useWatch("params", form) as ParamForm[] | undefined;
  const flowParams = watchedParams ?? [];

  useEffect(() => {
    if (detail) {
      form.setFieldsValue({
        name: detail.name,
        description: detail.description,
        max_duration_sec: detail.max_duration_sec,
        params: detail.params.map((p) => ({ ...p, optionsText: (p.options ?? []).join(", ") })),
      });
      setNodes(flowToRFNodes(detail.nodes, true));
      setEdges(flowToRFEdges(detail.edges));
      return;
    }
    if (isNew) {
      setNodes((prev) =>
        prev.length
          ? prev
          : [
              {
                id: "start",
                type: "start",
                position: { x: 40, y: 140 },
                data: {
                  kind: "start",
                  name: "开始",
                  toolID: "",
                  toolName: "",
                  toolType: "",
                  mapping: {},
                  timeoutSec: 0,
                  retryCount: 0,
                  retryDelaySec: 0,
                },
                deletable: false,
              },
            ],
      );
    }
  }, [detail, isNew, form]);

  const selectedNode = nodes.find((n) => n.id === selectedNodeID);
  const selectedEdge = edges.find((e) => e.id === selectedEdgeID);
  const selectedTool = selectedNode ? tools.find((t) => t.id === selectedNode.data.toolID) : undefined;

  const updateNodeData = (nodeID: string, patch: Partial<FlowNodeData>) =>
    setNodes((prev) => prev.map((n) => (n.id === nodeID ? { ...n, data: { ...n.data, ...patch } } : n)));

  const updateEdge = (edgeID: string, patch: Partial<FlowEdgeData>) =>
    setEdges((prev) =>
      prev.map((e) =>
        e.id === edgeID
          ? restyleEdge({
              ...e,
              data: {
                condition: e.data?.condition ?? "success",
                value: e.data?.value ?? "",
                ...patch,
              },
            })
          : e,
      ),
    );

  // In v13 node dimensions are reported through onNodesChange. They must be
  // applied to state, otherwise the internal handle bounds are reset on every
  // props sync and edges can no longer resolve their endpoints.
  const onNodesChange: OnNodesChange<FlowRFNode> = useCallback((changes) => {
    setNodes((prev) => changes.applyTo(prev));
  }, []);

  const onEdgesChange: OnEdgesChange<FlowRFEdge> = useCallback((changes) => {
    setEdges((prev) => changes.applyTo(prev));
  }, []);

  const onNodesDelete: OnNodesDelete<FlowRFNode> = useCallback((deleted) => {
    const ids = new Set(deleted.map((n) => n.id));
    setEdges((prev) => prev.filter((e) => !ids.has(e.source) && !ids.has(e.target)));
    setSelectedNodeID((cur) => (cur && ids.has(cur) ? undefined : cur));
  }, []);

  const onConnect = useCallback(
    (connection: Connection) => {
      const { source, target } = connection;
      if (!source || !target) return;
      if (source === target) {
        message.warning("连线不能指向自己");
        return;
      }
      if (target === "start") {
        message.warning("开始节点不能作为连线终点");
        return;
      }
      if (edges.some((e) => e.source === source && e.target === target)) {
        message.warning("这两个节点之间已有连线");
        return;
      }
      setEdges((prev) => [
        ...prev,
        makeRFEdge({ id: newID("e"), source, target, condition: "success", value: "" }),
      ]);
    },
    [edges, message],
  );

  /** Creates a tool node at a flow position and opens its config drawer. */
  const addNodeAt = useCallback((position: { x: number; y: number }, sourceID?: string) => {
    const nodeID = newID("n");
    setNodes((prev) => [...prev, { id: nodeID, type: "tool", position, data: toolNodeData() }]);
    if (sourceID) {
      setEdges((prev) => [
        ...prev,
        makeRFEdge({
          id: newID("e"),
          source: sourceID,
          target: nodeID,
          condition: sourceID === "start" ? "always" : "success",
          value: "",
        }),
      ]);
    }
    setSelectedNodeID(nodeID);
  }, []);

  const addNode = () => {
    const rightMost = nodes.reduce((max, n) => Math.max(max, n.position.x), 0);
    addNodeAt({ x: rightMost + 260, y: 60 + (nodes.length % 5) * 110 }, "start");
  };

  // Drop a connection on empty canvas to create a connected node there.
  const onConnectEnd: OnConnectEnd = useCallback(
    (event, state: FinalConnectionState) => {
      if (!state.fromHandle || state.fromHandle.nodeId === "" || state.isValid || state.toNode) return;
      const handle = state.fromHandle;
      if (handle.type !== "source") return;
      const point = "changedTouches" in event ? event.changedTouches[0] : event;
      if (!point) return;
      const instance = flowRef.current;
      if (!instance) return;
      const position = instance.screenToFlowPosition({ x: point.clientX, y: point.clientY });
      // Ignore a click on the handle without an actual drag.
      if (Math.hypot(position.x - state.from.x, position.y - state.from.y) < 40) return;
      addNodeAt({ x: position.x - 84, y: position.y - 24 }, handle.nodeId);
    },
    [addNodeAt],
  );

  const handleCanvasDoubleClick = (event: ReactMouseEvent<HTMLDivElement>) => {
    if (!(event.target instanceof HTMLElement) || !event.target.classList.contains("react-flow__pane")) return;
    const instance = flowRef.current;
    if (!instance) return;
    const position = instance.screenToFlowPosition({ x: event.clientX, y: event.clientY });
    addNodeAt({ x: position.x - 84, y: position.y - 24 });
  };

  const removeNode = (nodeID: string) => {
    if (nodeID === "start") return;
    setNodes((prev) => prev.filter((n) => n.id !== nodeID));
    setEdges((prev) => prev.filter((e) => e.source !== nodeID && e.target !== nodeID));
    setSelectedNodeID(undefined);
  };

  const buildInput = (values: FormValues): FlowInput => ({
    name: values.name,
    description: values.description ?? "",
    max_duration_sec: values.max_duration_sec ?? 3600,
    params: (values.params ?? []).map((p) => ({
      name: p.name,
      label: p.label ?? "",
      type: p.type,
      required: !!p.required,
      default: p.default ?? "",
      options: (p.optionsText ?? "")
        .split(",")
        .map((s) => s.trim())
        .filter(Boolean),
      description: p.description ?? "",
      sensitive: !!p.sensitive,
    })),
    nodes: nodes.map((n) => ({
      id: n.id,
      type: n.data.kind,
      name: n.data.kind === "start" ? "开始" : n.data.name,
      tool_id: n.data.kind === "start" ? "" : n.data.toolID,
      mapping: n.data.kind === "start" ? {} : n.data.mapping,
      timeout_sec: n.data.timeoutSec,
      retry_count: n.data.retryCount,
      retry_delay_sec: n.data.retryDelaySec,
      x: Math.round(n.position.x),
      y: Math.round(n.position.y),
    })),
    edges: edges.map((e) => ({
      from: e.source,
      to: e.target,
      condition: e.data?.condition ?? "success",
      value: e.data?.value ?? "",
    })),
  });

  const save = useMutation({
    mutationFn: (input: FlowInput) => (isNew ? createFlow(input) : updateFlow(id!, input)),
    onSuccess: (created) => {
      message.success("已保存");
      void qc.invalidateQueries({ queryKey: ["flows"] });
      void qc.invalidateQueries({ queryKey: ["flow", created.id] });
      navigate(`/flows/${created.id}`, { replace: true });
    },
    onError: (err) => message.error(err instanceof ApiError ? err.message : "保存失败"),
  });

  const submit = (values: FormValues) => {
    const noTool = nodes.find((n) => n.data.kind === "tool" && !n.data.toolID);
    if (noTool) {
      message.warning(`节点「${noTool.data.name || "未命名节点"}」还未选择工具`);
      setSelectedNodeID(noTool.id);
      return;
    }
    const noName = nodes.find((n) => n.data.kind === "tool" && !n.data.name.trim());
    if (noName) {
      message.warning("工具节点名称不能为空");
      setSelectedNodeID(noName.id);
      return;
    }
    save.mutate(buildInput(values));
  };

  const nodeCount = useMemo(() => nodes.length, [nodes]);
  const edgeCount = useMemo(() => edges.length, [edges]);

  if (!isAdmin) {
    return (
      <Card>
        <Typography.Title level={4}>流程定义仅管理员可编辑</Typography.Title>
        <Button onClick={() => navigate("/flows")}>返回列表</Button>
      </Card>
    );
  }

  return (
    <div>
      <Space style={{ marginBottom: 16 }} wrap>
        <Button onClick={() => navigate(isNew ? "/flows" : `/flows/${id}`)}>返回</Button>
        <Typography.Title level={4} style={{ margin: 0 }}>
          {isNew ? "新建流程" : `编辑流程：${detail?.name ?? ""}`}
        </Typography.Title>
        <Button onClick={addNode}>添加工具节点</Button>
        <Typography.Text type="secondary">
          {nodeCount} 个节点 · {edgeCount} 条连线
        </Typography.Text>
        <Button type="primary" loading={save.isPending} onClick={() => form.submit()}>
          保存流程
        </Button>
      </Space>

      <Form<FormValues>
        form={form}
        layout="vertical"
        onFinish={submit}
        initialValues={{ max_duration_sec: 3600, params: [] }}
      >
        <Card title="基本信息" style={{ marginBottom: 16 }}>
          <Space wrap align="start">
            <Form.Item name="name" label="名称" rules={[{ required: true, message: "请输入名称" }]} style={{ width: 240 }}>
              <Input />
            </Form.Item>
            <Form.Item name="max_duration_sec" label="单机最大时长（秒）" style={{ width: 180 }}>
              <InputNumber min={1} max={86400} style={{ width: "100%" }} />
            </Form.Item>
          </Space>
          <Form.Item name="description" label="描述">
            <Input />
          </Form.Item>
        </Card>

        <Card title="流程参数" style={{ marginBottom: 16 }}>
          <Form.List name="params">
            {(fields, { add, remove }) => (
              <>
                {fields.map(({ key, name, ...rest }) => (
                  <Space key={key} wrap align="start" style={{ display: "flex", marginBottom: 8 }}>
                    <Form.Item {...rest} name={[name, "name"]} label="变量名" rules={[{ required: true, message: "必填" }]}>
                      <Input style={{ width: 130 }} placeholder="ENV" />
                    </Form.Item>
                    <Form.Item {...rest} name={[name, "label"]} label="显示名">
                      <Input style={{ width: 120 }} />
                    </Form.Item>
                    <Form.Item {...rest} name={[name, "type"]} label="类型" initialValue="string">
                      <Select style={{ width: 100 }} options={paramTypeOptions} />
                    </Form.Item>
                    <Form.Item {...rest} name={[name, "required"]} label="必填" valuePropName="checked">
                      <Switch />
                    </Form.Item>
                    <Form.Item {...rest} name={[name, "sensitive"]} label="敏感" valuePropName="checked">
                      <Switch />
                    </Form.Item>
                    <Form.Item {...rest} name={[name, "default"]} label="默认值">
                      <Input style={{ width: 130 }} />
                    </Form.Item>
                    <Form.Item {...rest} name={[name, "optionsText"]} label="候选项">
                      <Input style={{ width: 140 }} placeholder="逗号分隔" />
                    </Form.Item>
                    <Button danger style={{ marginTop: 30 }} onClick={() => remove(name)}>
                      删除
                    </Button>
                  </Space>
                ))}
                <Button block onClick={() => add({ type: "string", required: false, sensitive: false })}>
                  添加流程参数
                </Button>
              </>
            )}
          </Form.List>
        </Card>

        <Card
          title="流程图"
          style={{ marginBottom: 16 }}
          extra={
            <Typography.Text type="secondary" style={{ fontSize: 12 }}>
              双击空白画布新建节点；从节点右侧圆点拖到空白处可直接新建并连接；点击节点/连线配置
            </Typography.Text>
          }
        >
          <div style={{ height: 620 }} onDoubleClick={handleCanvasDoubleClick}>
            <ReactFlow
              className="light"
              nodes={nodes}
              edges={edges}
              nodeTypes={flowNodeTypes}
              onInit={(instance) => {
                flowRef.current = instance;
              }}
              onNodesChange={onNodesChange}
              onEdgesChange={onEdgesChange}
              onNodesDelete={onNodesDelete}
              onConnect={onConnect}
              onConnectEnd={onConnectEnd}
              zoomOnDoubleClick={false}
              onNodeClick={(_, n) => {
                setSelectedNodeID(n.id);
                setSelectedEdgeID(undefined);
              }}
              onEdgeClick={(_, e) => {
                setSelectedEdgeID(e.id);
                setSelectedNodeID(undefined);
              }}
              onPaneClick={() => {
                setSelectedNodeID(undefined);
                setSelectedEdgeID(undefined);
              }}
              fitView
              proOptions={{ hideAttribution: true }}
            >
              <Background />
              <Controls showInteractive={false} />
            </ReactFlow>
          </div>
        </Card>
      </Form>

      <Drawer
        open={!!selectedNode}
        onClose={() => setSelectedNodeID(undefined)}
        width={520}
        title={selectedNode?.data.kind === "start" ? "开始节点" : "节点配置"}
      >
        {selectedNode && selectedNode.data.kind === "start" && (
          <Alert type="info" showIcon message="开始节点固定存在，只负责触发后续分支，不能绑定工具。" />
        )}
        {selectedNode && selectedNode.data.kind === "tool" && (
          <Space direction="vertical" size={12} style={{ width: "100%" }}>
            <Space wrap>
              <div>
                <div style={{ fontSize: 12, color: "#8c8c8c" }}>节点名称</div>
                <Input
                  value={selectedNode.data.name}
                  style={{ width: 180 }}
                  onChange={(e) => updateNodeData(selectedNode.id, { name: e.target.value })}
                />
              </div>
              <div>
                <div style={{ fontSize: 12, color: "#8c8c8c" }}>工具</div>
                <Select
                  style={{ width: 220 }}
                  showSearch
                  optionFilterProp="label"
                  value={selectedNode.data.toolID || undefined}
                  placeholder="选择工具"
                  onChange={(toolID) => {
                    const tool = tools.find((t) => t.id === toolID);
                    updateNodeData(selectedNode.id, {
                      toolID,
                      toolName: tool?.name ?? "",
                      toolType: tool?.type ?? "",
                      mapping: {},
                      name:
                        selectedNode.data.name === "新节点"
                          ? tool?.name ?? selectedNode.data.name
                          : selectedNode.data.name,
                    });
                  }}
                  options={tools.map((t) => ({ value: t.id, label: `${t.name} (v${t.version_no})` }))}
                />
              </div>
            </Space>
            <Space wrap>
              <div>
                <div style={{ fontSize: 12, color: "#8c8c8c" }}>超时（秒，0=工具默认）</div>
                <InputNumber
                  min={0}
                  max={86400}
                  value={selectedNode.data.timeoutSec}
                  onChange={(v) => updateNodeData(selectedNode.id, { timeoutSec: v ?? 0 })}
                />
              </div>
              <div>
                <div style={{ fontSize: 12, color: "#8c8c8c" }}>重试</div>
                <InputNumber
                  min={0}
                  max={3}
                  value={selectedNode.data.retryCount}
                  onChange={(v) => updateNodeData(selectedNode.id, { retryCount: v ?? 0 })}
                />
              </div>
              <div>
                <div style={{ fontSize: 12, color: "#8c8c8c" }}>重试间隔（秒）</div>
                <InputNumber
                  min={0}
                  max={300}
                  value={selectedNode.data.retryDelaySec}
                  onChange={(v) => updateNodeData(selectedNode.id, { retryDelaySec: v ?? 0 })}
                />
              </div>
            </Space>
            <div>
              <Typography.Text strong>参数映射</Typography.Text>
              <MappingEditor
                tool={selectedTool}
                mapping={selectedNode.data.mapping}
                flowParams={flowParams}
                onChange={(mapping) => updateNodeData(selectedNode.id, { mapping })}
              />
            </div>
            <Button danger onClick={() => removeNode(selectedNode.id)}>
              删除节点
            </Button>
          </Space>
        )}
      </Drawer>

      <Drawer
        open={!!selectedEdge}
        onClose={() => setSelectedEdgeID(undefined)}
        width={420}
        title="连线条件"
      >
        {selectedEdge && (
          <Space direction="vertical" size={12} style={{ width: "100%" }}>
            <div style={{ fontSize: 12, color: "#8c8c8c" }}>
              {selectedEdge.source} → {selectedEdge.target}
            </div>
            <div>
              <div style={{ fontSize: 12, color: "#8c8c8c" }}>条件</div>
              <Select
                style={{ width: "100%" }}
                value={selectedEdge.data?.condition ?? "success"}
                options={edgeConditionOptions}
                onChange={(v) => updateEdge(selectedEdge.id, { condition: v as EdgeCondition, value: "" })}
              />
            </div>
            {(selectedEdge.data?.condition === "output_contains" ||
              selectedEdge.data?.condition === "output_not_contains") && (
              <div>
                <div style={{ fontSize: 12, color: "#8c8c8c" }}>匹配文本</div>
                <Input
                  value={selectedEdge.data?.value ?? ""}
                  onChange={(e) => updateEdge(selectedEdge.id, { value: e.target.value })}
                />
              </div>
            )}
            <Button
              danger
              onClick={() => {
                setEdges((prev) => prev.filter((e) => e.id !== selectedEdge.id));
                setSelectedEdgeID(undefined);
              }}
            >
              删除连线
            </Button>
          </Space>
        )}
      </Drawer>
    </div>
  );
}
