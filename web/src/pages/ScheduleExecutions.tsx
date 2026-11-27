import { Card } from "antd";
import ExecutionTable from "../components/ExecutionTable";
import PageHeader from "../components/PageHeader";

export default function ScheduleExecutions() {
  return (
    <div>
      <PageHeader
        title="任务历史"
        subtitle="包含定时触发和从定时任务手动触发的执行记录"
      />
      <Card>
        <ExecutionTable scheduledOnly showSchedule showKind showVersion showFilters />
      </Card>
    </div>
  );
}
