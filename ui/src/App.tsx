import { Navigate, Route, Routes, useNavigate, Link, useLocation } from "react-router-dom";
import { Layout, Menu, Button, Typography, Select, Switch, Space } from "antd";
import { CloudServerOutlined, LogoutOutlined, PlusOutlined, SafetyOutlined } from "@ant-design/icons";
import { clearToken, getToken } from "./api/client";
import { useSettings } from "./i18n";
import Login from "./pages/Login";
import SitesList from "./pages/SitesList";
import CreateSite from "./pages/CreateSite";
import SiteDetail from "./pages/SiteDetail";
import Security from "./pages/Security";

const { Header, Content, Sider } = Layout;

function RequireAuth({ children }: { children: JSX.Element }) {
  if (!getToken()) return <Navigate to="/login" replace />;
  return children;
}

function Shell({ children }: { children: JSX.Element }) {
  const navigate = useNavigate();
  const location = useLocation();
  const { lang, setLang, theme, setTheme, t } = useSettings();
  const selected = location.pathname.startsWith("/sites/new")
    ? "/sites/new"
    : location.pathname.startsWith("/security")
      ? "/security"
      : "/sites";

  return (
    <Layout style={{ minHeight: "100vh" }}>
      <Sider theme="dark" breakpoint="lg" collapsedWidth="0">
        <div style={{ color: "#fff", padding: 16, fontWeight: 600, fontSize: 16 }}>
          <CloudServerOutlined /> WP Manager
        </div>
        <Menu
          theme="dark"
          mode="inline"
          selectedKeys={[selected]}
          items={[
            { key: "/sites", icon: <CloudServerOutlined />, label: <Link to="/sites">{t("nav.hosts")}</Link> },
            { key: "/sites/new", icon: <PlusOutlined />, label: <Link to="/sites/new">{t("nav.create")}</Link> },
            { key: "/security", icon: <SafetyOutlined />, label: <Link to="/security">{t("nav.security")}</Link> },
          ]}
        />
      </Sider>
      <Layout>
        <Header style={{ display: "flex", justifyContent: "space-between", alignItems: "center", paddingInline: 24 }}>
          <Typography.Title level={4} style={{ margin: 0 }}>
            {t("app.title")}
          </Typography.Title>
          <Space>
            <Select
              size="small"
              value={lang}
              onChange={setLang}
              style={{ width: 84 }}
              options={[
                { value: "vi", label: "🇻🇳 VI" },
                { value: "en", label: "🇬🇧 EN" },
              ]}
            />
            <Switch
              checkedChildren="🌙"
              unCheckedChildren="☀️"
              checked={theme === "dark"}
              onChange={(c) => setTheme(c ? "dark" : "light")}
            />
            <Button
              icon={<LogoutOutlined />}
              onClick={() => {
                clearToken();
                navigate("/login");
              }}
            >
              {t("common.logout")}
            </Button>
          </Space>
        </Header>
        <Content style={{ margin: 24 }}>{children}</Content>
      </Layout>
    </Layout>
  );
}

export default function App() {
  return (
    <Routes>
      <Route path="/login" element={<Login />} />
      <Route
        path="/sites"
        element={
          <RequireAuth>
            <Shell>
              <SitesList />
            </Shell>
          </RequireAuth>
        }
      />
      <Route
        path="/sites/new"
        element={
          <RequireAuth>
            <Shell>
              <CreateSite />
            </Shell>
          </RequireAuth>
        }
      />
      <Route
        path="/sites/:name"
        element={
          <RequireAuth>
            <Shell>
              <SiteDetail />
            </Shell>
          </RequireAuth>
        }
      />
      <Route
        path="/security"
        element={
          <RequireAuth>
            <Shell>
              <Security />
            </Shell>
          </RequireAuth>
        }
      />
      <Route path="*" element={<Navigate to="/sites" replace />} />
    </Routes>
  );
}
