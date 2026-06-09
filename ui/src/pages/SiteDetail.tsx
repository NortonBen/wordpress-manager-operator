import { useCallback, useEffect, useState } from "react";
import { useNavigate, useParams } from "react-router-dom";
import {
  Card, Descriptions, Tabs, Input, Button, Space, Tag, message, Spin, Typography, Alert, Tooltip,
} from "antd";
import {
  ArrowLeftOutlined, SaveOutlined, ReloadOutlined, GlobalOutlined, CodeOutlined, FileTextOutlined,
  PauseCircleOutlined, PlayCircleOutlined,
} from "@ant-design/icons";
import yaml from "js-yaml";
import {
  getSite, getSiteYAML, updateSiteYAML, suspendSite, resumeSite, getMetrics,
  type Site, type SiteYAML, type SiteUsage,
} from "../api/client";
import { millis, mib } from "../format";

const phaseColor: Record<string, string> = {
  Ready: "green", Provisioning: "blue", Pending: "default", Suspended: "orange", Error: "red",
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
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [busy, setBusy] = useState(false);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      // getSite is the only critical call; YAML and metrics are best-effort so a
      // partial API failure (e.g. an outdated apiserver, missing RBAC) still
      // renders the host details instead of blanking the whole page.
      const s = await getSite(name);
      setSite(s);
      const [y, m] = await Promise.all([
        getSiteYAML(name).catch(() => null),
        getMetrics().catch(() => undefined),
      ]);
      setYamlDoc(y);
      setDraft(y?.source ?? "");
      setUsage(m?.sites.find((u) => u.name === name));
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
        items={[
          {
            key: "config",
            label: (
              <span>
                <CodeOutlined /> Cấu hình YAML (sửa tay)
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
        ]}
      />
    </div>
  );
}
