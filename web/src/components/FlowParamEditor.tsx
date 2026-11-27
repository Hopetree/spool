import { Alert, Button, Card, Empty, Input, InputNumber, Select, Space, Table, Tag, Typography } from "antd";
import type { FlowMapping } from "../api/flows";
import type { ToolParam, ToolSummary } from "../api/tools";
import {
  compatibleUpstreamParams,
  inputConflicts,
  topologicalOrder,
  type CompatibleParam,
  type ParamEdge,
  type ParamFlowNode,
} from "../utils/flowParams";

interface Props {
  nodes: ParamFlowNode[];
  edges: ParamEdge[];
  tools: ToolSummary[];
  onChange: (nodeID: string, mapping: Record<string, FlowMapping>) => void;
  onOpenNode: (nodeID: string) => void;
}

const sourceOptions = [
  { value: "default", label: "工具默认值" },
  { value: "literal", label: "固定值" },
  { value: "input", label: "执行时输入" },
  { value: "node", label: "引用上游参数" },
];

function sourceOf(mapping: FlowMapping | undefined): string {
  return mapping?.source ?? "default";
}

function tagForParam(param: ToolParam) {
  return (
    <Space size={4} wrap>
      <Tag>{param.type}</Tag>
      {param.required && <Tag color="orange">必填</Tag>}
      {param.sensitive && <Tag color="red">敏感</Tag>}
    </Space>
  );
}

function defaultLiteral(param: ToolParam): string {
  if (param.default) return param.default;
  if (param.type === "bool") return "false";
  if (param.type === "select") return param.options[0] ?? "";
  return "";
}

export default function FlowParamEditor({ nodes, edges, tools, onChange, onOpenNode }: Props) {
  const toolsByID = new Map(tools.map((tool) => [tool.id, tool]));
  const conflicts = inputConflicts(nodes, toolsByID);
  const nodeByID = new Map(nodes.map((node) => [node.id, node]));
  const toolNodes = topologicalOrder(nodes, edges).filter((node) => node.kind === "tool");

  if (toolNodes.length === 0) {
    return <Empty description="尚未添加工具节点，请返回“流程设计”添加节点" />;
  }

  const updateMapping = (nodeID: string, paramName: string, mapping?: FlowMapping) => {
    const node = nodeByID.get(nodeID);
    if (!node) return;
    const next = { ...node.mapping };
    if (mapping) next[paramName] = mapping;
    else delete next[paramName];
    onChange(nodeID, next);
  };

  return (
    <Space direction="vertical" size={12} style={{ width: "100%" }}>
      {conflicts.length > 0 && (
        <Alert
          type="error"
          showIcon
          message="参数设置存在冲突"
          description={
            <Space direction="vertical" size={2}>
              {conflicts.map((conflict, index) => (
                <Typography.Text key={`${conflict.kind}-${conflict.value}-${index}`} type="danger">
                  {conflict.message}
                </Typography.Text>
              ))}
            </Space>
          }
        />
      )}
      {toolNodes.map((node) => {
        const tool = toolsByID.get(node.toolID);
        const params = tool?.params ?? [];
        return (
          <Card
            key={node.id}
            size="small"
            title={
              <Space wrap>
                <Typography.Text strong>{node.name || "未命名节点"}</Typography.Text>
                <Typography.Text type="secondary">{tool ? tool.name : "未选择工具"}</Typography.Text>
              </Space>
            }
            extra={
              <Button size="small" onClick={() => onOpenNode(node.id)}>
                节点设置
              </Button>
            }
          >
            {!tool ? (
              <Alert type="warning" showIcon message="请先在“流程设计”中为节点选择工具" />
            ) : params.length === 0 ? (
              <Typography.Text type="secondary">该工具没有参数</Typography.Text>
            ) : (
              <Table<ToolParam>
                rowKey="name"
                size="small"
                pagination={false}
                dataSource={params}
                columns={[
                  {
                    title: "参数",
                    width: 190,
                    render: (_, param) => (
                      <Space direction="vertical" size={0}>
                        <Typography.Text strong>{param.name}</Typography.Text>
                        <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                          {param.label || param.description || "—"}
                        </Typography.Text>
                        {tagForParam(param)}
                      </Space>
                    ),
                  },
                  {
                    title: "来源",
                    width: 160,
                    render: (_, param) => {
                      const mapping = node.mapping[param.name];
                      const compatible = compatibleUpstreamParams(node.id, param, nodes, edges, toolsByID);
                      const source = sourceOf(mapping);
                      return (
                        <Select
                          size="small"
                          value={source}
                          style={{ width: 140 }}
                          onChange={(next) => {
                            if (next === "default") {
                              updateMapping(node.id, param.name);
                              return;
                            }
                            if (next === "literal") {
                              updateMapping(node.id, param.name, {
                                source: "literal",
                                value: defaultLiteral(param),
                              });
                              return;
                            }
                            if (next === "input") {
                              updateMapping(node.id, param.name, {
                                source: "input",
                                input_name: param.name,
                                input_label: param.label || param.name,
                              });
                              return;
                            }
                            const first = compatible[0];
                            if (!first) return;
                            updateMapping(node.id, param.name, {
                              source: "node",
                              ref_node_id: first.nodeID,
                              ref_param: first.param.name,
                            });
                          }}
                          options={sourceOptions.map((option) => ({
                            ...option,
                            disabled:
                              (option.value === "literal" && param.sensitive) ||
                              (option.value === "node" && compatible.length === 0),
                          }))}
                        />
                      );
                    },
                  },
                  {
                    title: "值 / 配置",
                    render: (_, param) => (
                      <ParamValueEditor
                        param={param}
                        mapping={node.mapping[param.name]}
                        compatible={compatibleUpstreamParams(node.id, param, nodes, edges, toolsByID)}
                        onChange={(mapping) => updateMapping(node.id, param.name, mapping)}
                      />
                    ),
                  },
                ]}
              />
            )}
          </Card>
        );
      })}
    </Space>
  );
}

