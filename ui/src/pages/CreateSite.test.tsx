import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import CreateSite from "./CreateSite";
import * as client from "../api/client";

vi.mock("../api/client", () => ({
  createSite: vi.fn(),
  previewYAML: vi.fn(),
}));

const mockedCreate = vi.mocked(client.createSite);
const mockedPreview = vi.mocked(client.previewYAML);

function renderCreate() {
  render(
    <MemoryRouter>
      <CreateSite />
    </MemoryRouter>,
  );
}

describe("CreateSite page", () => {
  beforeEach(() => vi.clearAllMocks());

  it("submits the collected form values to createSite", async () => {
    mockedCreate.mockResolvedValue({
      name: "blog-acme",
      domain: "blog.acme.example",
      replicas: 1,
      tlsEnabled: false,
    });
    renderCreate();
    const user = userEvent.setup();

    await user.type(screen.getByLabelText(/^Name/), "blog-acme");
    await user.type(screen.getByLabelText(/Primary domain/i), "blog.acme.example");
    await user.click(screen.getByRole("button", { name: /create host/i }));

    await waitFor(() => expect(mockedCreate).toHaveBeenCalledTimes(1));
    expect(mockedCreate.mock.calls[0][0]).toMatchObject({
      name: "blog-acme",
      domain: "blog.acme.example",
    });
  });

  it("requests a YAML preview", async () => {
    mockedPreview.mockResolvedValue("kind: WordPressSite\n");
    renderCreate();
    const user = userEvent.setup();

    await user.type(screen.getByLabelText(/^Name/), "shop-foo");
    await user.type(screen.getByLabelText(/Primary domain/i), "shop.foo.example");
    await user.click(screen.getByRole("button", { name: /preview yaml/i }));

    await waitFor(() => expect(mockedPreview).toHaveBeenCalledTimes(1));
    expect(await screen.findByText(/kind: WordPressSite/)).toBeInTheDocument();
  });
});
