import { Card, Col, Progress, Row, Statistic, Tooltip, Tag, Typography } from "antd";
import { DashboardOutlined, DatabaseOutlined, CloudServerOutlined } from "@ant-design/icons";
import type { ClusterMetrics } from "../api/client";
import { cores, gib, pct } from "../format";
import { useT } from "../i18n";

const { Text } = Typography;

function barColor(p: number): string {
  if (p >= 85) return "#cf1322";
  if (p >= 70) return "#d48806";
  return "#21759b";
}

export default function ResourceCards({ cluster }: { cluster?: ClusterMetrics }) {
  const t = useT();
  if (!cluster) return null;

  const cpuPct = pct(cluster.cpu.used, cluster.cpu.allocatable);
  const memPct = pct(cluster.memory.used, cluster.memory.allocatable);

  return (
    <Row gutter={16} style={{ marginBottom: 16 }}>
      <Col xs={24} md={9}>
        <Card size="small">
          <Statistic
            title={
              <span>
                <DashboardOutlined /> CPU
              </span>
            }
            value={cores(cluster.cpu.used)}
            suffix={`/ ${cores(cluster.cpu.allocatable)} cores`}
            valueStyle={{ color: barColor(cpuPct) }}
          />
          <Progress percent={cpuPct} strokeColor={barColor(cpuPct)} size="small" />
          <Text type="secondary" data-testid="cpu-available">
            {t("res.free")}: {cores(cluster.cpu.available)} cores
          </Text>
        </Card>
      </Col>

      <Col xs={24} md={9}>
        <Card size="small">
          <Statistic
            title={
              <span>
                <DatabaseOutlined /> RAM
              </span>
            }
            value={gib(cluster.memory.used)}
            suffix={`/ ${gib(cluster.memory.allocatable)} GiB`}
            valueStyle={{ color: barColor(memPct) }}
          />
          <Progress percent={memPct} strokeColor={barColor(memPct)} size="small" />
          <Text type="secondary" data-testid="mem-available">
            {t("res.free")}: {gib(cluster.memory.available)} GiB
          </Text>
        </Card>
      </Col>

      <Col xs={24} md={6}>
        <Card size="small">
          <Statistic
            title={
              <span>
                <CloudServerOutlined /> Nodes
              </span>
            }
            value={cluster.nodes.length}
          />
          {cluster.metricsAvailable ? (
            <Tag color="green">metrics-server: on</Tag>
          ) : (
            <Tooltip title="Cài metrics-server để có số liệu sử dụng thực tế">
              <Tag color="orange">metrics-server: off</Tag>
            </Tooltip>
          )}
        </Card>
      </Col>
    </Row>
  );
}
