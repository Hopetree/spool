import { useMemo } from "react";
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
import dayjs from "dayjs";
import { listHosts } from "../api/hosts";
import { listTools } from "../api/tools";
import { listFlows } from "../api/flows";
import { listSchedules } from "../api/schedules";
import { listExecutions, type Execution } from "../api/executions";
import ExecutionTable from "../components/ExecutionTable";
import PageHeader from "../components/PageHeader";
import { useAuthStore } from "../stores/auth";

const trendDays = 7;

interface DayBucket {
  key: string;
  label: string;
  success: number;
  failed: number;
  other: number;
}

function buildTrend(items: Execution[]): { buckets: DayBucket[]; total: number; success: number; failed: number } {
  const start = dayjs().startOf("day").subtract(trendDays - 1, "day");
  const buckets: DayBucket[] = Array.from({ length: trendDays }, (_, i) => {
    const day = start.add(i, "day");
    return { key: day.format("YYYY-MM-DD"), label: day.format("MM-DD"), success: 0, failed: 0, other: 0 };
  });
  const byKey = new Map(buckets.map((b) => [b.key, b]));
  let total = 0;
  let success = 0;
  let failed = 0;
  for (const item of items) {
    const bucket = byKey.get(dayjs(item.created_at).format("YYYY-MM-DD"));
    if (!bucket) continue;
    total += 1;
    if (item.status === "success") {
      bucket.success += 1;
      success += 1;
    } else if (item.status === "failed") {
      bucket.failed += 1;
      failed += 1;
    } else {
      bucket.other += 1;
    }
  }
  return { buckets, total, success, failed };
}

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
  const { data: execPage } = useQuery({
    queryKey: ["executions", "home-trend"],
    queryFn: () => listExecutions({ page: 1, page_size: 100 }),
    refetchInterval: 30000,
  });

  const hosts = hostPage?.items ?? [];
  const hostEnabled = hosts.filter((h) => h.enabled).length;
  const tools = toolPage?.items ?? [];
  const flows = flowPage?.items ?? [];
  const schedules = schedulePage?.items ?? [];
  const trend = useMemo(() => buildTrend(execPage?.items ?? []), [execPage]);

  const barMax = Math.max(1, ...trend.buckets.map((b) => b.success + b.failed + b.other));
  const rate = trend.total ? Math.round((trend.success / trend.total) * 1000) / 10 : null;

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
      <PageHeader title="工作台" subtitle={`内网运维总览 · ${dayjs().format("YYYY-MM-DD")}`} />

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

      <Card
        style={{ marginTop: 16 }}
        title="最近 7 天执行"
        extra={
          <Typography.Text type="secondary" style={{ fontSize: 12 }}>
            {trend.buckets[0]?.label} ~ {trend.buckets[trend.buckets.length - 1]?.label}
          </Typography.Text>
        }
        styles={{ body: { padding: "16px 20px 12px" } }}
      >
        <div style={{ display: "flex", gap: 24, alignItems: "flex-end" }}>
          <div style={{ flex: 1, minWidth: 0 }}>
            <div style={{ display: "flex", alignItems: "flex-end", gap: 12, height: 132 }}>
              {trend.buckets.map((b) => {
                const total = b.success + b.failed + b.other;
                return (
                  <div
                    key={b.key}
                    title={`${b.label}：成功 ${b.success} / 失败 ${b.failed} / 其他 ${b.other}`}
                    style={{ flex: 1, display: "flex", flexDirection: "column", justifyContent: "flex-end", gap: 6 }}
                  >
                    <div
                      style={{
                        height: Math.max(4, Math.round((total / barMax) * 104)),
                        display: "flex",
                        flexDirection: "column-reverse",
                        borderRadius: 4,
                        overflow: "hidden",
                        background: "#f0f0f0",
                      }}
                    >
                      {b.success > 0 && (
                        <div style={{ height: `${(b.success / Math.max(total, 1)) * 100}%`, background: "#52c41a" }} />
                      )}
                      {b.failed > 0 && (
                        <div style={{ height: `${(b.failed / Math.max(total, 1)) * 100}%`, background: "#ff4d4f" }} />
                      )}
                      {b.other > 0 && (
                        <div style={{ height: `${(b.other / Math.max(total, 1)) * 100}%`, background: "#bfbfbf" }} />
                      )}
                    </div>
                    <div style={{ textAlign: "center", fontSize: 11, color: "#8c8c8c" }}>{b.label}</div>
                  </div>
                );
              })}
            </div>
          </div>
          <div style={{ display: "flex", gap: 24, flex: "none" }}>
            <Statistic title="总执行" value={trend.total} valueStyle={{ fontSize: 22 }} />
            <Statistic
              title="成功率"
              value={rate ?? "—"}
              suffix={rate === null ? undefined : "%"}
              valueStyle={{ fontSize: 22, color: rate !== null && rate < 90 ? "#ff4d4f" : "#52c41a" }}
            />
            <Statistic
              title="失败"
              value={trend.failed}
              valueStyle={{ fontSize: 22, color: trend.failed > 0 ? "#ff4d4f" : undefined }}
            />
          </div>
        </div>
        <Space size={16} style={{ marginTop: 8, fontSize: 12, color: "#8c8c8c" }}>
          <span>
            <span style={{ display: "inline-block", width: 8, height: 8, borderRadius: 2, background: "#52c41a", marginRight: 4 }} />
            成功
          </span>
          <span>
            <span style={{ display: "inline-block", width: 8, height: 8, borderRadius: 2, background: "#ff4d4f", marginRight: 4 }} />
            失败
          </span>
          <span>
            <span style={{ display: "inline-block", width: 8, height: 8, borderRadius: 2, background: "#bfbfbf", marginRight: 4 }} />
            其他
          </span>
        </Space>
      </Card>

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
