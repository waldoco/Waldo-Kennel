import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { homeFixture } from "../../lib/home-fixture";
import { HomeShell } from "./HomeShell";

describe("HomeMemoryReview", () => {
  it("keeps a memory candidate inspectable and visibly outside durable memory", () => {
    render(<HomeShell destination="memory" fixture={homeFixture("memory")} />);

    expect(screen.getByRole("heading", { name: "Memory review" })).toBeInTheDocument();
    expect(screen.getByText("Candidate — not memory")).toBeInTheDocument();
    expect(
      screen.getByText("Ashish should receive the deck only after the revision is reviewed."),
    ).toBeInTheDocument();
    expect(
      screen.getByText("Valid until the deck is sent or this instruction is corrected"),
    ).toBeInTheDocument();
    expect(screen.getByText("Source contains a 14 minute audio gap")).toBeInTheDocument();
    expect(screen.getByText("Ordinary context")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /remember this/i })).not.toBeInTheDocument();
  });

  it.each([
    ["Forget", "Forgotten in this preview. No durable memory changed."],
    ["Correct", "Correction opened in this preview. No durable memory changed."],
    ["Defer", "Deferred in this preview. No durable memory changed."],
  ])("keeps %s as a local review outcome", async (action, expected) => {
    const user = userEvent.setup();
    render(<HomeShell destination="memory" fixture={homeFixture("memory")} />);

    await user.click(screen.getByRole("button", { name: action }));

    expect(screen.getByRole("status", { name: "Memory review status" })).toHaveTextContent(
      expected,
    );
  });
});
