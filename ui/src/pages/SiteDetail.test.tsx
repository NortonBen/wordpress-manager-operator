import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor, fireEvent } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Routes, Route } from "react-router-dom";
import SiteDetail from "./SiteDetail";
import * as client from "../api/client";

vi.mock("../api/client", () => ({
  getSite: vi.fn(),
  getSiteYAML: vi.fn(),
  updateSiteYAML: vi.fn(),
  suspendSite: vi.fn(),
  resumeSite: vi.fn(),
  getMetrics: vi.fn(),
  getSiteStatus: vi.fn(),
}));

const mGetSite = vi.mocked(client.getSite);
const mGetYAML = vi.mocked(client.getSiteYAML);
const mUpdateYAML = vi.mocked(client.updateSiteYAML);
const mSuspend = vi.mocked(client.suspendSite);
const mMetrics = vi.mocked(client.getMetrics);
const mStatus = vi.mocked(client.getSiteStatus);

const SOURCE = `apiVersion: wp.benji.dev/v1alpha1
kind: WordPressSite
metadata:
  name: blog-acme
  namespace: wordpress-sites
spec:
  domain: blog.acme.example
  replicas: 1
`;

function renderDetail() {
  render(
    <MemoryRouter initialEntries={["/sites/blog-acme"]}>
      <Routes>
        <Route path="/sites/:name" element={<SiteDetail />} />
      </Routes>
    </MemoryRouter>,
  );
}

describe("SiteDetail page", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mGetSite.mockResolvedValue({
      name: "blog-acme", domain: "blog.acme.example", replicas: 1,
      tlsEnabled: false, phase: "Ready", url: "http://blog.acme.example",
      databaseName: "wp_blog_acme", databaseUser: "wpu_blog_acme",
    });
    mGetYAML.mockResolvedValue({ source: SOURCE, rendered: "kind: Deployment\nmetadata:\n  name: blog-acme\n" });
    mMetrics.mockResolvedValue({
      cluster: {
        cpu: { used: 0, capacity: 0, allocatable: 0, available: 0 },
        memory: { used: 0, capacity: 0, allocatable: 0, available: 0 },
        nodes: [], metricsAvailable: true,
      },
      sites: [{ name: "blog-acme", cpuMillicores: 120, memoryBytes: 268435456 }],
    });
    mStatus.mockResolvedValue({ phase: "Ready", conditions: [], pods: [], events: [] });
  });

  it("shows host details and the editable YAML", async () => {
    renderDetail();
    expect(await screen.findByText("blog.acme.example")).toBeInTheDocument();
    expect(screen.getByText("wp_blog_acme")).toBeInTheDocument();
    const editor = screen.getByLabelText("WordPressSite YAML") as HTMLTextAreaElement;
    expect(editor.value).toContain("kind: WordPressSite");
    expect(editor.value).toContain("domain: blog.acme.example");
  });

  it("saves hand-edited YAML", async () => {
    mUpdateYAML.mockResolvedValue({} as any);
    renderDetail();
    const editor = (await screen.findByLabelText("WordPressSite YAML")) as HTMLTextAreaElement;

    // Manual edit: bump replicas to 3.
    const edited = SOURCE.replace("replicas: 1", "replicas: 3");
    fireEvent.change(editor, { target: { value: edited } });

    const save = screen.getByRole("button", { name: /Lưu/ });
    expect(save).toBeEnabled();
    await userEvent.click(save);

    await waitFor(() => expect(mUpdateYAML).toHaveBeenCalledWith("blog-acme", edited));
  });

  it("disables Save and shows an error on invalid YAML", async () => {
    renderDetail();
    const editor = (await screen.findByLabelText("WordPressSite YAML")) as HTMLTextAreaElement;

    fireEvent.change(editor, { target: { value: "spec:\n  domain: x\n bad: : :" } });

    expect(await screen.findByTestId("yaml-error")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /Lưu/ })).toBeDisabled();
  });

  it("suspends a running host", async () => {
    mSuspend.mockResolvedValue({} as any);
    renderDetail();
    const btn = await screen.findByRole("button", { name: /Tạm dừng/ });
    await userEvent.click(btn);
    await waitFor(() => expect(mSuspend).toHaveBeenCalledWith("blog-acme"));
  });

  it("still renders host details when the YAML endpoint fails", async () => {
    mGetYAML.mockRejectedValue(new Error("404 not found"));
    renderDetail();
    // Host details (from getSite) still show…
    expect(await screen.findByText("blog.acme.example")).toBeInTheDocument();
    expect(screen.getByText("wp_blog_acme")).toBeInTheDocument();
    // …and the config tab degrades gracefully instead of blanking the page.
    expect(screen.getByText("Không tải được YAML cấu hình")).toBeInTheDocument();
  });

  it("shows Resume for a suspended host", async () => {
    mGetSite.mockResolvedValue({
      name: "blog-acme", domain: "blog.acme.example", replicas: 1,
      tlsEnabled: false, phase: "Suspended", suspended: true,
    });
    renderDetail();
    expect(await screen.findByRole("button", { name: /Kích hoạt/ })).toBeInTheDocument();
  });

  it("surfaces the error and pod diagnostics", async () => {
    mGetSite.mockResolvedValue({
      name: "blog-acme", domain: "blog.acme.example", replicas: 1,
      tlsEnabled: false, phase: "Error", message: "workload: PVC unbound",
    });
    mStatus.mockResolvedValue({
      phase: "Error",
      message: "workload: PVC unbound",
      conditions: [{ type: "Ready", status: "False", reason: "Error", message: "workload: PVC unbound" }],
      pods: [
        {
          name: "blog-acme-abc",
          phase: "Pending",
          ready: "0/1",
          reason: "Unschedulable",
          message: "pod has unbound immediate PersistentVolumeClaims",
          restarts: 0,
        },
      ],
      events: [
        {
          type: "Warning",
          reason: "FailedScheduling",
          message: "0/1 nodes available: unbound PVC",
          count: 3,
          object: "Pod/blog-acme-abc",
          lastSeen: "2026-06-09T00:00:00Z",
        },
      ],
    });

    renderDetail();
    // Error banner shows the reconcile message.
    expect(await screen.findByText('Host đang ở trạng thái "Error"')).toBeInTheDocument();
    expect(screen.getAllByText(/workload: PVC unbound/).length).toBeGreaterThan(0);

    // Status tab reveals the stuck pod reason.
    await userEvent.click(screen.getByRole("tab", { name: /Trạng thái/ }));
    expect(await screen.findByText("blog-acme-abc")).toBeInTheDocument();
    expect(screen.getByText("Unschedulable")).toBeInTheDocument();
  });
});
