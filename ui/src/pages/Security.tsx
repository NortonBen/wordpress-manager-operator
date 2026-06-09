import { useEffect, useState } from "react";
import { Card, Button, Input, Space, Typography, Alert, message, Spin, Steps } from "antd";
import { SafetyOutlined, CheckCircleOutlined } from "@ant-design/icons";
import { getTwoFA, setupTwoFA, enableTwoFA, disableTwoFA, type TwoFASetup } from "../api/client";

export default function Security() {
  const [loading, setLoading] = useState(true);
  const [enabled, setEnabled] = useState(false);
  const [setup, setSetup] = useState<TwoFASetup | null>(null);
  const [code, setCode] = useState("");
  const [busy, setBusy] = useState(false);

  async function refresh() {
    setLoading(true);
    try {
      setEnabled((await getTwoFA()).enabled);
    } catch {
      message.error("Không tải được trạng thái 2FA");
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    refresh();
  }, []);

  async function startSetup() {
    setBusy(true);
    try {
      setSetup(await setupTwoFA());
      setCode("");
    } catch {
      message.error("Không tạo được mã 2FA");
    } finally {
      setBusy(false);
    }
  }

  async function confirmEnable() {
    setBusy(true);
    try {
      await enableTwoFA(code.trim());
      message.success("Đã bật 2FA");
      setSetup(null);
      setCode("");
      await refresh();
    } catch (e: any) {
      message.error(e?.response?.data?.error || "Mã không đúng");
    } finally {
      setBusy(false);
    }
  }

  async function turnOff() {
    setBusy(true);
    try {
      await disableTwoFA(code.trim());
      message.success("Đã tắt 2FA");
      setCode("");
      await refresh();
    } catch (e: any) {
      message.error(e?.response?.data?.error || "Mã không đúng");
    } finally {
      setBusy(false);
    }
  }

  if (loading) return <Spin style={{ display: "block", marginTop: 80 }} />;

  return (
    <Card
      title={
        <span>
          <SafetyOutlined /> Bảo mật — Xác thực 2 lớp (2FA)
        </span>
      }
      style={{ maxWidth: 560 }}
    >
      {enabled ? (
        <>
          <Alert
            type="success"
            showIcon
            icon={<CheckCircleOutlined />}
            message="2FA đang BẬT"
            description="Mỗi lần đăng nhập admin sẽ cần mã 6 số từ ứng dụng Authenticator."
            style={{ marginBottom: 16 }}
          />
          <Typography.Paragraph>Nhập mã hiện tại để tắt 2FA:</Typography.Paragraph>
          <Space>
            <Input
              value={code}
              onChange={(e) => setCode(e.target.value)}
              inputMode="numeric"
              maxLength={6}
              placeholder="123456"
              style={{ width: 140 }}
              aria-label="Mã 2FA"
            />
            <Button danger loading={busy} disabled={code.length < 6} onClick={turnOff}>
              Tắt 2FA
            </Button>
          </Space>
        </>
      ) : !setup ? (
        <>
          <Alert type="warning" showIcon message="2FA đang TẮT" style={{ marginBottom: 16 }} />
          <Typography.Paragraph type="secondary">
            Bật xác thực 2 lớp bằng ứng dụng Authenticator (Google Authenticator, Authy, 1Password…).
          </Typography.Paragraph>
          <Button type="primary" icon={<SafetyOutlined />} loading={busy} onClick={startSetup}>
            Bật 2FA
          </Button>
        </>
      ) : (
        <>
          <Steps
            size="small"
            current={1}
            style={{ marginBottom: 16 }}
            items={[{ title: "Tạo mã" }, { title: "Quét QR" }, { title: "Xác nhận" }]}
          />
          <Typography.Paragraph>1. Quét QR bằng ứng dụng Authenticator:</Typography.Paragraph>
          <div style={{ textAlign: "center", marginBottom: 12 }}>
            <img src={setup.qr} alt="2FA QR" width={200} height={200} style={{ border: "1px solid #eee" }} />
          </div>
          <Typography.Paragraph type="secondary">
            Hoặc nhập thủ công secret: <Typography.Text code copyable>{setup.secret}</Typography.Text>
          </Typography.Paragraph>
          <Typography.Paragraph>2. Nhập mã 6 số đang hiển thị để xác nhận:</Typography.Paragraph>
          <Space>
            <Input
              value={code}
              onChange={(e) => setCode(e.target.value)}
              inputMode="numeric"
              maxLength={6}
              placeholder="123456"
              style={{ width: 140 }}
              aria-label="Mã xác nhận 2FA"
            />
            <Button type="primary" loading={busy} disabled={code.length < 6} onClick={confirmEnable}>
              Xác nhận bật
            </Button>
            <Button onClick={() => setSetup(null)}>Hủy</Button>
          </Space>
        </>
      )}
    </Card>
  );
}
