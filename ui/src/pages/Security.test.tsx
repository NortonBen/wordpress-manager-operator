import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import Security from "./Security";
import * as client from "../api/client";

vi.mock("../api/client", () => ({
  getTwoFA: vi.fn(),
  setupTwoFA: vi.fn(),
  enableTwoFA: vi.fn(),
  disableTwoFA: vi.fn(),
}));

const mGet = vi.mocked(client.getTwoFA);
const mSetup = vi.mocked(client.setupTwoFA);
const mEnable = vi.mocked(client.enableTwoFA);

describe("Security (2FA) page", () => {
  beforeEach(() => vi.clearAllMocks());

  it("enrolls 2FA: setup → scan → confirm code", async () => {
    mGet.mockResolvedValue({ enabled: false });
    mSetup.mockResolvedValue({
      secret: "ABCDEF234567",
      otpauthUrl: "otpauth://totp/WordPress%20Manager:admin?secret=ABCDEF234567",
      qr: "data:image/png;base64,iVBORw0KGgo=",
    });
    mEnable.mockResolvedValue({ enabled: true });

    render(<Security />);
    const user = userEvent.setup();

    await user.click(await screen.findByRole("button", { name: /Bật 2FA/ }));

    // QR + manual secret are shown.
    expect(await screen.findByAltText("2FA QR")).toBeInTheDocument();
    expect(screen.getByText("ABCDEF234567")).toBeInTheDocument();

    await user.type(screen.getByLabelText("Mã xác nhận 2FA"), "123456");
    await user.click(screen.getByRole("button", { name: /Xác nhận bật/ }));

    await waitFor(() => expect(mEnable).toHaveBeenCalledWith("123456"));
  });

  it("shows the disable control when 2FA is on", async () => {
    mGet.mockResolvedValue({ enabled: true });
    render(<Security />);
    expect(await screen.findByText("2FA đang BẬT")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /Tắt 2FA/ })).toBeInTheDocument();
  });
});
