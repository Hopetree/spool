import { useState } from "react";
import { useNavigate } from "react-router-dom";
import { Alert, Button, Card, Form, Input, Typography } from "antd";
import { ApiError } from "../api/client";
import { useAuthStore } from "../stores/auth";

interface LoginForm {
  username: string;
  password: string;
}

export default function Login() {
  const login = useAuthStore((s) => s.login);
  const navigate = useNavigate();
  const [error, setError] = useState<string>();
  const [loading, setLoading] = useState(false);

  const onFinish = async (values: LoginForm) => {
    setLoading(true);
    setError(undefined);
    try {
      const user = await login(values.username, values.password);
      navigate(user.must_change_password ? "/password" : "/", { replace: true });
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "登录失败，请稍后重试");
    } finally {
      setLoading(false);
    }
  };

  return (
    <div
      style={{
        minHeight: "100vh",
        display: "flex",
        alignItems: "center",
        justifyContent: "center",
        background: "#f0f2f5",
      }}
    >
      <Card style={{ width: 380 }}>
        <Typography.Title level={2} style={{ textAlign: "center", marginBottom: 4 }}>
          Spool
        </Typography.Title>
        <Typography.Paragraph type="secondary" style={{ textAlign: "center", marginBottom: 24 }}>
          内网运维平台
        </Typography.Paragraph>
        {error && <Alert type="error" message={error} style={{ marginBottom: 16 }} showIcon />}
        <Form<LoginForm> layout="vertical" onFinish={onFinish} autoComplete="off">
          <Form.Item label="用户名" name="username" rules={[{ required: true, message: "请输入用户名" }]}>
            <Input autoFocus />
          </Form.Item>
          <Form.Item label="密码" name="password" rules={[{ required: true, message: "请输入密码" }]}>
            <Input.Password />
          </Form.Item>
          <Button type="primary" htmlType="submit" block loading={loading}>
            登录
          </Button>
        </Form>
      </Card>
    </div>
  );
}
