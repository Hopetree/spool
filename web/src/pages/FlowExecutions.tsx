import { Card } from "antd";
import ExecutionTable from "../components/ExecutionTable";
import PageHeader from "../components/PageHeader";

export default function FlowExecutions() {
  return (
    <div>
      <PageHeader title="流程历史" subtitle="每次运行包含多个主机的节点执行与输出" />
      <Card>
        <ExecutionTable kind="flow" showFilters />
      </Card>
    </div>
  );
}
