import { useCallback, useEffect, useState } from "react";
import { useNavigate, useParams } from "react-router-dom";
import {
  Card, Descriptions, Tabs, Input, InputNumber, Switch, Form, Row, Col, Divider,
  Button, Space, Tag, message, Spin, Typography, Alert, Tooltip, Table, Empty,
} from "antd";
import {
  ArrowLeftOutlined, SaveOutlined, ReloadOutlined, GlobalOutlined, CodeOutlined, FileTextOutlined,
  PauseCircleOutlined, PlayCircleOutlined, ProfileOutlined, SettingOutlined,
} from "@ant-design/icons";
import yaml from "js-yaml";
import {
  getSite, getSiteYAML, updateSite, updateSiteYAML, suspendSite, resumeSite, getMetrics, getSiteStatus,
  type Site, type SiteYAML, type SiteUsage, type SiteStatus, type PodStatus, type SiteEvent,
} from "../api/client";
import { millis, mib } from "../format";

const phaseColor: Record<string, string> = {
  Ready: "green", Provisioning: "blue", Pending: "default", Suspended: "orange", Error: "red",
};

const podColor: Record<string, string> = {
  Running: "green", Succeeded: "green", Pending: "orange", Failed: "red", Unknown: "default",
};

const editorStyle: React.CSSProperties = {
  fontFamily: "monospace",
  fontSize: 13,
  background: "#1e1e1e",
  color: "#dcdcdc",
};

