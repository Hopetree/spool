import { useState } from "react";
import { App as AntApp, Button, Input, Popconfirm, Select, Space, Switch, Table, Tag } from "antd";
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
      <Space style={{ marginBottom: 16 }} wrap>
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
        {isAdmin && (
          <Button type="primary" onClick={() => navigate("/schedules/new")}>
            新建定时任务
          </Button>
        )}
      </Space>

      <Table<Schedule>
        rowKey="id"
        loading={isLoading}
        dataSource={items}
        columns={[
          { title: "名称", render: (_, r) => <Link to={`/schedules/${r.id}`}>{r.name}</Link> },
          {
            title: "执行对象",
            render: (_, r) => (
              <Space size={4}>
                <Tag color={r.target_kind === "flow" ? "purple" : "geekblue"}>{r.target_kind}</Tag>
                {r.target_name || r.target_ref_id}
              </Space>
            ),
          },
          { title: "cron", dataIndex: "cron_expr", width: 140 },
          {
            title: "下次运行",
            dataIndex: "next_run_at",
            width: 170,
            render: (v: number | null) => (v ? dayjs(v).format("YYYY-MM-DD HH:mm") : "—"),
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
            render: (_, r) => (
              <Space>
                {canRun && (
                  <Button size="small" loading={run.isPending} onClick={() => run.mutate(r.id)}>
                    立即执行
                  </Button>
                )}
                {isAdmin && (
                  <Button size="small" onClick={() => navigate(`/schedules/${r.id}`)}>
                    编辑
                  </Button>
                )}
                {isAdmin && (
                  <Popconfirm title="确认删除该定时任务？" onConfirm={() => remove.mutate(r.id)}>
                    <Button size="small" danger>
                      删除
                    </Button>
                  </Popconfirm>
                )}
              </Space>
            ),
          },
        ]}
      />
    </div>
  );
}
