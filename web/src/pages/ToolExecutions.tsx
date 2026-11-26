import { Card, Typography } from "antd";
import ExecutionTable from "../components/ExecutionTable";

export default function ToolExecutions() {
  return (
    <Card>
      <Typography.Title level={4}>工具执行历史</Typography.Title>
      <Typography.Paragraph type="secondary">
        这里仅显示直接执行工具产生的记录。流程中使用工具的运行记录属于流程执行历史。
      </Typography.Paragraph>
      <ExecutionTable kind="tool" showFilters />
    </Card>
  );
}
