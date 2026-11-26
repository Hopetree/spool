import { Card, Typography } from "antd";
import ExecutionTable from "../components/ExecutionTable";

export default function FlowExecutions() {
  return (
    <Card>
      <Typography.Title level={4}>流程执行历史</Typography.Title>
      <Typography.Paragraph type="secondary">
        这里仅显示流程运行记录；每次运行包含多个主机的步骤执行与输出。
      </Typography.Paragraph>
      <ExecutionTable kind="flow" showFilters />
    </Card>
  );
}
