import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { MemoryRouter } from "react-router-dom";
import ProfilesPage from "./ProfilesPage";
import * as api from "../services/api";
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
  // The detail panel loads the tab count on mount, so this needs a real
  // resolution rather than the undefined a bare vi.fn() answers with.
  fetchInstanceTabs: vi.fn().mockResolvedValue([]),
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

describe("ProfilesPage classification groups", () => {
  const mixedProfiles: Profile[] = [
    {
      id: "prof_user",
      name: "default",
      created: "2026-03-01T10:00:00Z",
      lastUsed: "2026-03-05T10:00:00Z",
      diskUsage: 4096,
      sizeMB: 0,
      running: false,
    },
    {
      // The flag decides, not the string: a profile the user named after the word is
      // still theirs, and it must not be grouped with the debris.
      id: "prof_notes",
      name: "my.quarantine-notes",
      created: "2026-03-01T10:00:00Z",
      lastUsed: "2026-03-05T10:00:00Z",
      diskUsage: 1024,
      sizeMB: 0,
      running: false,
    },
    {
      id: "prof_q1",
      name: "default.quarantine-1700000001",
      created: "2026-03-01T10:00:00Z",
      lastUsed: "2026-03-05T10:00:00Z",
      diskUsage: 1024 * 1024,
      sizeMB: 1,
      running: false,
      quarantined: true,
    },
    {
      id: "prof_q2",
      name: "default.quarantine-1700000002",
      created: "2026-03-01T10:00:00Z",
      lastUsed: "2026-03-05T10:00:00Z",
      diskUsage: 3 * 1024 * 1024,
      sizeMB: 3,
      running: false,
      quarantined: true,
    },
    {
      id: "prof_tmp",
      name: "instance-9868",
      created: "2026-03-01T10:00:00Z",
      lastUsed: "2026-03-05T10:00:00Z",
      diskUsage: 512 * 1024,
      sizeMB: 0,
      running: false,
      temporary: true,
    },
  ];

  beforeEach(() => {
    vi.clearAllMocks();
    useAppStore.setState({
      profiles: mixedProfiles,
      profilesLoading: false,
      instances: [],
    });
  });

  function sidebarRowNames() {
    const sidebar = document.querySelector(
      ".bg-bg-surface\\/50",
    ) as HTMLElement;
    return within(sidebar)
      .getAllByRole("button")
      .filter((b) => b.classList.contains("border-b"))
      .map((b) => b.querySelector(".text-sm.font-semibold")?.textContent ?? "");
  }

  it("keeps the user's own profiles above the quarantined and temporary groups", () => {
    renderProfilesPage();

    const names = sidebarRowNames();
    expect(names.slice(0, 2)).toEqual(["default", "my.quarantine-notes"]);
    expect(names.indexOf("instance-9868")).toBeGreaterThan(
      names.indexOf("my.quarantine-notes"),
    );
    expect(names.indexOf("default.quarantine-1700000001")).toBeGreaterThan(
      names.indexOf("instance-9868"),
    );
  });

  it("heads each classification group with its count and combined size", () => {
    renderProfilesPage();

    const quarantined = screen.getByTestId("profile-group-quarantined");
    expect(quarantined).toHaveTextContent("Quarantined (2)");
    expect(quarantined).toHaveTextContent("4.0 MB total");

    const temporary = screen.getByTestId("profile-group-temporary");
    expect(temporary).toHaveTextContent("Temporary (1)");
    expect(temporary).toHaveTextContent("512.0 KB total");
  });

  it("marks quarantined and temporary rows and leaves user rows unmarked", () => {
    renderProfilesPage();

    const rowFor = (name: string) => {
      const label = screen.getByText(name, {
        selector: ".text-sm.font-semibold",
      });
      return label.closest("button") as HTMLElement;
    };

    expect(rowFor("default.quarantine-1700000001")).toHaveTextContent(
      "quarantined",
    );
    expect(rowFor("instance-9868")).toHaveTextContent("temporary");

    // The name contains the word and the flag does not: no badge, and no group header
    // above it either.
    expect(rowFor("my.quarantine-notes")).not.toHaveTextContent("quarantined");
    expect(rowFor("default")).not.toHaveTextContent("quarantined");
    expect(rowFor("default")).not.toHaveTextContent("temporary");
  });

  it("adds no bulk-delete or cleanup affordance", () => {
    renderProfilesPage();

    const sidebar = document.querySelector(
      ".bg-bg-surface\\/50",
    ) as HTMLElement;
    for (const label of [
      /delete all/i,
      /clean ?up/i,
      /prune/i,
      /safe to delete/i,
    ]) {
      expect(within(sidebar).queryByText(label)).toBeNull();
    }
  });
});

