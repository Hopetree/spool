import { App as AntApp, Button, Card, Form, Input } from "antd";
import { useNavigate } from "react-router-dom";
import { ApiError } from "../api/client";
import PageHeader from "../components/PageHeader";
import { useAuthStore } from "../stores/auth";

interface PasswordForm {
  old_password: string;
  new_password: string;
  confirm: string;
}

export default function Password() {
  const changePassword = useAuthStore((s) => s.changePassword);
  const navigate = useNavigate();
  const { message } = AntApp.useApp();

  const onFinish = async (values: PasswordForm) => {
    try {
      await changePassword(values.old_password, values.new_password);
      message.success("密码已更新");
      navigate("/", { replace: true });
    } catch (err) {
      message.error(err instanceof ApiError ? err.message : "修改失败，请稍后重试");
    }
  };

  return (
    <div style={{ maxWidth: 460 }}>
      <PageHeader title="修改密码" />
      <Card>
      <Form<PasswordForm> layout="vertical" onFinish={onFinish} autoComplete="off">
        <Form.Item label="原密码" name="old_password" rules={[{ required: true, message: "请输入原密码" }]}>
          <Input.Password />
        </Form.Item>
        <Form.Item
          label="新密码"
          name="new_password"
          rules={[
            { required: true, message: "请输入新密码" },
            { min: 8, message: "至少 8 个字符" },
            {
              pattern: /^(?=.*[A-Za-z])(?=.*\d).+$/,
              message: "必须同时包含字母和数字",
            },
          ]}
        >
          <Input.Password />
        </Form.Item>
        <Form.Item
          label="确认新密码"
          name="confirm"
          dependencies={["new_password"]}
          rules={[
            { required: true, message: "请再次输入新密码" },
            ({ getFieldValue }) => ({
              validator(_, value) {
                if (!value || getFieldValue("new_password") === value) {
                  return Promise.resolve();
                }
                return Promise.reject(new Error("两次输入的密码不一致"));
              },
            }),
          ]}
        >
          <Input.Password />
        </Form.Item>
        <Button type="primary" htmlType="submit">
          保存
        </Button>
      </Form>
      </Card>
    </div>
  );
}
