import { useState } from "react";
import { useNavigate } from "react-router-dom";
import { Card, Form, Input, Button, Typography, Alert } from "antd";
import { CloudServerOutlined, SafetyOutlined } from "@ant-design/icons";
import { login, setToken } from "../api/client";
import { useT } from "../i18n";

export default function Login() {
  const navigate = useNavigate();
  const t = useT();
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [need2fa, setNeed2fa] = useState(false);

  async function onFinish(values: { username: string; password: string; totp?: string }) {
    setLoading(true);
    setError(null);
    try {
      const token = await login(values.username, values.password, values.totp);
      setToken(token);
      navigate("/sites");
    } catch (e: any) {
      const code = e?.response?.data?.error;
      if (code === "totp_required") {
        setNeed2fa(true);
        setError(null);
      } else if (need2fa) {
        setError(t("login.bad2fa"));
      } else {
        setError(t("login.bad"));
      }
    } finally {
      setLoading(false);
    }
  }

  return (
    <div style={{ display: "flex", minHeight: "100vh", alignItems: "center", justifyContent: "center", background: "#f0f2f5" }}>
      <Card style={{ width: 360 }}>
        <Typography.Title level={3} style={{ textAlign: "center" }}>
          <CloudServerOutlined /> WP Manager
        </Typography.Title>
        {error && <Alert type="error" message={error} style={{ marginBottom: 16 }} />}
        {need2fa && (
          <Alert
            type="info"
            showIcon
            icon={<SafetyOutlined />}
            message={t("login.2faPrompt")}
            style={{ marginBottom: 16 }}
          />
        )}
        <Form layout="vertical" onFinish={onFinish} initialValues={{ username: "admin" }}>
          <Form.Item name="username" label="Username" rules={[{ required: true }]}>
            <Input autoFocus={!need2fa} disabled={need2fa} />
          </Form.Item>
          <Form.Item name="password" label="Password" rules={[{ required: true }]}>
            <Input.Password disabled={need2fa} />
          </Form.Item>
          {need2fa && (
            <Form.Item name="totp" label={t("login.code")} rules={[{ required: true, message: t("login.codeRequired") }]}>
              <Input
                autoFocus
                inputMode="numeric"
                maxLength={6}
                placeholder="123456"
                prefix={<SafetyOutlined />}
              />
            </Form.Item>
          )}
          <Button type="primary" htmlType="submit" block loading={loading}>
            {need2fa ? t("login.verify") : t("login.signin")}
          </Button>
          {need2fa && (
            <Button
              type="link"
              block
              onClick={() => {
                setNeed2fa(false);
                setError(null);
              }}
            >
              {t("login.back")}
            </Button>
          )}
        </Form>
      </Card>
    </div>
  );
}
