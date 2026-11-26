import { Card, Typography } from "antd";
import ExecutionTable from "../components/ExecutionTable";

export default function AdhocHistory() {
  return (
    <Card>
      <Typography.Title level={4}>临时命令历史</Typography.Title>
      <Typography.Paragraph type="secondary">这里仅显示临时命令的执行记录。</Typography.Paragraph>
      <ExecutionTable kind="adhoc" showFilters />
    </Card>
  );
}
