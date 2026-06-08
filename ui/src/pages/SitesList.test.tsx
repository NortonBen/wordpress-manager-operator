import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import SitesList from "./SitesList";
import * as client from "../api/client";

vi.mock("../api/client", () => ({
  listSites: vi.fn(),
  deleteSite: vi.fn(),
  getMetrics: vi.fn(),
}));

const mockedList = vi.mocked(client.listSites);
const mockedDelete = vi.mocked(client.deleteSite);
const mockedMetrics = vi.mocked(client.getMetrics);

const sampleMetrics = {
  cluster: {
    cpu: { used: 2000, capacity: 8000, allocatable: 7600, available: 5600 },
    memory: { used: 4 * 1024 ** 3, capacity: 16 * 1024 ** 3, allocatable: 15 * 1024 ** 3, available: 11 * 1024 ** 3 },
    nodes: [{ name: "n1", cpu: { used: 2000, capacity: 8000, allocatable: 7600, available: 5600 }, memory: { used: 0, capacity: 0, allocatable: 0, available: 0 } }],
    metricsAvailable: true,
  },
  sites: [{ name: "blog-acme", cpuMillicores: 210, memoryBytes: 256 * 1024 ** 2 }],
};

function renderList() {
  render(
    <MemoryRouter>
      <SitesList />
    </MemoryRouter>,
  );
}

describe("SitesList page", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockedMetrics.mockResolvedValue(sampleMetrics);
  });

  it("renders hosts returned by the API", async () => {
    mockedList.mockResolvedValue([
      {
        name: "blog-acme",
        domain: "blog.acme.example",
        replicas: 2,
        tlsEnabled: true,
        phase: "Ready",
        databaseName: "wp_blog_acme",
        databaseUser: "wpu_blog_acme",
      },
    ]);

    renderList();

    expect(await screen.findByText("blog.acme.example")).toBeInTheDocument();
    expect(screen.getByText("Ready")).toBeInTheDocument();
    expect(screen.getByText("wp_blog_acme")).toBeInTheDocument();
  });

  it("shows cluster resource cards and per-site usage", async () => {
    mockedList.mockResolvedValue([
      { name: "blog-acme", domain: "blog.acme.example", replicas: 1, tlsEnabled: false, phase: "Ready" },
    ]);
    renderList();

    await screen.findByText("blog.acme.example");
    // cluster cards render available (remaining) for CPU and RAM
    expect(screen.getAllByText(/Còn trống:/i).length).toBeGreaterThanOrEqual(2);
    // per-site usage columns
    expect(await screen.findByText("210m")).toBeInTheDocument();
    expect(screen.getByText("256Mi")).toBeInTheDocument();
  });

  it("deletes a host on confirm", async () => {
    mockedList.mockResolvedValue([
      { name: "blog-acme", domain: "blog.acme.example", replicas: 1, tlsEnabled: false, phase: "Ready" },
    ]);
    mockedDelete.mockResolvedValue();

    renderList();
    const user = userEvent.setup();

    await screen.findByText("blog.acme.example");
    await user.click(screen.getByRole("button", { name: /delete/i }));
    // antd Popconfirm confirm button.
    await user.click(await screen.findByRole("button", { name: /^OK$/i }));

    await waitFor(() => expect(mockedDelete).toHaveBeenCalledWith("blog-acme"));
  });
});
