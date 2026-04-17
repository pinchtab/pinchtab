import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { MemoryRouter } from "react-router-dom";
import ProfilesPage from "./ProfilesPage";
import { useAppStore } from "../stores/useAppStore";
import type { Instance, Profile } from "../generated/types";

vi.mock("../services/api", () => ({
  fetchProfiles: vi.fn(),
  createProfile: vi.fn(),
  deleteProfile: vi.fn(),
  updateProfile: vi.fn(),
  fetchInstances: vi.fn(),
  launchInstance: vi.fn(),
  stopInstance: vi.fn(),
  fetchInstanceTabs: vi.fn(),
  fetchInstanceLogs: vi.fn(),
  fetchActivity: vi.fn(),
  fetchAllTabs: vi.fn(),
}));

const profiles: Profile[] = [
  {
    id: "prof_alpha",
    name: "alpha",
    created: "2026-03-01T10:00:00Z",
    lastUsed: "2026-03-05T10:00:00Z",
    diskUsage: 1024,
    sizeMB: 12,
    running: false,
    useWhen: "Use for personal logins",
  },
  {
    id: "prof_beta",
    name: "beta",
    created: "2026-03-02T10:00:00Z",
    lastUsed: "2026-03-06T10:00:00Z",
    diskUsage: 2048,
    sizeMB: 24,
    running: true,
    accountEmail: "team@example.com",
  },
];

const instances: Instance[] = [
  {
    id: "inst_beta",
    profileId: "prof_beta",
    profileName: "beta",
    port: "9988",
    mode: "headed",
    headless: false,
    status: "running",
    startTime: "2026-03-06T10:00:00Z",
    attached: false,
  },
];

function renderProfilesPage() {
  return render(
    <MemoryRouter>
      <ProfilesPage />
    </MemoryRouter>,
  );
}

function clickSidebarProfile(name: string) {
  const profileNameEl = screen.getByText(name, {
    selector: ".text-sm.font-semibold",
  });
  const button = profileNameEl.closest("button") as HTMLElement;
  return userEvent.click(button);
}

function getDetailPanel() {
  return document.querySelector(
    ".dashboard-panel .min-w-0.flex-1",
  ) as HTMLElement;
}

describe("ProfilesPage", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    useAppStore.setState({
      profiles,
      profilesLoading: false,
      instances,
    });
  });

  it("moves the running profile to the top and auto-selects it", async () => {
    renderProfilesPage();

    await waitFor(() => {
      expect(
        screen.getByRole("button", { name: /Profile: beta/i }),
      ).toBeInTheDocument();
    });

    const sidebar = document.querySelector(
      ".bg-bg-surface\\/50",
    ) as HTMLElement;
    const sidebarButtons = within(sidebar).getAllByRole("button");
    const profileButtons = sidebarButtons.filter((b) =>
      b.classList.contains("border-b"),
    );
    expect(profileButtons[0]).toHaveTextContent("beta");
    expect(profileButtons[1]).toHaveTextContent("alpha");

    const detailPanel = getDetailPanel()!;
    expect(
      within(detailPanel).getAllByText("team@example.com").length,
    ).toBeGreaterThan(0);
    expect(within(detailPanel).getByText("running")).toBeInTheDocument();
    expect(within(detailPanel).getByText("9988")).toBeInTheDocument();
  });

  it("switches the right detail pane when selecting another profile", async () => {
    renderProfilesPage();

    await waitFor(() => {
      expect(
        screen.getByRole("button", { name: /Profile: beta/i }),
      ).toBeInTheDocument();
    });

    await clickSidebarProfile("alpha");

    await waitFor(() => {
      expect(
        screen.getByRole("button", { name: /Profile: alpha/i }),
      ).toBeInTheDocument();
    });

    const detailPanel = getDetailPanel()!;
    expect(
      within(detailPanel).getAllByText("Use for personal logins").length,
    ).toBeGreaterThan(0);
    expect(
      within(detailPanel).getByRole("button", { name: "Start" }),
    ).toBeInTheDocument();
  });

  it("enables save only after profile fields change", async () => {
    renderProfilesPage();

    await waitFor(() => {
      expect(
        screen.getByRole("button", { name: /Profile: beta/i }),
      ).toBeInTheDocument();
    });

    await clickSidebarProfile("alpha");

    await waitFor(() => {
      expect(
        screen.getByRole("button", { name: /Profile: alpha/i }),
      ).toBeInTheDocument();
    });

    const detailPanel = getDetailPanel()!;
    const saveButton = within(detailPanel).getByRole("button", {
      name: "Save",
    });
    const nameInput = within(detailPanel).getByDisplayValue("alpha");

    expect(saveButton).toBeDisabled();

    await userEvent.clear(nameInput);
    await userEvent.type(nameInput, "alpha-updated");

    expect(saveButton).toBeEnabled();
  });
});
