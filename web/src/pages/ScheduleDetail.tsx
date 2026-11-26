import {
  App as AntApp,
  Button,
  Card,
  Descriptions,
  Popconfirm,
  Space,
  Switch,
  Tag,
  Typography,
} from "antd";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useNavigate, useParams } from "react-router-dom";
import dayjs from "dayjs";
import { ApiError } from "../api/client";
import { listHosts } from "../api/hosts";
import { listChannels } from "../api/notify";
import {
  deleteSchedule,
  getSchedule,
  runSchedule,
  setScheduleEnabled,
} from "../api/schedules";
import ExecutionTable from "../components/ExecutionTable";
import { useAuthStore } from "../stores/auth";

const policyLabel: Record<string, string> = { skip: "上次未完成则跳过", queue: "排队执行" };
const eventLabel: Record<string, string> = { failure: "仅失败时", always: "每次都通知" };

export default function ScheduleDetail() {
  const { id } = useParams();
  const navigate = useNavigate();
  const qc = useQueryClient();
  const { message } = AntApp.useApp();
  const user = useAuthStore((s) => s.user);
  const isAdmin = user?.role === "admin";
  const canRun = isAdmin || user?.role === "operator";

  const { data: detail, isLoading } = useQuery({
    queryKey: ["schedule", id],
    queryFn: () => getSchedule(id!),
    enabled: !!id,
  });
  const { data: hostPage } = useQuery({
    queryKey: ["hosts", "schedule-detail"],
    queryFn: () => listHosts({ page: 1, page_size: 100 }),
  });
  const { data: channels = [] } = useQuery({
    queryKey: ["notify-channels"],
    queryFn: listChannels,
    enabled: isAdmin,
  });

  const hostName = (hostID: string) =>
    hostPage?.items.find((h) => h.id === hostID)?.name ?? hostID;
  const channelName = (channelID: string) =>
    channels.find((c) => c.id === channelID)?.name ?? channelID;

  const invalidate = () => {
    void qc.invalidateQueries({ queryKey: ["schedules"] });
    void qc.invalidateQueries({ queryKey: ["schedule", id] });
  };

  const run = useMutation({
    mutationFn: () => runSchedule(id!),
    onSuccess: (ex) => {
      message.success("已触发执行");
      navigate(`/executions/${ex.id}`);
    },
    onError: (err) => message.error(err instanceof ApiError ? err.message : "触发失败"),
  });

  const toggle = useMutation({
    mutationFn: (value: boolean) => setScheduleEnabled(id!, value),
    onSuccess: () => {
      message.success("已更新");
      invalidate();
    },
    onError: (err) => message.error(err instanceof ApiError ? err.message : "更新失败"),
  });

  const remove = useMutation({
    mutationFn: () => deleteSchedule(id!),
    onSuccess: () => {
      message.success("定时任务已删除");
      void qc.invalidateQueries({ queryKey: ["schedules"] });
      navigate("/schedules");
    },
    onError: (err) => message.error(err instanceof ApiError ? err.message : "删除失败"),
  });

  return (
    <div>
      <Space style={{ marginBottom: 16 }} wrap>
        <Button onClick={() => navigate("/schedules")}>返回列表</Button>
        <Typography.Title level={4} style={{ margin: 0 }}>
          {detail?.name ?? "定时任务"}
        </Typography.Title>
        {detail && <Tag>{detail.cron_expr}</Tag>}
        {canRun && (
          <Button loading={run.isPending} onClick={() => run.mutate()}>
            立即执行
          </Button>
        )}
        {isAdmin && <Button onClick={() => navigate(`/schedules/${id}/edit`)}>编辑</Button>}
        {isAdmin && (
          <Space size={4}>
            <span>启用</span>
            <Switch checked={detail?.enabled} loading={toggle.isPending} onChange={(v) => toggle.mutate(v)} />
          </Space>
        )}
        {isAdmin && (
          <Popconfirm title="确认删除该定时任务？" onConfirm={() => remove.mutate()}>
            <Button danger loading={remove.isPending}>
              删除任务
            </Button>
          </Popconfirm>
        )}
      </Space>

      <Card loading={isLoading} style={{ marginBottom: 16 }}>
        {detail && (
          <Descriptions
            column={3}
            size="small"
            items={[
              {
                key: "target",
                label: "执行对象",
                children: (
                  <Space size={4}>
                    <Tag color={detail.target_kind === "flow" ? "purple" : "geekblue"}>
                      {detail.target_kind === "flow" ? "流程" : "工具"}
                    </Tag>
                    {detail.target_name || detail.target_ref_id}
                  </Space>
                ),
              },
              { key: "cron", label: "cron", children: detail.cron_expr },
              {
                key: "policy",
                label: "并发策略",
                children: policyLabel[detail.concurrency_policy] ?? detail.concurrency_policy,
              },
              {
                key: "next",
                label: "下次运行",
                children: detail.next_run_at ? dayjs(detail.next_run_at).format("YYYY-MM-DD HH:mm") : "—",
              },
              {
                key: "last",
                label: "上次运行",
                children: detail.last_run_at ? dayjs(detail.last_run_at).format("YYYY-MM-DD HH:mm") : "—",
              },
              {
                key: "hosts",
                label: "目标主机",
                children: (
                  <Space wrap>
                    {detail.host_ids.map((h) => (
                      <Tag key={h}>{hostName(h)}</Tag>
                    ))}
                  </Space>
                ),
              },
              {
                key: "params",
                label: "参数",
                children:
                  Object.keys(detail.params).length > 0 ? (
                    <Space wrap>
                      {Object.entries(detail.params).map(([k, v]) => (
                        <Tag key={k}>
                          {k}={v || "（空）"}
                        </Tag>
                      ))}
                    </Space>
                  ) : (
                    "—"
                  ),
              },
              {
                key: "notify",
                label: "通知",
                children:
                  detail.notify_channel_ids.length > 0 ? (
                    <Space wrap>
                      <Tag>{eventLabel[detail.notify_events] ?? detail.notify_events}</Tag>
                      {detail.notify_channel_ids.map((c) => (
                        <Tag key={c} color="blue">
                          {channelName(c)}
                        </Tag>
                      ))}
                    </Space>
                  ) : (
                    "—"
                  ),
              },
            ]}
          />
        )}
      </Card>

      <Card title="执行历史">
        {id && <ExecutionTable scheduleID={id} showFilters />}
      </Card>
    </div>
  );
}
