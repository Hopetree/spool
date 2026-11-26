import { HomeOutlined, LockOutlined, LogoutOutlined, UserOutlined } from "@ant-design/icons";
import { Avatar, Dropdown, Layout, Menu, Space, Typography } from "antd";
import { Link, Outlet, useLocation, useNavigate } from "react-router-dom";
import { useAuthStore } from "../stores/auth";

const { Header, Sider, Content } = Layout;

export default function AppLayout() {
  const user = useAuthStore((s) => s.user);
  const logout = useAuthStore((s) => s.logout);
  const navigate = useNavigate();
  const location = useLocation();

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
        <Menu
          theme="dark"
          mode="inline"
          selectedKeys={[location.pathname === "/" ? "/" : `/${location.pathname.split("/")[1]}`]}
          items={[{ key: "/", icon: <HomeOutlined />, label: <Link to="/">首页</Link> }]}
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
