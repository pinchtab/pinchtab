import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import CreateProfileModal from "./CreateProfileModal";

vi.mock("../../services/api", () => ({
  createProfile: vi.fn(),
}));

describe("CreateProfileModal", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("submits the local form and returns the created profile key", async () => {
    const { createProfile } = await import("../../services/api");
    const onClose = vi.fn();
    const onCreated = vi.fn();

    vi.mocked(createProfile).mockResolvedValue({
      status: "ok",
      id: "prof_work",
      name: "work",
    });

    render(
      <CreateProfileModal
        open={true}
        onClose={onClose}
        onCreated={onCreated}
      />,
    );

    await userEvent.type(
      screen.getByPlaceholderText("e.g. personal, work, scraping"),
      "work",
    );
    await userEvent.type(
      screen.getByPlaceholderText(
        "e.g. I need to access Gmail for the team account",
      ),
      "Team account access",
    );

    await userEvent.click(screen.getByRole("button", { name: "Create" }));

    await waitFor(() => {
      expect(createProfile).toHaveBeenCalledWith({
        name: "work",
        useWhen: "Team account access",
      });
    });
    expect(onClose).toHaveBeenCalledTimes(1);
    expect(onCreated).toHaveBeenCalledWith("prof_work");
  });

  it("resets its local fields when closed and reopened", async () => {
    const { rerender } = render(
      <CreateProfileModal
        open={true}
        onClose={() => {}}
        onCreated={() => {}}
      />,
    );

    const nameInput = screen.getByPlaceholderText(
      "e.g. personal, work, scraping",
    );
    await userEvent.type(nameInput, "scratch");

    rerender(
      <CreateProfileModal
        open={false}
        onClose={() => {}}
        onCreated={() => {}}
      />,
    );

    rerender(
      <CreateProfileModal
        open={true}
        onClose={() => {}}
        onCreated={() => {}}
      />,
    );

    expect(
      screen.getByPlaceholderText("e.g. personal, work, scraping"),
    ).toHaveValue("");
  });
});
