import {
  CloudServerOutlined,
  DeploymentUnitOutlined,
  HistoryOutlined,
  HomeOutlined,
  KeyOutlined,
  LockOutlined,
  LogoutOutlined,
  ToolOutlined,
  UserOutlined,
} from "@ant-design/icons";
import { Avatar, Dropdown, Layout, Menu, Space, Typography } from "antd";
import type { MenuProps } from "antd";
import { Link, Outlet, useLocation, useNavigate } from "react-router-dom";
import { useAuthStore } from "../stores/auth";

const { Header, Sider, Content } = Layout;

export default function AppLayout() {
  const user = useAuthStore((s) => s.user);
  const logout = useAuthStore((s) => s.logout);
  const navigate = useNavigate();
  const location = useLocation();

  const section = `/${location.pathname.split("/")[1] ?? ""}`;
  const items: MenuProps["items"] = [
    { key: "/", icon: <HomeOutlined />, label: <Link to="/">首页</Link> },
    { key: "/hosts", icon: <CloudServerOutlined />, label: <Link to="/hosts">主机</Link> },
    { key: "/tools", icon: <ToolOutlined />, label: <Link to="/tools">工具</Link> },
    { key: "/flows", icon: <DeploymentUnitOutlined />, label: <Link to="/flows">流程</Link> },
    { key: "/executions", icon: <HistoryOutlined />, label: <Link to="/executions">执行历史</Link> },
  ];
  if (user?.role === "admin") {
    items.push({ key: "/credentials", icon: <KeyOutlined />, label: <Link to="/credentials">凭据</Link> });
  }

  return (
    <Layout style={{ minHeight: "100vh" }}>
      <Sider theme="dark" width={200}>
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
        <Menu theme="dark" mode="inline" selectedKeys={[section === "/" ? "/" : section]} items={items} />
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
