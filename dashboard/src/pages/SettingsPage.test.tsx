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

    const tokenHint = screen.getByText("pinchtab config token").parentElement;

    expect(tokenHint).toHaveTextContent(
      "Token configured. Manage rotation through the CLI or config file; the current value is never returned by the server.",
    );
    expect(tokenHint).toHaveTextContent("pinchtab config token");
    expect(tokenHint).toHaveTextContent("to copy it to your clipboard.");
    expect(screen.queryByLabelText("API token")).not.toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "Generate" }),
    ).not.toBeInTheDocument();
    expect(screen.queryByText(/No API token is set/i)).not.toBeInTheDocument();
  });

  it("shows autosolver config as file-backed with no env var overrides", async () => {
    renderSettingsPage();

    await userEvent.click(
      await screen.findByRole("button", { name: /AutoSolver/i }),
    );

    expect(
      screen.getByText(
        "Dashboard edits are written back to this file. Set external provider keys under autoSolver.external in the same config file.",
      ),
    ).toBeInTheDocument();
    expect(screen.getByText("/tmp/config.json")).toBeInTheDocument();
  });

  it("shows navigation trust settings in the Security section", async () => {
    renderSettingsPage();

    await userEvent.click(
      (await screen.findByText("Security")).closest("button")!,
    );

    expect(screen.getByText("Allowed websites")).toBeInTheDocument();
    expect(screen.getByText("Trusted proxy CIDRs")).toBeInTheDocument();
    expect(screen.getByText("Trusted resolve CIDRs")).toBeInTheDocument();
    expect(screen.getAllByText(/single hosts/)).toHaveLength(2);
  });

  it("keeps the IDPI section focused on IDPI toggles and patterns", async () => {
    renderSettingsPage();

    await userEvent.click(
      (await screen.findByText("Security IDPI")).closest("button")!,
    );

    expect(screen.queryByText("Allowed websites")).not.toBeInTheDocument();
    expect(screen.getByText("Custom patterns")).toBeInTheDocument();
  });
});
