import { useMemo } from "react";
import { Background, Controls, ReactFlow } from "@xyflow/react";
import "@xyflow/react/dist/style.css";
import type { FlowEdge, FlowNode } from "../api/flows";
import { flowNodeTypes, flowToRFEdges, flowToRFNodes } from "./FlowGraphParts";

export default function FlowGraph({
  nodes,
  edges,
  height = 460,
  interactive = false,
}: {
  nodes: FlowNode[];
  edges: FlowEdge[];
  height?: number;
  interactive?: boolean;
}) {
  const rfNodes = useMemo(() => flowToRFNodes(nodes, interactive), [nodes, interactive]);
  const rfEdges = useMemo(() => flowToRFEdges(edges), [edges]);

  return (
    <div style={{ height }}>
      <ReactFlow
        className="light"
        nodes={rfNodes}
        edges={rfEdges}
        nodeTypes={flowNodeTypes}
        fitView
        nodesDraggable={interactive}
        nodesConnectable={interactive}
        elementsSelectable={interactive}
        proOptions={{ hideAttribution: true }}
      >
        <Background />
        <Controls showInteractive={false} />
      </ReactFlow>
    </div>
  );
}
