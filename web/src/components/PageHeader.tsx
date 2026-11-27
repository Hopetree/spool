import type { CSSProperties, ReactNode } from "react";
import { Breadcrumb, Button, Space, Typography } from "antd";
import { Link, useLocation } from "react-router-dom";

export interface Crumb {
  title: ReactNode;
  to?: string;
}

const sectionCrumbs: Record<string, Crumb[]> = {
  "/": [{ title: "首页", to: "/" }, { title: "工作台" }],
  "/hosts": [{ title: "主机与凭据", to: "/hosts" }, { title: "主机列表" }],
  "/hosts/groups": [{ title: "主机与凭据", to: "/hosts" }, { title: "分组管理" }],
  "/credentials": [{ title: "主机与凭据", to: "/hosts" }, { title: "凭据" }],
  "/tools": [{ title: "工具", to: "/tools" }, { title: "工具列表" }],
  "/tools/executions": [{ title: "工具", to: "/tools" }, { title: "工具历史" }],
  "/flows": [{ title: "流程", to: "/flows" }, { title: "流程列表" }],
  "/flows/executions": [{ title: "流程", to: "/flows" }, { title: "流程历史" }],
  "/schedules": [{ title: "自动化", to: "/schedules" }, { title: "定时任务" }],
  "/schedules/executions": [{ title: "自动化", to: "/schedules" }, { title: "任务历史" }],
  "/adhoc": [{ title: "自动化", to: "/schedules" }, { title: "临时命令" }],
  "/files": [{ title: "首页", to: "/" }, { title: "文件分发" }],
  "/users": [{ title: "系统管理", to: "/users" }, { title: "用户" }],
  "/channels": [{ title: "系统管理", to: "/users" }, { title: "通知渠道" }],
  "/audit": [{ title: "系统管理", to: "/users" }, { title: "审计日志" }],
  "/password": [{ title: "系统管理", to: "/users" }, { title: "修改密码" }],
};

function autoCrumbs(pathname: string): Crumb[] {
  if (sectionCrumbs[pathname]) return sectionCrumbs[pathname];
  const exact = Object.keys(sectionCrumbs)
    .filter((key) => key !== "/" && pathname.startsWith(key + "/"))
    .sort((a, b) => b.length - a.length)[0];
  return exact ? sectionCrumbs[exact] : [];
}

interface Props {
  title: ReactNode;
  subtitle?: ReactNode;
  extra?: ReactNode;
  back?: { label?: string; onClick: () => void };
  crumbs?: Crumb[];
  style?: CSSProperties;
}

/**
 * Standard page header: breadcrumb, title with optional back button and
 * right-aligned actions.
 */
export default function PageHeader({ title, subtitle, extra, back, crumbs, style }: Props) {
  const location = useLocation();
  const trail = crumbs ?? autoCrumbs(location.pathname);
  return (
    <div style={{ marginBottom: 16, ...style }}>
      {trail.length > 0 && (
        <Breadcrumb
          style={{ marginBottom: 8, fontSize: 12 }}
          items={trail.map((c) => ({
            title: c.to ? <Link to={c.to}>{c.title}</Link> : c.title,
          }))}
        />
      )}
      <div
        style={{
          display: "flex",
          alignItems: "flex-start",
          justifyContent: "space-between",
          gap: 16,
          flexWrap: "wrap",
        }}
      >
        <div style={{ display: "flex", alignItems: "center", gap: 12, minWidth: 0 }}>
          {back && (
            <Button onClick={back.onClick} style={{ flex: "none" }}>
              {back.label ?? "返回"}
            </Button>
          )}
          <Space size={10} wrap align="center">
            <Typography.Title level={4} style={{ margin: 0 }}>
              {title}
            </Typography.Title>
            {subtitle && (
              <Typography.Text type="secondary" style={{ fontSize: 13 }}>
                {subtitle}
              </Typography.Text>
            )}
          </Space>
        </div>
        {extra && (
          <Space wrap size={8}>
            {extra}
          </Space>
        )}
      </div>
    </div>
  );
}
