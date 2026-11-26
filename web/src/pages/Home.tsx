import {
  ClockCircleOutlined,
  CloudServerOutlined,
  DeploymentUnitOutlined,
  ThunderboltOutlined,
  ToolOutlined,
} from "@ant-design/icons";
import { Button, Card, Col, Row, Space, Statistic, Typography } from "antd";
import { useQuery } from "@tanstack/react-query";
import { useNavigate } from "react-router-dom";
import { listHosts } from "../api/hosts";
import { listTools } from "../api/tools";
import { listFlows } from "../api/flows";
import { listSchedules } from "../api/schedules";
import ExecutionTable from "../components/ExecutionTable";
import { useAuthStore } from "../stores/auth";

export default function Home() {
  const navigate = useNavigate();
  const user = useAuthStore((s) => s.user);
  const canWrite = user?.role === "admin";

  const { data: hostPage } = useQuery({
    queryKey: ["hosts", "home"],
    queryFn: () => listHosts({ page: 1, page_size: 100 }),
  });
  const { data: toolPage } = useQuery({
    queryKey: ["tools", "home"],
    queryFn: () => listTools({ page: 1, page_size: 100 }),
  });
  const { data: flowPage } = useQuery({ queryKey: ["flows", "home"], queryFn: () => listFlows() });
  const { data: schedulePage } = useQuery({ queryKey: ["schedules", "home"], queryFn: () => listSchedules() });

  const hosts = hostPage?.items ?? [];
  const hostEnabled = hosts.filter((h) => h.enabled).length;
  const tools = toolPage?.items ?? [];
  const flows = flowPage?.items ?? [];
  const schedules = schedulePage?.items ?? [];

  const stats = [
    {
      key: "hosts",
      title: "主机",
      value: hosts.length,
      suffix: hostEnabled !== hosts.length ? `启用 ${hostEnabled}` : undefined,
      icon: <CloudServerOutlined />,
      color: "#1677ff",
      onClick: () => navigate("/hosts"),
    },
    {
      key: "tools",
      title: "工具",
      value: tools.length,
      suffix: undefined as string | undefined,
      icon: <ToolOutlined />,
      color: "#52c41a",
      onClick: () => navigate("/tools"),
    },
    {
      key: "flows",
      title: "流程",
      value: flows.length,
      suffix: undefined as string | undefined,
      icon: <DeploymentUnitOutlined />,
      color: "#722ed1",
      onClick: () => navigate("/flows"),
    },
    {
      key: "schedules",
      title: "定时任务",
      value: schedules.length,
      suffix: schedules.some((s) => s.enabled) ? `启用 ${schedules.filter((s) => s.enabled).length}` : undefined,
      icon: <ClockCircleOutlined />,
      color: "#fa8c16",
      onClick: () => navigate("/schedules"),
    },
  ];

  return (
    <div>
      <Row gutter={16}>
        {stats.map((s) => (
          <Col key={s.key} xs={12} md={6}>
            <Card hoverable onClick={s.onClick} styles={{ body: { padding: 20 } }}>
              <Space align="start" size={16}>
                <span
                  style={{
                    width: 40,
                    height: 40,
                    borderRadius: 10,
                    display: "inline-flex",
                    alignItems: "center",
                    justifyContent: "center",
                    fontSize: 20,
                    color: s.color,
                    background: `${s.color}14`,
                  }}
                >
                  {s.icon}
                </span>
                <Statistic
                  title={s.title}
                  value={s.value}
                  valueStyle={{ fontSize: 24, fontWeight: 600 }}
                  suffix={s.suffix ? <span style={{ fontSize: 12, color: "#8c8c8c", marginLeft: 6 }}>{s.suffix}</span> : undefined}
                />
              </Space>
            </Card>
          </Col>
        ))}
      </Row>

      <Card style={{ marginTop: 16 }} title="快捷操作" styles={{ body: { padding: 16 } }}>
        <Space wrap>
          <Button icon={<ThunderboltOutlined />} onClick={() => navigate("/adhoc")}>
            临时命令
          </Button>
          <Button onClick={() => navigate("/tools/new")} disabled={!canWrite}>
            新建工具
          </Button>
          <Button onClick={() => navigate("/flows/new")} disabled={!canWrite}>
            新建流程
          </Button>
          <Button onClick={() => navigate("/schedules/new")} disabled={!canWrite}>
            新建定时任务
          </Button>
          <Button onClick={() => navigate("/files")}>文件分发</Button>
        </Space>
      </Card>

      <Card
        style={{ marginTop: 16 }}
        title="最近执行"
        extra={
          <Space size={12}>
            <Typography.Link onClick={() => navigate("/tools/executions")}>工具历史</Typography.Link>
            <Typography.Link onClick={() => navigate("/flows/executions")}>流程历史</Typography.Link>
          </Space>
        }
      >
        <ExecutionTable pageSize={8} showKind />
      </Card>
    </div>
  );
}
