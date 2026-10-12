import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { homeFixture } from "../../lib/home-fixture";
import { HomeShell } from "./HomeShell";

describe("HomeOpenLoops", () => {
  it("explains a selected responsibility instead of presenting a task board", async () => {
    const user = userEvent.setup();
    render(
      <HomeShell
        destination="open_loops"
        fixture={homeFixture("open_loops")}
      />,
    );

    expect(screen.getByRole("heading", { name: "Open loops" })).toBeInTheDocument();
    expect(screen.getByRole("tab", { name: "Needs you 1" })).toHaveAttribute(
      "aria-selected",
      "true",
    );

    await user.click(screen.getByRole("button", { name: "Deck follow-up" }));

    expect(
      screen.getByText("Prepare the revised deck for Ashish; do not send it yet."),
    ).toBeInTheDocument();
    expect(screen.getByText("User-confirmed")).toBeInTheDocument();
    expect(screen.getByText("Recheck tomorrow morning")).toBeInTheDocument();
    expect(screen.getByText("Meeting note · corrected by you · 3:31 PM")).toBeInTheDocument();
  });

  it("keeps responsibility edits and Work continuation inside an explicit preview boundary", async () => {
    const user = userEvent.setup();
    render(
      <HomeShell
        destination="open_loops"
        fixture={homeFixture("open_loops")}
      />,
    );

    await user.click(screen.getByRole("button", { name: "Deck follow-up" }));
    await user.click(screen.getByRole("button", { name: "Continue in Work" }));

    expect(screen.getByRole("status", { name: "Open loop preview status" })).toHaveTextContent(
      "No Work Outcome or responsibility link has been created",
    );

    await user.click(screen.getByRole("button", { name: "Correct this" }));
    expect(screen.getByRole("status", { name: "Open loop preview status" })).toHaveTextContent(
      "Correction controls are local to this preview",
    );

    await user.click(screen.getByRole("button", { name: "Add context" }));
    expect(screen.getByRole("status", { name: "Open loop preview status" })).toHaveTextContent(
      "Context controls are local to this preview",
    );
  });

  it("moves between responsibility states and returns focus to the selected row", async () => {
    const user = userEvent.setup();
    render(
      <HomeShell
        destination="open_loops"
        fixture={homeFixture("open_loops")}
      />,
    );

    await user.click(screen.getByRole("tab", { name: "Waiting 1" }));
    expect(screen.getByRole("button", { name: "Vendor response" })).toBeInTheDocument();

    const row = screen.getByRole("button", { name: "Vendor response" });
    await user.click(row);
    await user.click(screen.getByRole("button", { name: "Back to open loops" }));

    expect(row).toHaveFocus();
  });
});
