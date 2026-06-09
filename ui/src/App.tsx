import { Navigate, Route, Routes, useNavigate, Link, useLocation } from "react-router-dom";
import { Layout, Menu, Button, Typography } from "antd";
import { CloudServerOutlined, LogoutOutlined, PlusOutlined } from "@ant-design/icons";
import { clearToken, getToken } from "./api/client";
import Login from "./pages/Login";
import SitesList from "./pages/SitesList";
import CreateSite from "./pages/CreateSite";
import SiteDetail from "./pages/SiteDetail";

const { Header, Content, Sider } = Layout;

function RequireAuth({ children }: { children: JSX.Element }) {
  if (!getToken()) return <Navigate to="/login" replace />;
  return children;
}

function Shell({ children }: { children: JSX.Element }) {
  const navigate = useNavigate();
  const location = useLocation();
  const selected = location.pathname.startsWith("/sites/new") ? "/sites/new" : "/sites";

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
            { key: "/sites", icon: <CloudServerOutlined />, label: <Link to="/sites">Hosts</Link> },
            { key: "/sites/new", icon: <PlusOutlined />, label: <Link to="/sites/new">Create host</Link> },
          ]}
        />
      </Sider>
      <Layout>
        <Header style={{ background: "#fff", display: "flex", justifyContent: "space-between", alignItems: "center", paddingInline: 24 }}>
          <Typography.Title level={4} style={{ margin: 0 }}>
            WordPress Hosting
          </Typography.Title>
          <Button
            icon={<LogoutOutlined />}
            onClick={() => {
              clearToken();
              navigate("/login");
            }}
          >
            Logout
          </Button>
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
      <Route path="*" element={<Navigate to="/sites" replace />} />
    </Routes>
  );
}
