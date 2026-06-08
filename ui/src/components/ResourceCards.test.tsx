import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/react";
import ResourceCards from "./ResourceCards";
import type { ClusterMetrics } from "../api/client";

const cluster: ClusterMetrics = {
  cpu: { used: 2000, capacity: 8000, allocatable: 7600, available: 5600 },
  memory: {
    used: 4 * 1024 ** 3,
    capacity: 16 * 1024 ** 3,
    allocatable: 15 * 1024 ** 3,
    available: 11 * 1024 ** 3,
  },
  nodes: [
    {
      name: "n1",
      cpu: { used: 2000, capacity: 8000, allocatable: 7600, available: 5600 },
      memory: { used: 0, capacity: 0, allocatable: 0, available: 0 },
    },
  ],
  metricsAvailable: true,
};

describe("ResourceCards", () => {
  it("renders nothing when no metrics", () => {
    const { container } = render(<ResourceCards cluster={undefined} />);
    expect(container).toBeEmptyDOMElement();
  });

  it("shows CPU and RAM used + remaining", () => {
    render(<ResourceCards cluster={cluster} />);
    // CPU used 2000m = 2 cores
    expect(screen.getByText("2")).toBeInTheDocument();
    expect(screen.getByText(/\/ 7.6 cores/)).toBeInTheDocument();
    // remaining values
    expect(screen.getByTestId("cpu-available")).toHaveTextContent("Còn trống: 5.6 cores");
    expect(screen.getByTestId("mem-available")).toHaveTextContent("Còn trống: 11.0 GiB");
    // metrics-server badge
    expect(screen.getByText(/metrics-server: on/)).toBeInTheDocument();
  });
});