function ParamValueEditor({
  param,
  mapping,
  compatible,
  onChange,
}: {
  param: ToolParam;
  mapping: FlowMapping | undefined;
  compatible: CompatibleParam[];
  onChange: (mapping?: FlowMapping) => void;
}) {
  const source = sourceOf(mapping);
  if (source === "default") {
    if (param.required && !param.default) {
      return <Typography.Text type="danger">必填参数未配置来源</Typography.Text>;
    }
    return <Typography.Text type="secondary">{param.default || "—"}</Typography.Text>;
  }

  if (mapping?.source === "literal") {
    return (
      <Space direction="vertical" size={2} style={{ width: "100%" }}>
        {param.type === "bool" ? (
          <Select
            size="small"
            value={mapping.value}
            style={{ width: 160 }}
            onChange={(value) => onChange({ source: "literal", value })}
            options={[
              { value: "true", label: "true" },
              { value: "false", label: "false" },
            ]}
          />
        ) : param.type === "select" ? (
          <Select
            size="small"
            value={mapping.value || undefined}
            style={{ width: 200 }}
            placeholder="选择固定值"
            onChange={(value) => onChange({ source: "literal", value })}
            options={param.options.map((option) => ({ value: option, label: option }))}
          />
        ) : param.type === "int" ? (
          <InputNumber
            size="small"
            value={mapping.value ? Number(mapping.value) : undefined}
            style={{ width: 160 }}
            onChange={(value) => onChange({ source: "literal", value: value === null ? "" : String(value) })}
          />
        ) : (
          <Input
            size="small"
            value={mapping.value}
            placeholder="固定值"
            onChange={(e) => onChange({ source: "literal", value: e.target.value })}
          />
        )}
      </Space>
    );
  }

  if (mapping?.source === "input") {
    return (
      <Space wrap>
        <Input
          size="small"
          value={mapping.input_name ?? ""}
          style={{ width: 170 }}
          placeholder="输入名"
          onChange={(e) => onChange({ ...mapping, input_name: e.target.value })}
        />
        <Input
          size="small"
          value={mapping.input_label ?? ""}
          style={{ width: 180 }}
          placeholder="显示标题"
          onChange={(e) => onChange({ ...mapping, input_label: e.target.value })}
        />
      </Space>
    );
  }

  if (mapping?.source === "node") {
    const nodeOptions = Array.from(new Set(compatible.map((item) => item.nodeID))).map((id) => {
      const item = compatible.find((candidate) => candidate.nodeID === id)!;
      return { value: id, label: item.nodeName };
    });
    const paramOptions = compatible
      .filter((item) => item.nodeID === mapping.ref_node_id)
      .map((item) => ({ value: item.param.name, label: `${item.param.name}（${item.param.type}）` }));
    return (
      <Space direction="vertical" size={4} style={{ width: "100%" }}>
        <Space wrap>
          <Select
            size="small"
            value={mapping.ref_node_id || undefined}
            style={{ width: 160 }}
            placeholder="上游节点"
            options={nodeOptions}
            onChange={(nodeID) => {
              const first = compatible.find((item) => item.nodeID === nodeID);
              if (!first) return;
              onChange({ source: "node", ref_node_id: nodeID, ref_param: first.param.name });
            }}
          />
          <Select
            size="small"
            value={mapping.ref_param || undefined}
            style={{ width: 190 }}
            placeholder="上游参数"
            options={paramOptions}
            onChange={(paramName) =>
              onChange({ source: "node", ref_node_id: mapping.ref_node_id, ref_param: paramName })
            }
          />
        </Space>
      </Space>
    );
  }

  return null;
}
