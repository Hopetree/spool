import { useState } from "react";
import { App as AntApp, Button, Card, Input, Select, Space, Switch, Table, Tag } from "antd";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, useNavigate } from "react-router-dom";
import dayjs from "dayjs";
import { ApiError } from "../api/client";
import {
  deleteSchedule,
  listSchedules,
  runSchedule,
  setScheduleEnabled,
  type Schedule,
} from "../api/schedules";
import PageHeader from "../components/PageHeader";
import { describeCron } from "../utils/cron";
import { relativeFuture } from "../utils/time";
import RowActions from "../components/RowActions";
import { useAuthStore } from "../stores/auth";

export default function Schedules() {
  const qc = useQueryClient();
  const { message } = AntApp.useApp();
  const navigate = useNavigate();
  const user = useAuthStore((s) => s.user);
  const isAdmin = user?.role === "admin";
  const canRun = isAdmin || user?.role === "operator";

  const [keyword, setKeyword] = useState("");
  const [enabled, setEnabled] = useState<string>();

  const { data, isLoading } = useQuery({
    queryKey: ["schedules", { keyword, enabled }],
    queryFn: () =>
      listSchedules({
        keyword,
        enabled: enabled === "true" ? true : enabled === "false" ? false : undefined,
      }),
  });
  const items = data?.items ?? [];

  const toggle = useMutation({
    mutationFn: (v: { id: string; value: boolean }) => setScheduleEnabled(v.id, v.value),
    onSuccess: () => {
      message.success("已更新");
      void qc.invalidateQueries({ queryKey: ["schedules"] });
    },
    onError: (err) => message.error(err instanceof ApiError ? err.message : "更新失败"),
  });

  const run = useMutation({
    mutationFn: runSchedule,
    onSuccess: (ex) => {
      message.success("已触发执行");
      void qc.invalidateQueries({ queryKey: ["schedules"] });
      navigate(`/executions/${ex.id}`);
    },
    onError: (err) => message.error(err instanceof ApiError ? err.message : "触发失败"),
  });

  const remove = useMutation({
    mutationFn: deleteSchedule,
    onSuccess: () => {
      message.success("已删除");
      void qc.invalidateQueries({ queryKey: ["schedules"] });
    },
    onError: (err) => message.error(err instanceof ApiError ? err.message : "删除失败"),
  });

  return (
    <div>
      <PageHeader
        title="定时任务"
        subtitle={`共 ${items.length} 个`}
        extra={
          isAdmin && (
            <Button type="primary" onClick={() => navigate("/schedules/new")}>
              新建定时任务
            </Button>
          )
        }
      />

      <Card size="small" style={{ marginBottom: 16 }} styles={{ body: { padding: 12 } }}>
        <Space wrap>
          <Input.Search allowClear placeholder="任务名称" style={{ width: 220 }} onSearch={setKeyword} />
          <Select
            allowClear
            placeholder="状态"
            style={{ width: 110 }}
            value={enabled}
            onChange={setEnabled}
            options={[
              { value: "true", label: "启用" },
              { value: "false", label: "停用" },
            ]}
          />
        </Space>
      </Card>

      <Table<Schedule>
        rowKey="id"
        size="small"
        loading={isLoading}
        dataSource={items}
        columns={[
          { title: "名称", render: (_, r) => <Link to={`/schedules/${r.id}`}>{r.name}</Link> },
          {
            title: "执行对象",
            render: (_, r) => (
              <Space size={4}>
                <Tag color={r.target_kind === "flow" ? "purple" : "geekblue"}>
                  {r.target_kind === "flow" ? "流程" : "工具"}
                </Tag>
                {r.target_name || r.target_ref_id}
              </Space>
            ),
          },
          {
            title: "cron",
            dataIndex: "cron_expr",
            width: 180,
            render: (v: string) => (
              <div>
                <code style={{ fontSize: 12 }}>{v}</code>
                <div style={{ fontSize: 11, color: "#8c8c8c" }}>{describeCron(v)}</div>
              </div>
            ),
          },
          {
            title: "下次运行",
            dataIndex: "next_run_at",
            width: 180,
            render: (v: number | null) =>
              v ? (
                <div>
                  {dayjs(v).format("YYYY-MM-DD HH:mm")}
                  <div style={{ fontSize: 11, color: "#8c8c8c" }}>{relativeFuture(v)}</div>
                </div>
              ) : (
                "—"
              ),
          },
          {
            title: "上次运行",
            dataIndex: "last_run_at",
            width: 170,
            render: (v: number | null) => (v ? dayjs(v).format("YYYY-MM-DD HH:mm") : "—"),
          },
          {
            title: "状态",
            dataIndex: "enabled",
            width: 90,
            render: (v: boolean, r) => (
              <Switch
                checked={v}
                disabled={!isAdmin}
                loading={toggle.isPending && toggle.variables?.id === r.id}
                onChange={(value) => toggle.mutate({ id: r.id, value })}
              />
            ),
          },
          {
            title: "操作",
            width: 190,
            render: (_, r) => (
              <RowActions
                inline={2}
                actions={[
                  ...(canRun
                    ? [{ key: "run", label: "立即执行", loading: run.isPending, onClick: () => run.mutate(r.id) }]
                    : []),
                  ...(isAdmin
                    ? [{ key: "edit", label: "编辑", onClick: () => navigate(`/schedules/${r.id}/edit`) }]
                    : []),
                  ...(isAdmin
                    ? [
                        {
                          key: "delete",
                          label: "删除",
                          danger: true,
                          onClick: () => remove.mutate(r.id),
                          confirm: { title: "确认删除该定时任务？" },
                        },
                      ]
                    : []),
                ]}
              />
            ),
          },
        ]}
      />
    </div>
  );
}
