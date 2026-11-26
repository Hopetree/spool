import { Card, Result } from "antd";

export default function Home() {
  return (
    <Card>
      <Result
        status="info"
        title="Spool 已就绪"
        subTitle="主机、工具、流程、定时任务将在后续里程碑上线。"
      />
    </Card>
  );
}
