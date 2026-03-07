import { render, screen } from "@testing-library/react";
import { describe, it, expect } from "vitest";
import Badge from "./Badge";

describe("Badge", () => {
  it("renders children", () => {
    render(<Badge>Status</Badge>);
    expect(screen.getByText("Status")).toBeInTheDocument();
  });

  it("applies default variant styles", () => {
    render(<Badge>Default</Badge>);
    const badge = screen.getByText("Default");
    expect(badge).toHaveClass(
      "border",
      "border-border-subtle",
      "bg-white/[0.03]",
      "text-text-secondary",
    );
  });

  it("applies success variant styles", () => {
    render(<Badge variant="success">Running</Badge>);
    const badge = screen.getByText("Running");
    expect(badge).toHaveClass("text-success");
  });

  it("applies danger variant styles", () => {
    render(<Badge variant="danger">Error</Badge>);
    const badge = screen.getByText("Error");
    expect(badge).toHaveClass("text-destructive");
  });

  it("applies warning variant styles", () => {
    render(<Badge variant="warning">Warning</Badge>);
    const badge = screen.getByText("Warning");
    expect(badge).toHaveClass("text-warning");
  });

  it("applies info variant styles", () => {
    render(<Badge variant="info">Info</Badge>);
    const badge = screen.getByText("Info");
    expect(badge).toHaveClass("text-info");
  });

  it("has correct base styles", () => {
    render(<Badge>Test</Badge>);
    const badge = screen.getByText("Test");
    expect(badge).toHaveClass(
      "inline-flex",
      "items-center",
      "rounded-sm",
      "font-semibold",
      "uppercase",
    );
  });
});
