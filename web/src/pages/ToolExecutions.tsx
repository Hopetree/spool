import { Card } from "antd";
import ExecutionTable from "../components/ExecutionTable";
import PageHeader from "../components/PageHeader";

export default function ToolExecutions() {
  return (
    <div>
      <PageHeader
        title="工具执行历史"
        subtitle="仅显示直接执行工具产生的记录；流程中使用工具的运行记录属于流程执行历史"
      />
      <Card>
        <ExecutionTable kind="tool" showFilters />
      </Card>
    </div>
  );
}
