import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import ProfileDetailsPanel from "./ProfileDetailsPanel";
import * as api from "../services/api";
import { useAppStore } from "../stores/useAppStore";
import type { Instance, InstanceTab, Profile } from "../generated/types";

vi.mock(import("../services/api"), async (importOriginal) => ({
  ...(await importOriginal()),
  fetchInstanceTabs: vi.fn(),
}));

const profile: Profile = {
  id: "prof_beta",
  name: "beta",
  created: "2026-03-02T10:00:00Z",
  lastUsed: "2026-03-06T10:00:00Z",
  diskUsage: 2048,
  sizeMB: 24,
  running: true,
};

const instance: Instance = {
  id: "inst_beta",
  profileId: "prof_beta",
  profileName: "beta",
  port: "9988",
  mode: "headed",
  headless: false,
  status: "running",
  startTime: "2026-03-06T10:00:00Z",
  attached: false,
};

const tabs: InstanceTab[] = [
  {
    id: "tab_1",
    instanceId: instance.id,
    url: "https://example.com",
    title: "Example",
  },
  {
    id: "tab_2",
    instanceId: instance.id,
    url: "https://www.iana.org",
    title: "IANA",
  },
  {
    id: "tab_3",
    instanceId: instance.id,
    url: "about:blank",
    title: "New tab",
  },
];

function renderPanel() {
  return render(
    <ProfileDetailsPanel
      profile={profile}
      instance={instance}
      onLaunch={() => {}}
    />,
  );
}

// The header tab always carries its badge, which is what tells it apart from the
// icon button named plain "Tabs" that the opened sub-panel adds.
function tabsButton() {
  return screen.getByRole("button", { name: /^Tabs\s*(\d+|—)$/ });
}

describe("ProfileDetailsPanel tabs badge", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    useAppStore.setState({ handoffNotifications: [] });
  });

  // The badge is what tells an operator whether opening the Tabs sub-panel is
  // worth it, so it cannot be the sub-panel that populates it. Nothing here
  // clicks "Tabs": the panel opens on "Profile", exactly as it does on a fresh
  // page load.
  it("shows the real tab count before the Tabs sub-panel is opened", async () => {
    vi.mocked(api.fetchInstanceTabs).mockResolvedValue(tabs);

    renderPanel();

    await waitFor(() => {
      expect(tabsButton()).toHaveTextContent("Tabs3");
    });
    expect(screen.getByRole("button", { name: /^Profile: beta/ })).toHaveClass(
      "bg-bg-hover",
    );
  });

  // A count that could not be read must not borrow the representation of "this
  // profile holds nothing": beside a running instance a hard 0 reads as "idle,
  // safe to stop", and Stop is two buttons away in this same panel.
  it("does not render a count it could not read as 0", async () => {
    vi.mocked(api.fetchInstanceTabs).mockRejectedValue(new Error("network"));
    vi.spyOn(console, "error").mockImplementation(() => {});

    renderPanel();

    await waitFor(() => {
      expect(api.fetchInstanceTabs).toHaveBeenCalled();
    });
    await waitFor(() => {
      expect(tabsButton()).not.toHaveTextContent("Tabs0");
    });
    expect(tabsButton()).toHaveTextContent("Tabs—");
  });

  // The first paint happens before the fetch resolves, so an initial empty list
  // would put a hard 0 on screen for exactly as long as the request takes — the
  // reading this whole card is about, just briefer.
  it("does not claim a count while the fetch is still in flight", async () => {
    let resolveTabs: (value: InstanceTab[]) => void = () => {};
    vi.mocked(api.fetchInstanceTabs).mockReturnValue(
      new Promise<InstanceTab[]>((resolve) => {
        resolveTabs = resolve;
      }),
    );

    renderPanel();

    expect(tabsButton()).toHaveTextContent("Tabs—");

    await act(async () => {
      resolveTabs(tabs);
    });
    expect(tabsButton()).toHaveTextContent("Tabs3");
  });

  // The badge and the sub-panel read one state, so opening the sub-panel can
  // only ever confirm the number already on screen.
  it("keeps the same number when the Tabs sub-panel is opened", async () => {
    vi.mocked(api.fetchInstanceTabs).mockResolvedValue(tabs);

    renderPanel();
    await waitFor(() => {
      expect(tabsButton()).toHaveTextContent("Tabs3");
    });

    await userEvent.click(tabsButton());

    expect(tabsButton()).toHaveTextContent("Tabs3");
    expect(await screen.findByText("Example")).toBeInTheDocument();
  });

  // A profile with no instance genuinely holds no tabs, so 0 is the honest
  // answer there — this is what keeps the case above from passing by never
  // showing 0 at all.
  it("shows 0 for a profile with no instance", async () => {
    render(<ProfileDetailsPanel profile={profile} onLaunch={() => {}} />);

    await waitFor(() => {
      expect(tabsButton()).toHaveTextContent("Tabs0");
    });
    expect(api.fetchInstanceTabs).not.toHaveBeenCalled();
  });
});
