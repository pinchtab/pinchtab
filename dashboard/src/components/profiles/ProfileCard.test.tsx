import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi } from "vitest";
import ProfileCard from "./ProfileCard";
import type { Profile, Instance } from "../../types";

const mockProfile: Profile = {
  id: "prof_123",
  name: "test-profile",
  created: new Date().toISOString(),
  lastUsed: new Date().toISOString(),
  diskUsage: 1024 * 1024 * 100, // 100MB
  sizeMB: 100,
  running: false,
};

const mockRunningInstance: Instance = {
  id: "inst_123",
  profileId: "prof_123",
  profileName: "test-profile",
  port: "9868",
  headless: false,
  status: "running",
  startTime: new Date().toISOString(),
  attached: false,
};

const mockErrorInstance: Instance = {
  ...mockRunningInstance,
  status: "error",
  error: "Failed to start Chrome",
};

describe("ProfileCard", () => {
  it("renders profile name", () => {
    render(<ProfileCard profile={mockProfile} onLaunch={() => {}} />);
    expect(screen.getByText("test-profile")).toBeInTheDocument();
  });

  it("shows stopped badge when no instance", () => {
    render(<ProfileCard profile={mockProfile} onLaunch={() => {}} />);
    expect(screen.getByText("stopped")).toBeInTheDocument();
  });

  it("shows port badge when running", () => {
    render(
      <ProfileCard
        profile={mockProfile}
        instance={mockRunningInstance}
        onLaunch={() => {}}
      />,
    );
    expect(screen.getByText(":9868")).toBeInTheDocument();
  });

  it("shows error badge when errored", () => {
    render(
      <ProfileCard
        profile={mockProfile}
        instance={mockErrorInstance}
        onLaunch={() => {}}
      />,
    );
    expect(screen.getByText("error")).toBeInTheDocument();
  });

  it("shows error message when errored", () => {
    render(
      <ProfileCard
        profile={mockProfile}
        instance={mockErrorInstance}
        onLaunch={() => {}}
      />,
    );
    expect(screen.getByText("Failed to start Chrome")).toBeInTheDocument();
  });

  it("displays size in MB", () => {
    render(<ProfileCard profile={mockProfile} onLaunch={() => {}} />);
    expect(screen.getByText("100 MB")).toBeInTheDocument();
  });

  it("displays dash for missing size", () => {
    const noSizeProfile = { ...mockProfile, sizeMB: undefined };
    render(<ProfileCard profile={noSizeProfile} onLaunch={() => {}} />);
    expect(screen.getAllByText("—")).toHaveLength(2); // Size and Account both show —
  });

  it("displays account email", () => {
    const profileWithEmail = {
      ...mockProfile,
      accountEmail: "test@pinchtab.com",
    };
    render(<ProfileCard profile={profileWithEmail} onLaunch={() => {}} />);
    expect(screen.getByText("test@pinchtab.com")).toBeInTheDocument();
  });

  it("displays account name when no email", () => {
    const profileWithName = { ...mockProfile, accountName: "Test User" };
    render(<ProfileCard profile={profileWithName} onLaunch={() => {}} />);
    expect(screen.getByText("Test User")).toBeInTheDocument();
  });

  it("displays useWhen text", () => {
    const profileWithUseWhen = {
      ...mockProfile,
      useWhen: "For testing purposes",
    };
    render(<ProfileCard profile={profileWithUseWhen} onLaunch={() => {}} />);
    expect(screen.getByText("For testing purposes")).toBeInTheDocument();
  });

  it("shows Start button when stopped", () => {
    render(<ProfileCard profile={mockProfile} onLaunch={() => {}} />);
    expect(screen.getByRole("button", { name: "Start" })).toBeInTheDocument();
  });

  it("shows Stop button when running", () => {
    render(
      <ProfileCard
        profile={mockProfile}
        instance={mockRunningInstance}
        onLaunch={() => {}}
        onStop={() => {}}
      />,
    );
    expect(screen.getByRole("button", { name: "Stop" })).toBeInTheDocument();
  });

  it("calls onLaunch when Start clicked", async () => {
    const handleLaunch = vi.fn();
    render(<ProfileCard profile={mockProfile} onLaunch={handleLaunch} />);

    await userEvent.click(screen.getByRole("button", { name: "Start" }));
    expect(handleLaunch).toHaveBeenCalledTimes(1);
  });

  it("calls onStop when Stop clicked", async () => {
    const handleStop = vi.fn();
    render(
      <ProfileCard
        profile={mockProfile}
        instance={mockRunningInstance}
        onLaunch={() => {}}
        onStop={handleStop}
      />,
    );

    await userEvent.click(screen.getByRole("button", { name: "Stop" }));
    expect(handleStop).toHaveBeenCalledTimes(1);
  });

  it("shows Details button when onDetails provided", () => {
    render(
      <ProfileCard
        profile={mockProfile}
        onLaunch={() => {}}
        onDetails={() => {}}
      />,
    );
    expect(screen.getByRole("button", { name: "Details" })).toBeInTheDocument();
  });

  it("calls onDetails when Details clicked", async () => {
    const handleDetails = vi.fn();
    render(
      <ProfileCard
        profile={mockProfile}
        onLaunch={() => {}}
        onDetails={handleDetails}
      />,
    );

    await userEvent.click(screen.getByRole("button", { name: "Details" }));
    expect(handleDetails).toHaveBeenCalledTimes(1);
  });
});