// One unconfirmed click used to destroy a profile directory with no undo, and a failed
// delete died in console.error — so a 409 refusal from the in-use guard reached the user
// as nothing at all. These tests pin the confirmation, both failure paths, the visible
// success, and the running-profile withholding.
describe("ProfilesPage delete safety", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    useAppStore.setState({
      profiles,
      profilesLoading: false,
      instances,
    });
  });

  async function openDeleteConfirmation() {
    renderProfilesPage();
    await waitFor(() => {
      expect(
        screen.getByRole("button", { name: /Profile: beta/i }),
      ).toBeInTheDocument();
    });
    await clickSidebarProfile("alpha");
    const detailPanel = getDetailPanel()!;
    await userEvent.click(
      within(detailPanel).getByRole("button", { name: "Delete" }),
    );
    expect(screen.getByText(/Delete profile "alpha"\?/)).toBeInTheDocument();
  }

  it("dismissing the confirmation issues no request", async () => {
    await openDeleteConfirmation();

    await userEvent.click(screen.getByRole("button", { name: "Cancel" }));

    expect(api.deleteProfile).not.toHaveBeenCalled();
    expect(screen.queryByText(/Delete profile "alpha"\?/)).toBeNull();
  });

  it("confirming deletes and visibly confirms the success", async () => {
    vi.mocked(api.deleteProfile).mockResolvedValue(undefined);
    vi.mocked(api.fetchProfiles).mockResolvedValue([profiles[1]]);
    await openDeleteConfirmation();

    await userEvent.click(
      screen.getByRole("button", { name: "Delete profile" }),
    );

    expect(api.deleteProfile).toHaveBeenCalledWith("prof_alpha");
    await waitFor(() => {
      expect(screen.getByRole("status")).toHaveTextContent(
        'Profile "alpha" deleted',
      );
    });
  });

  it("renders the server's 409 refusal instead of swallowing it", async () => {
    vi.mocked(api.deleteProfile).mockRejectedValue(
      new Error('profile "alpha" is in use by inst_x; delete with force=true'),
    );
    await openDeleteConfirmation();

    await userEvent.click(
      screen.getByRole("button", { name: "Delete profile" }),
    );

    await waitFor(() => {
      expect(screen.getByRole("alert")).toHaveTextContent(/in use by inst_x/);
    });
    expect(
      screen.getByRole("button", { name: /Profile: alpha/i }),
    ).toBeInTheDocument();
  });

  it("renders a 500 failure through the same path", async () => {
    vi.mocked(api.deleteProfile).mockRejectedValue(
      new Error("HTTP 500: something broke"),
    );
    await openDeleteConfirmation();

    await userEvent.click(
      screen.getByRole("button", { name: "Delete profile" }),
    );

    await waitFor(() => {
      expect(screen.getByRole("alert")).toHaveTextContent(/something broke/);
    });
  });

  it("does not offer Delete for a profile reporting running", async () => {
    renderProfilesPage();
    await waitFor(() => {
      expect(
        screen.getByRole("button", { name: /Profile: beta/i }),
      ).toBeInTheDocument();
    });

    const detailPanel = getDetailPanel()!;
    expect(
      within(detailPanel).queryByRole("button", { name: "Delete" }),
    ).toBeNull();
    // The withholding fails safe: alpha reports running false and keeps the
    // confirmed delete path, asserted by the dismissal test above.
  });
});
