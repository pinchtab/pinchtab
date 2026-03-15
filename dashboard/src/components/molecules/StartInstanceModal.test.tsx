import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import StartInstanceModal from "./StartInstanceModal";
import { useAppStore } from "../../stores/useAppStore";
import type { Instance, Profile } from "../../generated/types";

vi.mock("../../services/api", () => ({
  launchInstance: vi.fn(),
  fetchInstances: vi.fn(),
}));

const profile: Profile = {
  id: "prof_alpha",
  name: "alpha",
  created: "2026-03-01T10:00:00Z",
  lastUsed: "2026-03-05T10:00:00Z",
  diskUsage: 1024,
  sizeMB: 12,
  running: false,
};

describe("StartInstanceModal", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    useAppStore.setState({ instances: [] });
  });

  it("builds its own launch command from local form state", async () => {
    render(
      <StartInstanceModal open={true} profile={profile} onClose={() => {}} />,
    );

    const command = document.querySelector("textarea") as HTMLTextAreaElement;
    expect(command.value).toContain('"profileId":"prof_alpha"');
    expect(command.value).toContain('"mode":"headed"');

    await userEvent.type(
      screen.getByPlaceholderText("Auto-select from configured range"),
      "9988",
    );
    await userEvent.click(
      screen.getByLabelText("Headless (best for Docker/VPS)"),
    );

    expect(command.value).toContain('"port":"9988"');
    expect(command.value).not.toContain('"mode":"headed"');
  });

  it("launches the selected profile and refreshes instances", async () => {
    const { fetchInstances, launchInstance } =
      await import("../../services/api");
    const onClose = vi.fn();
    const updatedInstances: Instance[] = [
      {
        id: "inst_alpha",
        profileId: "prof_alpha",
        profileName: "alpha",
        port: "9988",
        headless: false,
        status: "running",
        startTime: "2026-03-06T10:00:00Z",
        attached: false,
      },
    ];

    vi.mocked(launchInstance).mockResolvedValue({} as never);
    vi.mocked(fetchInstances).mockResolvedValue(updatedInstances);

    render(
      <StartInstanceModal open={true} profile={profile} onClose={onClose} />,
    );

    await userEvent.click(screen.getByRole("button", { name: "Start" }));

    await waitFor(() => {
      expect(launchInstance).toHaveBeenCalledWith({
        name: "alpha",
        mode: "headed",
        port: undefined,
      });
    });
    expect(fetchInstances).toHaveBeenCalledTimes(1);
    expect(useAppStore.getState().instances).toEqual(updatedInstances);
    expect(onClose).toHaveBeenCalledTimes(1);
  });
});
