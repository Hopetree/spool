import { useEffect, useState } from "react";
import {
  ClockCircleOutlined,
  CloudServerOutlined,
  DeploymentUnitOutlined,
  DownOutlined,
  FolderOpenOutlined,
  HomeOutlined,
  LockOutlined,
  LogoutOutlined,
  SettingOutlined,
  ToolOutlined,
  UserOutlined,
} from "@ant-design/icons";
import { Avatar, Dropdown, Layout, Menu } from "antd";
import type { MenuProps } from "antd";
import { Link, Outlet, useLocation, useNavigate } from "react-router-dom";
import { useAuthStore } from "../stores/auth";

const { Sider, Content } = Layout;

function selectedKey(pathname: string): string {
  if (pathname === "/") return "/";
  const exact = [
    "/tools/executions",
    "/tools",
    "/flows/executions",
    "/flows",
    "/adhoc",
    "/hosts",
    "/credentials",
    "/schedules/executions",
    "/schedules",
    "/files",
    "/users",
    "/channels",
    "/audit",
    "/password",
  ];
  if (exact.includes(pathname)) return pathname;
  if (pathname.startsWith("/tools")) return "/tools";
  if (pathname.startsWith("/flows")) return "/flows";
  if (pathname.startsWith("/hosts")) return "/hosts";
  if (pathname.startsWith("/adhoc")) return "/adhoc";
  if (pathname.startsWith("/schedules")) return "/schedules";
  return "";
}

const groupOfKey: Record<string, string> = {
  "/hosts": "grp-hosts",
  "/credentials": "grp-hosts",
  "/tools": "grp-tools",
  "/tools/executions": "grp-tools",
  "/flows": "grp-flows",
  "/flows/executions": "grp-flows",
  "/schedules": "grp-automation",
  "/schedules/executions": "grp-automation",
  "/adhoc": "grp-automation",
  "/users": "grp-system",
  "/channels": "grp-system",
  "/audit": "grp-system",
};

export default function AppLayout() {
  const user = useAuthStore((s) => s.user);
  const logout = useAuthStore((s) => s.logout);
  const navigate = useNavigate();
  const location = useLocation();
  const isAdmin = user?.role === "admin";

  const current = selectedKey(location.pathname);
  const activeGroup = groupOfKey[current];
  const [openKeys, setOpenKeys] = useState<string[]>(() => (activeGroup ? [activeGroup] : []));

  // Only keep the section of the current page open by default; the user can
  // still expand other sections manually.
  useEffect(() => {
    if (activeGroup) {
      setOpenKeys((keys) => (keys.includes(activeGroup) ? keys : [...keys, activeGroup]));
    }
  }, [activeGroup]);

  const hostChildren: MenuProps["items"] = [{ key: "/hosts", label: <Link to="/hosts">主机列表</Link> }];
  if (isAdmin) {
    hostChildren.push({ key: "/credentials", label: <Link to="/credentials">凭据</Link> });
  }
  const toolChildren: MenuProps["items"] = [
    { key: "/tools", label: <Link to="/tools">工具列表</Link> },
    { key: "/tools/executions", label: <Link to="/tools/executions">工具执行历史</Link> },
  ];
  const flowChildren: MenuProps["items"] = [
    { key: "/flows", label: <Link to="/flows">流程列表</Link> },
    { key: "/flows/executions", label: <Link to="/flows/executions">流程执行历史</Link> },
  ];
  const automationChildren: MenuProps["items"] = [
    { key: "/schedules", label: <Link to="/schedules">定时任务</Link> },
    { key: "/schedules/executions", label: <Link to="/schedules/executions">定时任务历史</Link> },
    { key: "/adhoc", label: <Link to="/adhoc">临时命令</Link> },
  ];
  const systemChildren: MenuProps["items"] = [
    { key: "/users", label: <Link to="/users">用户</Link> },
    { key: "/channels", label: <Link to="/channels">通知渠道</Link> },
    { key: "/audit", label: <Link to="/audit">审计日志</Link> },
  ];

  const items: MenuProps["items"] = [
    { key: "/", icon: <HomeOutlined />, label: <Link to="/">工作台</Link> },
    {
      key: "grp-hosts",
      icon: <CloudServerOutlined />,
      label: <Link to="/hosts">主机与凭据</Link>,
      children: hostChildren,
    },
    { key: "grp-tools", icon: <ToolOutlined />, label: <Link to="/tools">工具</Link>, children: toolChildren },
    { key: "grp-flows", icon: <DeploymentUnitOutlined />, label: <Link to="/flows">流程</Link>, children: flowChildren },
    {
      key: "grp-automation",
      icon: <ClockCircleOutlined />,
      label: <Link to="/schedules">自动化</Link>,
      children: automationChildren,
    },
    { key: "/files", icon: <FolderOpenOutlined />, label: <Link to="/files">文件分发</Link> },
  ];
  if (isAdmin) {
    items.push({
      key: "grp-system",
      icon: <SettingOutlined />,
      label: <Link to="/users">系统管理</Link>,
      children: systemChildren,
    });
  }

  return (
    <Layout style={{ minHeight: "100vh" }}>
      <Sider
        theme="dark"
        width={220}
        style={{ height: "100vh", position: "sticky", top: 0, overflow: "hidden" }}
      >
        <div style={{ height: "100vh", display: "flex", flexDirection: "column" }}>
          <Link
            to="/"
            style={{
              height: 56,
              display: "flex",
              alignItems: "center",
              justifyContent: "center",
              color: "#fff",
              fontSize: 20,
              fontWeight: 600,
              letterSpacing: 3,
              flex: "none",
            }}
          >
            SPOOL
          </Link>
          <div style={{ flex: 1, minHeight: 0, overflowY: "auto" }}>
            <Menu
              theme="dark"
              mode="inline"
              selectedKeys={[current]}
              openKeys={openKeys}
              onOpenChange={(keys) => setOpenKeys(keys)}
              items={items}
            />
          </div>
          <Dropdown
            placement="topLeft"
            trigger={["click"]}
            menu={{
              items: [
                {
                  key: "password",
                  icon: <LockOutlined />,
                  label: "修改密码",
                  onClick: () => navigate("/password"),
                },
                {
                  key: "logout",
                  icon: <LogoutOutlined />,
                  label: "退出登录",
                  onClick: async () => {
                    await logout();
                    navigate("/login", { replace: true });
                  },
                },
              ],
            }}
          >
            <div
              style={{
                flex: "none",
                display: "flex",
                alignItems: "center",
                gap: 8,
                padding: "12px 16px",
                borderTop: "1px solid rgba(255,255,255,0.08)",
                color: "rgba(255,255,255,0.85)",
                cursor: "pointer",
              }}
            >
              <Avatar size="small" icon={<UserOutlined />} />
              <span style={{ flex: 1, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>
                {user?.display_name || user?.username}
              </span>
              <DownOutlined style={{ fontSize: 10, color: "rgba(255,255,255,0.45)" }} />
            </div>
          </Dropdown>
        </div>
      </Sider>
      <Layout>
        <Content style={{ padding: 16 }}>
          <Outlet />
        </Content>
      </Layout>
    </Layout>
  );
}
