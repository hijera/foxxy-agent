import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import React from "react";
import { EnvHealthBanner } from "./EnvHealthBanner";
import { useActiveEnvHealth } from "./activeHealth";
import { snapshotEnv } from "./remoteEnv";
import { initLocale, setLocale } from "../i18n/i18n";

// The banner only renders for a *remote* environment that probes down, so both are stubbed.
vi.mock("./activeHealth", async (importOriginal) => ({
  ...(await importOriginal<typeof import("./activeHealth")>()),
  useActiveEnvHealth: vi.fn(() => "down" as const),
}));

// useSyncExternalStore requires a cached snapshot: a fresh object per call spins forever.
const REMOTE_ENV = {
  mode: "remote" as const,
  baseUrl: "https://box.example:12345",
  token: "t",
  name: "box",
};

vi.mock("./remoteEnv", async (importOriginal) => ({
  ...(await importOriginal<typeof import("./remoteEnv")>()),
  snapshotEnv: vi.fn(() => REMOTE_ENV),
}));

beforeEach(() => {
  initLocale("en");
});

afterEach(() => {
  cleanup();
  setLocale("en");
});

describe("EnvHealthBanner", () => {
  it("keeps the remote name bold and the config key as code", () => {
    render(<EnvHealthBanner />);
    const banner = screen.getByTestId("env-health-banner");
    expect(banner.querySelector("strong")?.textContent).toBe("box");
    expect(banner.querySelector("code")?.textContent).toBe("httpserver.cors");
    expect(banner.textContent).toContain("is unreachable or unauthorized");
  });

  it("translates the sentence and the action button", () => {
    setLocale("ru");
    render(<EnvHealthBanner />);
    const banner = screen.getByTestId("env-health-banner");
    expect(banner.textContent).toContain("недоступно или не авторизовано");
    expect(banner.textContent).toContain("Переключиться на локальное");
    // Slots survive translation: no raw placeholder is left in the output.
    expect(banner.textContent).not.toContain("{name}");
    expect(banner.textContent).not.toContain("{cors}");
    expect(banner.querySelector("strong")?.textContent).toBe("box");
  });

  it("renders nothing while the remote is reachable", () => {
    vi.mocked(useActiveEnvHealth).mockReturnValueOnce("up");
    const { container } = render(<EnvHealthBanner />);
    expect(container.firstChild).toBeNull();
    expect(vi.mocked(snapshotEnv)).toHaveBeenCalled();
  });
});
