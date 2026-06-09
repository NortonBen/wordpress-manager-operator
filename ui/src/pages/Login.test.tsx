import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import Login from "./Login";
import * as client from "../api/client";

// Mock flow: the backend is replaced by a mocked API client.
vi.mock("../api/client", () => ({
  login: vi.fn(),
  setToken: vi.fn(),
}));

const mockedLogin = vi.mocked(client.login);
const mockedSetToken = vi.mocked(client.setToken);

function renderLogin() {
  render(
    <MemoryRouter>
      <Login />
    </MemoryRouter>,
  );
}

describe("Login page", () => {
  beforeEach(() => vi.clearAllMocks());

  it("signs in with the entered credentials and stores the token", async () => {
    mockedLogin.mockResolvedValue("jwt-token");
    renderLogin();
    const user = userEvent.setup();

    await user.type(screen.getByLabelText("Password"), "s3cret");
    await user.click(screen.getByRole("button", { name: /sign in/i }));

    await waitFor(() => expect(mockedLogin).toHaveBeenCalledWith("admin", "s3cret", undefined));
    expect(mockedSetToken).toHaveBeenCalledWith("jwt-token");
  });

  it("shows an error message when credentials are rejected", async () => {
    mockedLogin.mockRejectedValue(new Error("unauthorized"));
    renderLogin();
    const user = userEvent.setup();

    await user.type(screen.getByLabelText("Password"), "wrong");
    await user.click(screen.getByRole("button", { name: /sign in/i }));

    expect(await screen.findByText(/Sai tài khoản hoặc mật khẩu/i)).toBeInTheDocument();
    expect(mockedSetToken).not.toHaveBeenCalled();
  });

  it("prompts for a 2FA code then logs in", async () => {
    // First attempt: server says a TOTP code is required.
    mockedLogin.mockRejectedValueOnce({ response: { data: { error: "totp_required" } } });
    renderLogin();
    const user = userEvent.setup();

    await user.type(screen.getByLabelText("Password"), "s3cret");
    await user.click(screen.getByRole("button", { name: /sign in/i }));

    // The 2FA field appears.
    const code = await screen.findByLabelText("Mã 2FA");
    mockedLogin.mockResolvedValueOnce("jwt-token");
    await user.type(code, "123456");
    await user.click(screen.getByRole("button", { name: /Xác minh/ }));

    await waitFor(() => expect(mockedLogin).toHaveBeenLastCalledWith("admin", "s3cret", "123456"));
    expect(mockedSetToken).toHaveBeenCalledWith("jwt-token");
  });
});
