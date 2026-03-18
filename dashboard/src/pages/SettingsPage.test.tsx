import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { MemoryRouter } from "react-router-dom";
import SettingsPage from "./SettingsPage";
import { useAppStore } from "../stores/useAppStore";
import { defaultBackendConfig, type BackendConfigState } from "../types";

vi.mock("../services/api", () => ({
  fetchBackendConfig: vi.fn(),
  fetchHealth: vi.fn(),
  saveBackendConfig: vi.fn(),
  elevate: vi.fn(),
  isApiError: vi.fn(() => false),
}));

function renderSettingsPage() {
  return render(
    <MemoryRouter>
      <SettingsPage />
    </MemoryRouter>,
  );
}

function makeConfigState(): BackendConfigState {
  return {
    config: structuredClone(defaultBackendConfig),
    configPath: "/tmp/config.json",
    tokenConfigured: true,
    restartRequired: false,
    restartReasons: [],
  };
}

describe("SettingsPage", () => {
  beforeEach(async () => {
    vi.clearAllMocks();
    const api = await import("../services/api");
    const configState = makeConfigState();
    vi.mocked(api.fetchBackendConfig).mockResolvedValue(configState);
    vi.mocked(api.fetchHealth).mockRejectedValue(new Error("skip health"));
    vi.mocked(api.saveBackendConfig).mockResolvedValue(configState);

    useAppStore.setState({
      settings: useAppStore.getState().settings,
      serverInfo: null,
    });
  });

  it("shows token status without exposing token management controls", async () => {
    renderSettingsPage();

    await userEvent.click(
      await screen.findByRole("button", { name: /Network & Attach/i }),
    );

    expect(
      screen.getByText(
        "Token configured. Manage rotation through the CLI or config file; the current value is never returned by the server.",
      ),
    ).toBeInTheDocument();
    expect(screen.queryByLabelText("API token")).not.toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "Generate" }),
    ).not.toBeInTheDocument();
    expect(screen.queryByText(/No API token is set/i)).not.toBeInTheDocument();
  });
});