export default function SiteDetail() {
  const { name = "" } = useParams();
  const navigate = useNavigate();

  const [site, setSite] = useState<Site | null>(null);
  const [yamlDoc, setYamlDoc] = useState<SiteYAML | null>(null);
  const [draft, setDraft] = useState("");
  const [usage, setUsage] = useState<SiteUsage | undefined>();
  const [status, setStatus] = useState<SiteStatus | undefined>();
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [savingForm, setSavingForm] = useState(false);
  const [busy, setBusy] = useState(false);
  const [form] = Form.useForm();

  const load = useCallback(async () => {
    setLoading(true);
    try {
      // getSite is the only critical call; YAML and metrics are best-effort so a
      // partial API failure (e.g. an outdated apiserver, missing RBAC) still
      // renders the host details instead of blanking the whole page.
      const s = await getSite(name);
      setSite(s);
      const [y, m, st] = await Promise.all([
        getSiteYAML(name).catch(() => null),
        getMetrics().catch(() => undefined),
        getSiteStatus(name).catch(() => undefined),
      ]);
      setYamlDoc(y);
      setDraft(y?.source ?? "");
      setUsage(m?.sites.find((u) => u.name === name));
      setStatus(st);
      if (!y) {
        message.warning("Không tải được YAML cấu hình — kiểm tra apiserver đã có endpoint /yaml chưa");
      }
    } catch {
      message.error("Không tải được host");
    } finally {
      setLoading(false);
    }
  }, [name]);

  useEffect(() => {
    load();
  }, [load]);

  // Populate the structured form whenever the site loads/reloads.
  useEffect(() => {
    if (site) {
      form.setFieldsValue({
        domain: site.domain,
        aliases: (site.aliases ?? []).join(", "),
        image: site.image,
        replicas: site.replicas,
        ingressClass: site.ingressClass,
        tlsEnabled: site.tlsEnabled,
        tlsIssuer: site.tlsIssuer,
        tablePrefix: site.tablePrefix,
        phpIni: site.phpIni,
        phpConfig: site.phpConfig,
      });
    }
  }, [site, form]);

  async function saveForm() {
    let v: any;
    try {
      v = await form.validateFields();
    } catch {
      return;
    }
    setSavingForm(true);
    try {
      await updateSite(name, {
        domain: v.domain,
        aliases: v.aliases ? v.aliases.split(",").map((s: string) => s.trim()).filter(Boolean) : [],
        image: v.image || undefined,
        replicas: v.replicas ?? 1,
        ingressClass: v.ingressClass || undefined,
        tlsEnabled: !!v.tlsEnabled,
        tlsIssuer: v.tlsIssuer || undefined,
        tablePrefix: v.tablePrefix || undefined,
        phpIni: v.phpIni || undefined,
        phpConfig: v.phpConfig || undefined,
      });
      message.success("Đã lưu cấu hình — đang reconcile");
      await load();
    } catch (e: any) {
      message.error(e?.response?.data?.error || "Lưu thất bại");
    } finally {
      setSavingForm(false);
    }
  }

  async function save() {
    setSaving(true);
    try {
      await updateSiteYAML(name, draft);
      message.success("Đã lưu cấu hình — đang reconcile");
      await load();
    } catch (e: any) {
      message.error(e?.response?.data?.error || "Lưu thất bại");
    } finally {
      setSaving(false);
    }
  }

  async function toggleSuspend(suspend: boolean) {
    setBusy(true);
    try {
      await (suspend ? suspendSite(name) : resumeSite(name));
      message.success(suspend ? "Đã tạm dừng host" : "Đã kích hoạt host");
      await load();
    } catch {
      message.error("Thao tác thất bại");
    } finally {
      setBusy(false);
    }
  }

  if (loading) return <Spin style={{ display: "block", marginTop: 80 }} />;
  if (!site) return <Alert type="error" message={`Không tìm thấy host "${name}"`} />;

  const dirty = yamlDoc !== null && draft !== yamlDoc.source;

  // Client-side YAML validation: parse the draft and surface parse errors.
  let yamlError: string | null = null;
  try {
    yaml.load(draft);
  } catch (e: any) {
    yamlError = e?.message || "YAML không hợp lệ";
  }

  return (
    <div>
      <Space style={{ marginBottom: 16, width: "100%", justifyContent: "space-between" }}>
        <Space>
          <Button icon={<ArrowLeftOutlined />} onClick={() => navigate("/sites")}>
            Hosts
          </Button>
          <Typography.Title level={4} style={{ margin: 0 }}>
            {site.name}
          </Typography.Title>
          <Tag color={phaseColor[site.phase || ""] || "default"}>{site.phase || "Unknown"}</Tag>
        </Space>
        {site.suspended ? (
          <Button icon={<PlayCircleOutlined />} loading={busy} onClick={() => toggleSuspend(false)}>
            Kích hoạt
          </Button>
        ) : (
          <Button danger icon={<PauseCircleOutlined />} loading={busy} onClick={() => toggleSuspend(true)}>
            Tạm dừng
          </Button>
        )}
      </Space>

      {site.phase && site.phase !== "Ready" && site.phase !== "Suspended" && (
        <Alert
          type={site.phase === "Error" ? "error" : "warning"}
          showIcon
          style={{ marginBottom: 16 }}
          message={`Host đang ở trạng thái "${site.phase}"`}
          description={
            status?.message ||
            site.message ||
            "Xem tab “Trạng thái & Sự kiện” để biết pod đang vướng ở đâu (PVC, image, lịch schedule…)."
          }
        />
      )}

      <Card size="small" style={{ marginBottom: 16 }}>
        <Descriptions column={{ xs: 1, sm: 2, md: 3 }} size="small" bordered>
          <Descriptions.Item label="Domain">
            <a href={site.url || `http://${site.domain}`} target="_blank" rel="noreferrer">
              <GlobalOutlined /> {site.domain}
            </a>
          </Descriptions.Item>
          <Descriptions.Item label="URL">{site.url || "-"}</Descriptions.Item>
          <Descriptions.Item label="Image">
            <code>{site.image || "wordpress:latest (mặc định)"}</code>
          </Descriptions.Item>
          <Descriptions.Item label="Replicas">{site.replicas}</Descriptions.Item>
          <Descriptions.Item label="TLS">
            {site.tlsEnabled ? <Tag color="green">on{site.tlsIssuer ? ` · ${site.tlsIssuer}` : ""}</Tag> : <Tag>off</Tag>}
          </Descriptions.Item>
          <Descriptions.Item label="Ingress class">{site.ingressClass || "nginx"}</Descriptions.Item>
          <Descriptions.Item label="Database">
            <Tooltip title={`user: ${site.databaseUser || "-"}`}>
              <code>{site.databaseName || "-"}</code>
            </Tooltip>
          </Descriptions.Item>
          <Descriptions.Item label="CPU">{usage ? millis(usage.cpuMillicores) : "–"}</Descriptions.Item>
          <Descriptions.Item label="RAM">{usage ? mib(usage.memoryBytes) : "–"}</Descriptions.Item>
          <Descriptions.Item label="php.ini">
            {site.phpIni ? (
              <Tooltip title={<pre style={{ margin: 0, whiteSpace: "pre-wrap" }}>{site.phpIni}</pre>}>
                <Tag color="blue">tùy chỉnh</Tag>
              </Tooltip>
            ) : (
              <Tooltip title="Đang dùng php.ini mặc định (memory_limit 256M, upload 500M, mysqli…). Sửa spec.phpIni trong tab YAML để override.">
                <Tag>mặc định</Tag>
              </Tooltip>
            )}
          </Descriptions.Item>
        </Descriptions>
      </Card>

      <Tabs
        defaultActiveKey="form"
        items={[
          {
            key: "form",
            label: (
              <span>
                <SettingOutlined /> Cấu hình
              </span>
            ),
            children: (
              <Card size="small">
                <Alert
                  type="info"
                  showIcon
                  style={{ marginBottom: 12 }}
                  message="Chọn & nhập các trường thường dùng rồi Lưu — không cần viết YAML. Cấu hình nâng cao (env, resources…) vẫn được giữ nguyên."
                />
                <Form form={form} layout="vertical">
                  <Row gutter={16}>
                    <Col xs={24} md={12}>
                      <Form.Item name="domain" label="Domain" rules={[{ required: true, message: "Nhập domain" }]}>
                        <Input placeholder="blog.acme.example" />
                      </Form.Item>
                    </Col>
                    <Col xs={24} md={12}>
                      <Form.Item name="aliases" label="Domain aliases (phân cách bằng dấu phẩy)">
                        <Input placeholder="www.blog.acme.example" />
                      </Form.Item>
                    </Col>
                  </Row>
                  <Row gutter={16}>
                    <Col xs={24} md={12}>
                      <Form.Item name="image" label="WordPress image">
                        <Input placeholder="wordpress:latest (mặc định)" />
                      </Form.Item>
                    </Col>
                    <Col xs={12} md={6}>
                      <Form.Item name="replicas" label="Replicas">
                        <InputNumber min={0} max={10} style={{ width: "100%" }} />
                      </Form.Item>
                    </Col>
                    <Col xs={12} md={6}>
                      <Form.Item name="tablePrefix" label="Table prefix">
                        <Input />
                      </Form.Item>
                    </Col>
                  </Row>

                  <Divider orientation="left">Ingress &amp; TLS</Divider>
                  <Row gutter={16}>
                    <Col xs={24} md={8}>
                      <Form.Item name="ingressClass" label="Ingress class">
                        <Input placeholder="nginx" />
                      </Form.Item>
                    </Col>
                    <Col xs={8} md={4}>
                      <Form.Item name="tlsEnabled" label="HTTPS" valuePropName="checked">
                        <Switch />
                      </Form.Item>
                    </Col>
                    <Col xs={16} md={12}>
                      <Form.Item name="tlsIssuer" label="cert-manager ClusterIssuer">
                        <Input placeholder="letsencrypt-prod" />
                      </Form.Item>
                    </Col>
                  </Row>

                  <Divider orientation="left">PHP</Divider>
                  <Form.Item name="phpIni" label="php.ini (để trống = dùng mặc định)">
                    <Input.TextArea rows={5} placeholder={"memory_limit = 256M\nupload_max_filesize = 500M"} />
                  </Form.Item>
                  <Form.Item name="phpConfig" label="Extra wp-config.php (WORDPRESS_CONFIG_EXTRA)">
                    <Input.TextArea rows={3} placeholder="define('WP_MEMORY_LIMIT', '256M');" />
                  </Form.Item>

                  <Space>
                    <Button type="primary" icon={<SaveOutlined />} loading={savingForm} onClick={saveForm}>
                      Lưu &amp; Apply
                    </Button>
                    <Button icon={<ReloadOutlined />} onClick={load}>
                      Tải lại
                    </Button>
                  </Space>
                </Form>
              </Card>
            ),
          },
          {
            key: "config",
            label: (
              <span>
                <CodeOutlined /> YAML (nâng cao)
              </span>
            ),
            children: !yamlDoc ? (
              <Alert
                type="warning"
                showIcon
                message="Không tải được YAML cấu hình"
                description="apiserver có thể đang chạy bản cũ thiếu endpoint GET /api/v1/sites/{name}/yaml. Hãy rebuild & deploy lại apiserver (make docker / tag mới)."
              />
            ) : (
              <div>
                <Alert
                  type="info"
                  showIcon
                  style={{ marginBottom: 12 }}
                  message="Sửa trực tiếp WordPressSite rồi Lưu — operator sẽ reconcile lại Deployment/Service/Ingress/DB."
                />
                <Input.TextArea
                  value={draft}
                  onChange={(e) => setDraft(e.target.value)}
                  autoSize={{ minRows: 16, maxRows: 32 }}
                  spellCheck={false}
                  style={editorStyle}
                  aria-label="WordPressSite YAML"
                />
                {yamlError && (
                  <Alert
                    type="error"
                    showIcon
                    style={{ marginTop: 8 }}
                    message="YAML không hợp lệ"
                    description={yamlError}
                    data-testid="yaml-error"
                  />
                )}
                <Space style={{ marginTop: 12 }}>
                  <Button
                    type="primary"
                    icon={<SaveOutlined />}
                    loading={saving}
                    disabled={!dirty || !!yamlError}
                    onClick={save}
                  >
                    Lưu &amp; Apply
                  </Button>
                  <Button icon={<ReloadOutlined />} disabled={!dirty} onClick={() => yamlDoc && setDraft(yamlDoc.source)}>
                    Hoàn tác
                  </Button>
                  {dirty && !yamlError && <Typography.Text type="warning">● thay đổi chưa lưu</Typography.Text>}
                </Space>
              </div>
            ),
          },
          {
            key: "rendered",
            label: (
              <span>
                <FileTextOutlined /> Manifests đã deploy
              </span>
            ),
            children: (
              <pre style={{ ...editorStyle, padding: 16, borderRadius: 6, maxHeight: "60vh", overflow: "auto" }}>
                {yamlDoc?.rendered}
              </pre>
            ),
          },
          {
            key: "status",
            label: (
              <span>
                <ProfileOutlined /> Trạng thái &amp; Sự kiện
              </span>
            ),
            children: (
              <div>
                <Space style={{ marginBottom: 12 }}>
                  <Button size="small" icon={<ReloadOutlined />} onClick={load}>
                    Làm mới
                  </Button>
                </Space>

                <Typography.Text strong>Conditions</Typography.Text>
                {status?.conditions?.length ? (
                  <Table
                    style={{ marginTop: 8 }}
                    size="small"
                    rowKey="type"
                    pagination={false}
                    dataSource={status.conditions}
                    columns={[
                      { title: "Type", dataIndex: "type", width: 110 },
                      {
                        title: "Status",
                        dataIndex: "status",
                        width: 90,
                        render: (v: string) => <Tag color={v === "True" ? "green" : "red"}>{v}</Tag>,
                      },
                      { title: "Reason", dataIndex: "reason", width: 150 },
                      { title: "Message", dataIndex: "message" },
                    ]}
                  />
                ) : (
                  <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="Chưa có condition" />
                )}

                <Typography.Text strong style={{ display: "block", margin: "16px 0 8px" }}>
                  Pods
                </Typography.Text>
                {status?.pods?.length ? (
                  <Table<PodStatus>
                    size="small"
                    rowKey="name"
                    pagination={false}
                    dataSource={status.pods}
                    columns={[
                      { title: "Pod", dataIndex: "name" },
                      {
                        title: "Phase",
                        dataIndex: "phase",
                        width: 100,
                        render: (p: string) => <Tag color={podColor[p] || "default"}>{p}</Tag>,
                      },
                      { title: "Ready", dataIndex: "ready", width: 70 },
                      {
                        title: "Reason",
                        dataIndex: "reason",
                        width: 170,
                        render: (rsn: string, rec: PodStatus) =>
                          rsn ? (
                            <Tooltip title={rec.message}>
                              <Tag color="orange">{rsn}</Tag>
                            </Tooltip>
                          ) : (
                            <Typography.Text type="secondary">–</Typography.Text>
                          ),
                      },
                      { title: "Restarts", dataIndex: "restarts", width: 90 },
                      { title: "Node", dataIndex: "node", render: (n: string) => n || "–" },
                    ]}
                  />
                ) : (
                  <Empty
                    image={Empty.PRESENTED_IMAGE_SIMPLE}
                    description="Không có pod (cluster mock, hoặc pod chưa được schedule)"
                  />
                )}

                <Typography.Text strong style={{ display: "block", margin: "16px 0 8px" }}>
                  Events
                </Typography.Text>
                {status?.events?.length ? (
                  <Table<SiteEvent>
                    size="small"
                    rowKey={(e) => `${e.object}|${e.reason}|${e.lastSeen ?? ""}`}
                    pagination={false}
                    dataSource={status.events}
                    columns={[
                      {
                        title: "Type",
                        dataIndex: "type",
                        width: 90,
                        render: (t: string) => <Tag color={t === "Warning" ? "red" : "default"}>{t}</Tag>,
                      },
                      { title: "Reason", dataIndex: "reason", width: 160 },
                      { title: "Object", dataIndex: "object", width: 180 },
                      { title: "Message", dataIndex: "message" },
                      { title: "×", dataIndex: "count", width: 50 },
                    ]}
                  />
                ) : (
                  <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="Không có event" />
                )}
              </div>
            ),
          },
        ]}
      />
    </div>
  );
}
