import { Card } from "antd";
import ExecutionTable from "../components/ExecutionTable";
import PageHeader from "../components/PageHeader";

export default function AdhocHistory() {
  return (
    <div>
      <PageHeader title="临时命令历史" subtitle="仅显示临时命令的执行记录" />
      <Card>
        <ExecutionTable kind="adhoc" showFilters />
      </Card>
    </div>
  );
}
