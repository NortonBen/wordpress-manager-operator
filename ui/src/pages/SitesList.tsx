import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { Table, Tag, Button, Space, Popconfirm, message, Typography, Tooltip } from "antd";
import { ReloadOutlined, PlusOutlined, DeleteOutlined, GlobalOutlined } from "@ant-design/icons";
import { listSites, deleteSite, type Site } from "../api/client";

const phaseColor: Record<string, string> = {
  Ready: "green",
  Provisioning: "blue",
  Pending: "default",
  Suspended: "orange",
  Error: "red",
};

export default function SitesList() {
  const [sites, setSites] = useState<Site[]>([]);
  const [loading, setLoading] = useState(false);

  async function refresh() {
    setLoading(true);
    try {
      setSites(await listSites());
    } catch {
      message.error("Failed to load hosts");
    } finally {
      setLoading(false);
    }
  }

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
          WordPress hosts
        </Typography.Title>
        <Space>
          <Button icon={<ReloadOutlined />} onClick={refresh}>
            Refresh
          </Button>
          <Link to="/sites/new">
            <Button type="primary" icon={<PlusOutlined />}>
              Create host
            </Button>
          </Link>
        </Space>
      </Space>

      <Table<Site>
        rowKey="name"
        loading={loading}
        dataSource={sites}
        pagination={false}
        columns={[
          { title: "Name", dataIndex: "name" },
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
            render: (p?: string) => <Tag color={phaseColor[p || ""] || "default"}>{p || "Unknown"}</Tag>,
          },
          { title: "Replicas", dataIndex: "replicas", width: 90 },
          {
            title: "TLS",
            dataIndex: "tlsEnabled",
            width: 70,
            render: (v: boolean) => (v ? <Tag color="green">on</Tag> : <Tag>off</Tag>),
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
