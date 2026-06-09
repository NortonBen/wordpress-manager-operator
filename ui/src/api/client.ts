import axios from "axios";

// Single axios instance. The JWT is attached from localStorage on every call;
// a 401 clears it and bounces back to the login screen.
export const api = axios.create({ baseURL: "/api/v1" });

const TOKEN_KEY = "wpmgr.token";

export function getToken(): string | null {
  return localStorage.getItem(TOKEN_KEY);
}
export function setToken(token: string) {
  localStorage.setItem(TOKEN_KEY, token);
}
export function clearToken() {
  localStorage.removeItem(TOKEN_KEY);
}

api.interceptors.request.use((config) => {
  const token = getToken();
  if (token) {
    config.headers.Authorization = `Bearer ${token}`;
  }
  return config;
});

api.interceptors.response.use(
  (res) => res,
  (err) => {
    if (err?.response?.status === 401 && !err.config?.url?.includes("/login")) {
      clearToken();
      if (window.location.pathname !== "/login") {
        window.location.href = "/login";
      }
    }
    return Promise.reject(err);
  },
);

// ---- Domain types & calls ----

export interface Site {
  name: string;
  domain: string;
  aliases?: string[];
  image?: string;
  replicas: number;
  tlsEnabled: boolean;
  tlsIssuer?: string;
  ingressClass?: string;
  tablePrefix?: string;
  phpConfig?: string;
  phpIni?: string;
  forceHTTPS?: boolean;
  suspended?: boolean;
  // status (read-only)
  phase?: string;
  message?: string;
  url?: string;
  databaseName?: string;
  databaseUser?: string;
}

export async function login(username: string, password: string, totp?: string): Promise<string> {
  const { data } = await api.post<{ token: string }>("/login", { username, password, totp });
  return data.token;
}

// ---- Admin 2FA (TOTP) ----

export interface TwoFASetup {
  secret: string;
  otpauthUrl: string;
  qr: string; // data:image/png;base64,...
}

export async function getTwoFA(): Promise<{ enabled: boolean }> {
  const { data } = await api.get<{ enabled: boolean }>("/2fa");
  return data;
}
export async function setupTwoFA(): Promise<TwoFASetup> {
  const { data } = await api.post<TwoFASetup>("/2fa/setup");
  return data;
}
export async function enableTwoFA(code: string): Promise<{ enabled: boolean }> {
  const { data } = await api.post<{ enabled: boolean }>("/2fa/enable", { code });
  return data;
}
export async function disableTwoFA(code: string): Promise<{ enabled: boolean }> {
  const { data } = await api.post<{ enabled: boolean }>("/2fa/disable", { code });
  return data;
}

export async function listSites(): Promise<Site[]> {
  const { data } = await api.get<Site[]>("/sites");
  return data ?? [];
}

export async function createSite(site: Partial<Site>): Promise<Site> {
  const { data } = await api.post<Site>("/sites", site);
  return data;
}

export async function getSite(name: string): Promise<Site> {
  const { data } = await api.get<Site>(`/sites/${name}`);
  return data;
}

export async function updateSite(name: string, site: Partial<Site>): Promise<Site> {
  const { data } = await api.put<Site>(`/sites/${name}`, site);
  return data;
}

export async function deleteSite(name: string): Promise<void> {
  await api.delete(`/sites/${name}`);
}

export interface SiteYAML {
  source: string; // editable WordPressSite CR
  rendered: string; // read-only deployed manifests
}

export async function getSiteYAML(name: string): Promise<SiteYAML> {
  const { data } = await api.get<SiteYAML>(`/sites/${name}/yaml`);
  return data;
}

export async function updateSiteYAML(name: string, yaml: string): Promise<Site> {
  const { data } = await api.put<Site>(`/sites/${name}/yaml`, yaml, {
    headers: { "Content-Type": "application/x-yaml" },
    transformRequest: [(d) => d], // send raw string, don't JSON-encode
  });
  return data;
}

// ---- Diagnostics (status / pods / events) ----

export interface Condition {
  type: string;
  status: string;
  reason?: string;
  message?: string;
  lastTransition?: string;
}
export interface PodStatus {
  name: string;
  phase: string;
  ready: string;
  reason?: string;
  message?: string;
  restarts: number;
  node?: string;
  created?: string;
}
export interface SiteEvent {
  type: string;
  reason: string;
  message: string;
  count: number;
  object: string;
  lastSeen?: string;
}
export interface SiteStatus {
  phase: string;
  message?: string;
  conditions?: Condition[];
  pods?: PodStatus[];
  events?: SiteEvent[];
}

export async function getSiteStatus(name: string): Promise<SiteStatus> {
  const { data } = await api.get<SiteStatus>(`/sites/${name}/status`);
  return data;
}

export async function suspendSite(name: string): Promise<Site> {
  const { data } = await api.post<Site>(`/sites/${name}/suspend`);
  return data;
}

export async function resumeSite(name: string): Promise<Site> {
  const { data } = await api.post<Site>(`/sites/${name}/resume`);
  return data;
}

export async function previewYAML(site: Partial<Site>): Promise<string> {
  const { data } = await api.post("/sites/preview", site, { responseType: "text" });
  return data as string;
}

// ---- Resource metrics ----

export interface Metric {
  used: number;
  capacity: number;
  allocatable: number;
  available: number;
}
export interface NodeMetric {
  name: string;
  cpu: Metric;
  memory: Metric;
}
export interface ClusterMetrics {
  cpu: Metric; // millicores
  memory: Metric; // bytes
  nodes: NodeMetric[];
  metricsAvailable: boolean;
}
export interface SiteUsage {
  name: string;
  cpuMillicores: number;
  memoryBytes: number;
}
export interface MetricsResponse {
  cluster: ClusterMetrics;
  sites: SiteUsage[];
}

export async function getMetrics(): Promise<MetricsResponse> {
  const { data } = await api.get<MetricsResponse>("/metrics");
  return data;
}
