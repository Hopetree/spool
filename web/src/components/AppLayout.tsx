import {
  ClockCircleOutlined,
  CloudServerOutlined,
  DeploymentUnitOutlined,
  FolderOpenOutlined,
  HomeOutlined,
  LockOutlined,
  LogoutOutlined,
  SettingOutlined,
  ToolOutlined,
  UserOutlined,
} from "@ant-design/icons";
import { Avatar, Dropdown, Layout, Menu, Space, Typography } from "antd";
import type { MenuProps } from "antd";
import { Link, Outlet, useLocation, useNavigate } from "react-router-dom";
import { useAuthStore } from "../stores/auth";

const { Header, Sider, Content } = Layout;

const groupKeys = ["grp-hosts", "grp-tools", "grp-flows", "grp-automation", "grp-system"];

function selectedKey(pathname: string): string {
  if (pathname === "/") return "/";
  const exact = [
    "/tools/executions",
    "/tools",
    "/flows/executions",
    "/flows",
    "/adhoc/history",
    "/adhoc",
    "/hosts",
    "/credentials",
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

export default function AppLayout() {
  const user = useAuthStore((s) => s.user);
  const logout = useAuthStore((s) => s.logout);
  const navigate = useNavigate();
  const location = useLocation();
  const isAdmin = user?.role === "admin";

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
    { key: "/adhoc", label: <Link to="/adhoc">临时命令</Link> },
    { key: "/adhoc/history", label: <Link to="/adhoc/history">临时命令历史</Link> },
  ];
  const systemChildren: MenuProps["items"] = [
    { key: "/users", label: <Link to="/users">用户</Link> },
    { key: "/channels", label: <Link to="/channels">通知渠道</Link> },
    { key: "/audit", label: <Link to="/audit">审计日志</Link> },
  ];

  const items: MenuProps["items"] = [
    { key: "/", icon: <HomeOutlined />, label: <Link to="/">工作台</Link> },
    { key: "grp-hosts", icon: <CloudServerOutlined />, label: "主机与凭据", children: hostChildren },
    { key: "grp-tools", icon: <ToolOutlined />, label: "工具", children: toolChildren },
    { key: "grp-flows", icon: <DeploymentUnitOutlined />, label: "流程", children: flowChildren },
    { key: "grp-automation", icon: <ClockCircleOutlined />, label: "自动化", children: automationChildren },
    { key: "/files", icon: <FolderOpenOutlined />, label: <Link to="/files">文件分发</Link> },
  ];
  if (isAdmin) {
    items.push({ key: "grp-system", icon: <SettingOutlined />, label: "系统管理", children: systemChildren });
  }

  return (
    <Layout style={{ minHeight: "100vh" }}>
      <Sider theme="dark" width={220}>
        <div
          style={{
            height: 56,
            display: "flex",
            alignItems: "center",
            justifyContent: "center",
            color: "#fff",
            fontSize: 20,
            fontWeight: 600,
            letterSpacing: 3,
          }}
        >
          SPOOL
        </div>
        <Menu
          theme="dark"
          mode="inline"
          selectedKeys={[selectedKey(location.pathname)]}
          defaultOpenKeys={groupKeys}
          items={items}
        />
      </Sider>
      <Layout>
        <Header
          style={{
            background: "#fff",
            display: "flex",
            alignItems: "center",
            justifyContent: "space-between",
            padding: "0 24px",
          }}
        >
          <Typography.Text type="secondary">内网运维平台</Typography.Text>
          <Dropdown
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
            <Space style={{ cursor: "pointer" }}>
              <Avatar size="small" icon={<UserOutlined />} />
              {user?.display_name || user?.username}
            </Space>
          </Dropdown>
        </Header>
        <Content style={{ margin: 24 }}>
          <Outlet />
        </Content>
      </Layout>
    </Layout>
  );
}
