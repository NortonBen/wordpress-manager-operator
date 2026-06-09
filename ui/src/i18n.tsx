import { createContext, useCallback, useContext, useState, type ReactNode } from "react";

export type Lang = "vi" | "en";
export type ThemeMode = "light" | "dark";

// Dictionary. The `vi` values are the canonical current UI strings (so they stay
// byte-identical); `en` are the translations. Add keys as needed.
const dict: Record<string, { vi: string; en: string }> = {
  // shell / nav
  "app.title": { vi: "WordPress Hosting", en: "WordPress Hosting" },
  "nav.hosts": { vi: "Hosts", en: "Hosts" },
  "nav.create": { vi: "Create host", en: "Create host" },
  "nav.security": { vi: "Bảo mật (2FA)", en: "Security (2FA)" },
  "common.logout": { vi: "Logout", en: "Logout" },
  "common.language": { vi: "Ngôn ngữ", en: "Language" },
  "common.dark": { vi: "Tối", en: "Dark" },

  // resource cards
  "res.free": { vi: "Còn trống", en: "Free" },

  // login
  "login.bad": { vi: "Sai tài khoản hoặc mật khẩu", en: "Wrong username or password" },
  "login.2faPrompt": { vi: "Nhập mã 6 số từ ứng dụng Authenticator", en: "Enter the 6-digit code from your Authenticator app" },
  "login.signin": { vi: "Sign in", en: "Sign in" },
  "login.verify": { vi: "Xác minh", en: "Verify" },
  "login.back": { vi: "← Đăng nhập lại", en: "← Back to login" },
  "login.code": { vi: "Mã 2FA", en: "2FA code" },
  "login.codeRequired": { vi: "Nhập mã 2FA", en: "Enter the 2FA code" },
  "login.bad2fa": { vi: "Mã 2FA không đúng", en: "Invalid 2FA code" },

  // sites list
  "sites.title": { vi: "WordPress hosts", en: "WordPress hosts" },
  "sites.refresh": { vi: "Refresh", en: "Refresh" },
  "sites.create": { vi: "Create host", en: "Create host" },
  "sites.loadFail": { vi: "Failed to load hosts", en: "Failed to load hosts" },
  "sites.deleteConfirm": { vi: "Data on the shared volume is kept.", en: "Data on the shared volume is kept." },
};

interface Settings {
  lang: Lang;
  theme: ThemeMode;
  setLang: (l: Lang) => void;
  setTheme: (t: ThemeMode) => void;
  t: (key: string) => string;
}

function lookup(lang: Lang, key: string): string {
  return dict[key]?.[lang] ?? key;
}

const Ctx = createContext<Settings>({
  lang: "vi",
  theme: "light",
  setLang: () => {},
  setTheme: () => {},
  t: (key) => lookup("vi", key),
});

export function useSettings() {
  return useContext(Ctx);
}
export function useT() {
  return useContext(Ctx).t;
}

const LANG_KEY = "wpmgr.lang";
const THEME_KEY = "wpmgr.theme";

export function SettingsProvider({ children }: { children: ReactNode }) {
  const [lang, setLangState] = useState<Lang>(() => (localStorage.getItem(LANG_KEY) as Lang) || "vi");
  const [theme, setThemeState] = useState<ThemeMode>(() => (localStorage.getItem(THEME_KEY) as ThemeMode) || "light");

  const setLang = useCallback((l: Lang) => {
    localStorage.setItem(LANG_KEY, l);
    setLangState(l);
  }, []);
  const setTheme = useCallback((th: ThemeMode) => {
    localStorage.setItem(THEME_KEY, th);
    setThemeState(th);
  }, []);
  const t = useCallback((key: string) => lookup(lang, key), [lang]);

  return <Ctx.Provider value={{ lang, theme, setLang, setTheme, t }}>{children}</Ctx.Provider>;
}
