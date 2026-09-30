import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import type { Instance } from "../types";
import InstanceCard from "./InstanceCard";

describe("InstanceCard", () => {
  it("shows 'just now' for a start time in the future", () => {
    const instance = {
      profileName: "work",
      port: "9868",
      headless: true,
      startTime: new Date(Date.now() + 60_000).toISOString(),
    } as Instance;

    render(
      <InstanceCard instance={instance} onOpen={() => {}} onStop={() => {}} />,
    );

    expect(screen.getByText("just now")).toBeInTheDocument();
  });
});
