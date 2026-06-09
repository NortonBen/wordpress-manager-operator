import { useState } from "react";
import { useNavigate } from "react-router-dom";
import { Card, Form, Input, InputNumber, Switch, Button, Space, message, Row, Col, Divider, Modal } from "antd";
import { EyeOutlined, SaveOutlined } from "@ant-design/icons";
import { createSite, previewYAML, type Site } from "../api/client";

export default function CreateSite() {
  const navigate = useNavigate();
  const [form] = Form.useForm();
  const [saving, setSaving] = useState(false);
  const [yaml, setYaml] = useState<string | null>(null);

  function collect(): Partial<Site> {
    const v = form.getFieldsValue();
    return {
      name: v.name,
      domain: v.domain,
      aliases: v.aliases ? v.aliases.split(",").map((s: string) => s.trim()).filter(Boolean) : [],
      image: v.image || undefined,
      replicas: v.replicas ?? 1,
      tlsEnabled: !!v.tlsEnabled,
      tlsIssuer: v.tlsIssuer || undefined,
      ingressClass: v.ingressClass || undefined,
      tablePrefix: v.tablePrefix || "wp_",
      phpConfig: v.phpConfig || undefined,
      phpIni: v.phpIni || undefined,
    };
  }

  async function onSave() {
    try {
      await form.validateFields();
    } catch {
      return;
    }
    setSaving(true);
    try {
      await createSite(collect());
      message.success("Host created — provisioning started");
      navigate("/sites");
    } catch (e: any) {
      message.error(e?.response?.data?.error || "Create failed");
    } finally {
      setSaving(false);
    }
  }

  async function onPreview() {
    try {
      await form.validateFields(["name", "domain"]);
    } catch {
      return;
    }
    try {
      setYaml(await previewYAML(collect()));
    } catch {
      message.error("Preview failed");
    }
  }

  return (
    <Card title="Create WordPress host" style={{ maxWidth: 820 }}>
      <Form
        form={form}
        layout="vertical"
        initialValues={{ replicas: 1, tablePrefix: "wp_", tlsEnabled: false, ingressClass: "nginx" }}
      >
        <Row gutter={16}>
          <Col span={12}>
            <Form.Item
              name="name"
              label="Name (k8s resource id)"
              rules={[
                { required: true },
                { pattern: /^[a-z0-9-]+$/, message: "lowercase letters, digits and dashes only" },
              ]}
            >
              <Input placeholder="blog-acme" />
            </Form.Item>
          </Col>
          <Col span={12}>
            <Form.Item name="domain" label="Primary domain" rules={[{ required: true }]}>
              <Input placeholder="blog.acme.example" />
            </Form.Item>
          </Col>
        </Row>

        <Form.Item name="aliases" label="Domain aliases (comma-separated)">
          <Input placeholder="www.blog.acme.example" />
        </Form.Item>

        <Row gutter={16}>
          <Col span={12}>
            <Form.Item name="image" label="WordPress image (optional)">
              <Input placeholder="wordpress:latest (mặc định)" />
            </Form.Item>
          </Col>
          <Col span={6}>
            <Form.Item name="replicas" label="Replicas">
              <InputNumber min={0} max={10} style={{ width: "100%" }} />
            </Form.Item>
          </Col>
          <Col span={6}>
            <Form.Item name="tablePrefix" label="Table prefix">
              <Input />
            </Form.Item>
          </Col>
        </Row>

        <Divider orientation="left">Ingress & TLS</Divider>
        <Row gutter={16}>
          <Col span={8}>
            <Form.Item name="ingressClass" label="Ingress class">
              <Input placeholder="nginx" />
            </Form.Item>
          </Col>
          <Col span={4}>
            <Form.Item name="tlsEnabled" label="HTTPS" valuePropName="checked">
              <Switch />
            </Form.Item>
          </Col>
          <Col span={12}>
            <Form.Item name="tlsIssuer" label="cert-manager ClusterIssuer">
              <Input placeholder="letsencrypt-prod" />
            </Form.Item>
          </Col>
        </Row>

        <Divider orientation="left">Advanced</Divider>
        <Form.Item name="phpConfig" label="Extra wp-config.php (WORDPRESS_CONFIG_EXTRA)">
          <Input.TextArea rows={3} placeholder="define('WP_MEMORY_LIMIT', '256M');" />
        </Form.Item>
        <Form.Item
          name="phpIni"
          label="php.ini (để trống = dùng mặc định; mount vào conf.d, tự rollout khi đổi)"
        >
          <Input.TextArea
            rows={7}
            placeholder={
              "Mặc định nếu để trống:\nfile_uploads = On\nmemory_limit = 256M\nupload_max_filesize = 500M\npost_max_size = 500M\nmax_execution_time = 300\nextension=mysqli"
            }
          />
        </Form.Item>

        <Space>
          <Button type="primary" icon={<SaveOutlined />} loading={saving} onClick={onSave}>
            Create host
          </Button>
          <Button icon={<EyeOutlined />} onClick={onPreview}>
            Preview YAML
          </Button>
          <Button onClick={() => navigate("/sites")}>Cancel</Button>
        </Space>
      </Form>

      <Modal
        title="Generated manifests"
        open={yaml !== null}
        onCancel={() => setYaml(null)}
        footer={<Button onClick={() => setYaml(null)}>Close</Button>}
        width={820}
      >
        <pre style={{ maxHeight: "60vh", overflow: "auto", background: "#1e1e1e", color: "#dcdcdc", padding: 16, borderRadius: 6 }}>
          {yaml}
        </pre>
      </Modal>
    </Card>
  );
}
