import { describe, it, expect, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { SettingsProvider, useSettings, useT } from "./i18n";

function Probe() {
  const t = useT();
  const { lang, theme, setLang, setTheme } = useSettings();
  return (
    <div>
      <span data-testid="free">{t("res.free")}</span>
      <span data-testid="lang">{lang}</span>
      <span data-testid="theme">{theme}</span>
      <button onClick={() => setLang("en")}>en</button>
      <button onClick={() => setTheme("dark")}>dark</button>
    </div>
  );
}

describe("i18n / settings", () => {
  beforeEach(() => localStorage.clear());

  it("defaults to Vietnamese and switches to English", async () => {
    render(
      <SettingsProvider>
        <Probe />
      </SettingsProvider>,
    );
    expect(screen.getByTestId("free").textContent).toBe("Còn trống");
    expect(screen.getByTestId("lang").textContent).toBe("vi");

    await userEvent.click(screen.getByText("en"));
    expect(screen.getByTestId("free").textContent).toBe("Free");
    expect(localStorage.getItem("wpmgr.lang")).toBe("en");
  });

  it("toggles and persists dark mode", async () => {
    render(
      <SettingsProvider>
        <Probe />
      </SettingsProvider>,
    );
    expect(screen.getByTestId("theme").textContent).toBe("light");
    await userEvent.click(screen.getByText("dark"));
    expect(screen.getByTestId("theme").textContent).toBe("dark");
    expect(localStorage.getItem("wpmgr.theme")).toBe("dark");
  });

  it("falls back to Vietnamese without a provider", () => {
    render(<Probe />);
    expect(screen.getByTestId("free").textContent).toBe("Còn trống");
  });
});
