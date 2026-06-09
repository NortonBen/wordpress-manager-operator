import React from "react";
import ReactDOM from "react-dom/client";
import { BrowserRouter } from "react-router-dom";
import { ConfigProvider, theme as antdTheme } from "antd";
import viVN from "antd/locale/vi_VN";
import enUS from "antd/locale/en_US";
import App from "./App";
import { SettingsProvider, useSettings } from "./i18n";
import "antd/dist/reset.css";

function Themed() {
  const { theme, lang } = useSettings();
  return (
    <ConfigProvider
      locale={lang === "vi" ? viVN : enUS}
      theme={{
        algorithm: theme === "dark" ? antdTheme.darkAlgorithm : antdTheme.defaultAlgorithm,
        token: { colorPrimary: "#21759b" },
      }}
    >
      <BrowserRouter>
        <App />
      </BrowserRouter>
    </ConfigProvider>
  );
}

ReactDOM.createRoot(document.getElementById("root")!).render(
  <React.StrictMode>
    <SettingsProvider>
      <Themed />
    </SettingsProvider>
  </React.StrictMode>,
);
