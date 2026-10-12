import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { homeFixture } from "../../lib/home-fixture";
import { HomeShell } from "./HomeShell";

describe("HomeHistory", () => {
  it("presents evidence-bounded Insights before the underlying Records", async () => {
    const user = userEvent.setup();
    render(<HomeShell destination="history" fixture={homeFixture("history")} previewEnabled />);

    expect(screen.getByRole("heading", { name: "Insights" })).toBeInTheDocument();
    expect(screen.getByText("Evidence before interpretation")).toBeInTheDocument();
    expect(screen.getByText("Candidate", { selector: "span" })).toBeInTheDocument();
    expect(screen.getByText("Directly observed")).toBeInTheDocument();
    expect(screen.getByText("Inference boundary")).toBeInTheDocument();
    expect(screen.getByText("Known gaps")).toBeInTheDocument();
    expect(screen.getByText(/no model or provider invoked/i)).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Why this?" }));
    expect(screen.getByText(/may help you decide/i)).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Confirm insight" }));
    expect(screen.getByText("Confirmed", { selector: "span" })).toBeInTheDocument();

    await user.click(screen.getByRole("tab", { name: "Records" }));
    expect(screen.getByRole("button", { name: "User correction recorded" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Work link proposed" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Tomorrow re-entry selected" })).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Work link proposed" }));

    expect(screen.getByText("Pitch-deck Project · no Outcome created in this preview.")).toBeInTheDocument();
    expect(screen.getByText("Waldo proposal · architecture preview")).toBeInTheDocument();
    expect(screen.getByText("No Work Outcome or responsibility link exists.")).toBeInTheDocument();
  });

  it("keeps supporting activity labelled as evidence rather than productivity", async () => {
    const user = userEvent.setup();
    render(<HomeShell destination="history" fixture={homeFixture("history")} previewEnabled />);

    await user.click(screen.getByRole("tab", { name: "Records" }));
    await user.click(screen.getByRole("tab", { name: "Supporting activity" }));

    expect(screen.getByRole("heading", { name: "Activity evidence, not productivity" })).toBeInTheDocument();
    expect(screen.getByText("Meeting note · corrected by you")).toBeInTheDocument();
    expect(screen.queryByText(/productivity score/i)).not.toBeInTheDocument();
    expect(screen.queryByText(/hours tracked/i)).not.toBeInTheDocument();
  });

  it("uses roving focus and arrow keys for insight and record layers", async () => {
    const user = userEvent.setup();
    render(<HomeShell destination="history" fixture={homeFixture("history")} previewEnabled />);

    const insights = screen.getByRole("tab", { name: "Insights" });
    const records = screen.getByRole("tab", { name: "Records" });
    expect(insights).toHaveAttribute("tabindex", "0");
    expect(records).toHaveAttribute("tabindex", "-1");
    insights.focus();
    await user.keyboard("{ArrowRight}");
    expect(records).toHaveFocus();
    expect(records).toHaveAttribute("aria-selected", "true");

    const continuity = screen.getByRole("tab", { name: "Continuity" });
    const activity = screen.getByRole("tab", { name: "Supporting activity" });
    continuity.focus();
    await user.keyboard("{ArrowLeft}");
    expect(activity).toHaveFocus();
    expect(activity).toHaveAttribute("aria-selected", "true");
  });

  it("keeps a local correction visible after applying it", async () => {
    const user = userEvent.setup();
    render(<HomeShell destination="history" fixture={homeFixture("history")} previewEnabled />);

    await user.click(screen.getByRole("button", { name: "Correct" }));
    await user.type(
      screen.getByRole("textbox", { name: /Correction for A short focus block/i }),
      "Leave the block intentionally uncommitted.",
    );
    await user.click(screen.getByRole("button", { name: "Apply correction" }));

    expect(screen.getByText("Corrected", { selector: "span" })).toBeInTheDocument();
    expect(screen.getByText("Correction applied")).toBeInTheDocument();
    expect(screen.getByText("Leave the block intentionally uncommitted.")).toBeInTheDocument();
  });

  it("returns focus to the selected continuity event from its narrow detail control", async () => {
    const user = userEvent.setup();
    render(<HomeShell destination="history" fixture={homeFixture("history")} previewEnabled />);

    await user.click(screen.getByRole("tab", { name: "Records" }));
    const event = screen.getByRole("button", { name: "Tomorrow re-entry selected" });
    await user.click(event);
    await user.click(screen.getByRole("button", { name: "Back to records" }));

    expect(event).toHaveFocus();
  });

  it("shows an honest canonical empty state without synthesized claims", () => {
    render(<HomeShell destination="history" fixture={homeFixture("history")} previewEnabled={false} />);

    expect(screen.getByRole("heading", { name: "Insights" })).toBeInTheDocument();
    expect(screen.getByText("No insight candidates yet")).toBeInTheDocument();
    expect(screen.queryByText("Candidate", { selector: "span" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Confirm insight" })).not.toBeInTheDocument();
  });
});
