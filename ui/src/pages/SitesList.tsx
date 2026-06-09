import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { Table, Tag, Button, Space, Popconfirm, message, Typography, Tooltip } from "antd";
import { ReloadOutlined, PlusOutlined, DeleteOutlined, GlobalOutlined } from "@ant-design/icons";
import { listSites, deleteSite, getMetrics, type Site, type MetricsResponse } from "../api/client";
import ResourceCards from "../components/ResourceCards";
import { millis, mib } from "../format";
import { useT } from "../i18n";

const phaseColor: Record<string, string> = {
  Ready: "green",
  Provisioning: "blue",
  Pending: "default",
  Suspended: "orange",
  Error: "red",
};

export default function SitesList() {
  const t = useT();
  const [sites, setSites] = useState<Site[]>([]);
  const [metrics, setMetrics] = useState<MetricsResponse | undefined>();
  const [loading, setLoading] = useState(false);

  async function refresh() {
    setLoading(true);
    try {
      const [s, m] = await Promise.all([listSites(), getMetrics().catch(() => undefined)]);
      setSites(s);
      if (m) setMetrics(m);
    } catch {
      message.error(t("sites.loadFail"));
    } finally {
      setLoading(false);
    }
  }

  const usage = new Map((metrics?.sites ?? []).map((u) => [u.name, u]));

  useEffect(() => {
    refresh();
    const t = setInterval(refresh, 8000);
    return () => clearInterval(t);
  }, []);

  async function onDelete(name: string) {
    try {
      await deleteSite(name);
      message.success(`Deleted ${name}`);
      refresh();
    } catch {
      message.error("Delete failed");
    }
  }

  return (
    <div>
      <Space style={{ marginBottom: 16, justifyContent: "space-between", width: "100%" }}>
        <Typography.Title level={4} style={{ margin: 0 }}>
          {t("sites.title")}
        </Typography.Title>
        <Space>
          <Button icon={<ReloadOutlined />} onClick={refresh}>
            {t("sites.refresh")}
          </Button>
          <Link to="/sites/new">
            <Button type="primary" icon={<PlusOutlined />}>
              {t("sites.create")}
            </Button>
          </Link>
        </Space>
      </Space>

      <ResourceCards cluster={metrics?.cluster} />

      <Table<Site>
        rowKey="name"
        loading={loading}
        dataSource={sites}
        pagination={false}
        columns={[
          {
            title: "Name",
            dataIndex: "name",
            render: (n: string) => <Link to={`/sites/${n}`}>{n}</Link>,
          },
          {
            title: "Domain",
            dataIndex: "domain",
            render: (d: string, r) => (
              <a href={r.url || `http://${d}`} target="_blank" rel="noreferrer">
                <GlobalOutlined /> {d}
              </a>
            ),
          },
          {
            title: "Phase",
            dataIndex: "phase",
            render: (p: string | undefined, r) => {
              const tag = <Tag color={phaseColor[p || ""] || "default"}>{p || "Unknown"}</Tag>;
              return r.message ? <Tooltip title={r.message}>{tag}</Tooltip> : tag;
            },
          },
          { title: "Replicas", dataIndex: "replicas", width: 90 },
          {
            title: "TLS",
            dataIndex: "tlsEnabled",
            width: 70,
            render: (v: boolean) => (v ? <Tag color="green">on</Tag> : <Tag>off</Tag>),
          },
          {
            title: "CPU",
            key: "cpu",
            width: 80,
            render: (_, r) => {
              const u = usage.get(r.name);
              return u ? <span>{millis(u.cpuMillicores)}</span> : <Typography.Text type="secondary">–</Typography.Text>;
            },
          },
          {
            title: "RAM",
            key: "ram",
            width: 90,
            render: (_, r) => {
              const u = usage.get(r.name);
              return u ? <span>{mib(u.memoryBytes)}</span> : <Typography.Text type="secondary">–</Typography.Text>;
            },
          },
          {
            title: "Database",
            dataIndex: "databaseName",
            render: (db?: string, r?: Site) => (
              <Tooltip title={`user: ${r?.databaseUser || "-"}`}>
                <code>{db || "-"}</code>
              </Tooltip>
            ),
          },
          {
            title: "",
            key: "actions",
            width: 110,
            render: (_, r) => (
              <Popconfirm title={`Delete ${r.name}?`} description="Data on the shared volume is kept." onConfirm={() => onDelete(r.name)}>
                <Button danger size="small" icon={<DeleteOutlined />}>
                  Delete
                </Button>
              </Popconfirm>
            ),
          },
        ]}
      />
    </div>
  );
}
