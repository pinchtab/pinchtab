import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi } from "vitest";
import InstanceListItem from "./InstanceListItem";
import type { Instance } from "../../generated/types";

const mockInstance: Instance = {
  id: "inst_123",
  profileId: "prof_456",
  profileName: "test-profile",
  port: "9868",
  headless: false,
  status: "running",
  startTime: new Date().toISOString(),
  attached: false,
};

describe("InstanceListItem", () => {
  it("renders instance name and port", () => {
    render(
      <InstanceListItem
        instance={mockInstance}
        tabCount={5}
        selected={false}
        onClick={() => {}}
      />,
    );

    expect(screen.getByText("test-profile")).toBeInTheDocument();
    expect(screen.getByText(":9868 · 5 tabs")).toBeInTheDocument();
  });

  it("shows running status badge", () => {
    render(
      <InstanceListItem
        instance={mockInstance}
        tabCount={0}
        selected={false}
        onClick={() => {}}
      />,
    );

    expect(screen.getByText("running")).toBeInTheDocument();
  });

  it("shows error status for errored instances", () => {
    const errorInstance = { ...mockInstance, status: "error" };
    render(
      <InstanceListItem
        instance={errorInstance}
        tabCount={0}
        selected={false}
        onClick={() => {}}
      />,
    );

    expect(screen.getByText("error")).toBeInTheDocument();
  });

  it("applies selected styles when selected", () => {
    render(
      <InstanceListItem
        instance={mockInstance}
        tabCount={0}
        selected={true}
        onClick={() => {}}
      />,
    );

    const button = screen.getByRole("button");
    expect(button).toHaveClass("border-primary");
  });

  it("calls onClick when clicked", async () => {
    const handleClick = vi.fn();
    render(
      <InstanceListItem
        instance={mockInstance}
        tabCount={0}
        selected={false}
        onClick={handleClick}
      />,
    );

    await userEvent.click(screen.getByRole("button"));
    expect(handleClick).toHaveBeenCalledTimes(1);
  });
});
