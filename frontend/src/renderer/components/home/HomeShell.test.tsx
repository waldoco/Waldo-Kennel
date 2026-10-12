import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { homeFixture } from "../../lib/home-fixture";
import { WaldoLauncher } from "../waldo/WaldoLauncher";
import { WaldoRailProvider } from "../waldo/WaldoRailContext";
import { HomeShell } from "./HomeShell";

describe("HomeShell", () => {
  it("opens Today as a morning brief with the one decision under Needs you", () => {
    render(<HomeShell fixture={homeFixture("today")} />);

    expect(screen.getByRole("heading", { name: "Home" })).toBeInTheDocument();
    expect(
      screen.getByRole("heading", { name: "Good morning, Shivansh." }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("heading", { name: "Morning brief" }),
    ).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "Needs you" })).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "Catch up" })).toBeInTheDocument();
    expect(screen.getByText("Preview context — not live data")).toBeInTheDocument();
    expect(
      screen.getByText("Prepare the revised deck for Ashish; do not send it."),
    ).toBeInTheDocument();
    expect(
      screen.getByText("Meeting audio unavailable from 3:10–3:24 PM"),
    ).toBeInTheDocument();
    expect(
      screen.getAllByRole("textbox", { name: "Quick capture" }),
    ).toHaveLength(1);
    expect(
      screen.queryByRole("link", { name: /Work \(recommended\)/i }),
    ).not.toBeInTheDocument();
  });

  it("temporarily gives Waldo the Today inspector and restores the glance", async () => {
    const user = userEvent.setup();
    render(
      <WaldoRailProvider>
        <WaldoLauncher />
        <HomeShell fixture={homeFixture("today")} />
      </WaldoRailProvider>,
    );

    expect(screen.getByRole("complementary", { name: "Today at a glance" })).toBeInTheDocument();
    const launcher = screen.getByRole("button", { name: "Open Waldo" });
    await user.click(launcher);

    expect(screen.getByRole("region", { name: "Waldo" })).toBeInTheDocument();
    expect(
      screen.queryByRole("complementary", { name: "Today at a glance" }),
    ).not.toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "Needs you" })).toBeInTheDocument();
    expect(screen.getAllByRole("textbox", { name: "Quick capture" })).toHaveLength(1);

    await user.click(screen.getByRole("button", { name: "Close Waldo" }));
    expect(screen.getByRole("complementary", { name: "Today at a glance" })).toBeInTheDocument();
    await waitFor(() => expect(launcher).toHaveFocus());
  });

  it("opens Waldo as a layer without unmounting supporting Home screens", async () => {
    const user = userEvent.setup();
    render(
      <WaldoRailProvider>
        <WaldoLauncher />
        <HomeShell
          destination="history"
          fixture={homeFixture("history")}
        />
      </WaldoRailProvider>,
    );

    await user.click(screen.getByRole("button", { name: "Open Waldo" }));

    expect(screen.getByRole("region", { name: "Waldo" })).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "Insights" })).toBeInTheDocument();
    expect(screen.queryByRole("textbox", { name: "Quick capture" })).not.toBeInTheDocument();
  });

  it("keeps inferred work visibly separate from proposed Waldo suggestions", () => {
    render(<HomeShell fixture={homeFixture("today")} />);

    const todo = screen.getByRole("list", { name: "To do" });
    const suggestions = screen.getByRole("group", { name: "Waldo suggests" });

    expect(within(todo).getByText("Prepare the revised deck")).toBeInTheDocument();
    expect(within(suggestions).getByText("Draft a reply to Ashish")).toBeInTheDocument();
    expect(screen.getByText("Proposed, not added")).toBeInTheDocument();
  });

  it("shows what Waldo is carrying and one exact way back in", () => {
    render(<HomeShell fixture={homeFixture("today")} />);

    expect(screen.getByRole("heading", { name: "Waldo is carrying" })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Vendor response" })).toHaveAttribute(
      "href",
      "#/home/open-loops",
    );
    expect(screen.queryByRole("link", { name: "Deck follow-up" })).not.toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Open the loop" })).toHaveAttribute(
      "href",
      "#/home/open-loops",
    );
  });

  it("keeps the Brief's proposal local to the preview", async () => {
    const user = userEvent.setup();
    render(<HomeShell fixture={homeFixture("today")} />);

    await user.click(screen.getByRole("button", { name: "Add to today" }));
    expect(
      within(screen.getByRole("group", { name: "Waldo suggests" })).getByRole("status"),
    ).toHaveTextContent("Added in this preview. Nothing was saved.");
  });

  it("recalibrates Today around the next commitment in the afternoon", () => {
    render(
      <HomeShell
        fixture={homeFixture("today", "ready", {
          dayPhase: "afternoon",
          contextFlow: "before_next",
        })}
      />,
    );

    expect(
      screen.getByRole("heading", { name: "Good afternoon, Shivansh." }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("heading", { name: "Afternoon brief" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("heading", { name: "Before your next thing" }),
    ).toBeInTheDocument();
    expect(screen.getByText("Pricing workshop")).toBeInTheDocument();
    expect(screen.getByPlaceholderText("Note what changed…")).toBeInTheDocument();
    expect(screen.getAllByRole("textbox", { name: "Quick capture" })).toHaveLength(1);
  });

  it("shows an honest replan when a prior assumption changed", () => {
    render(
      <HomeShell
        fixture={homeFixture("today", "ready", {
          dayPhase: "afternoon",
          contextFlow: "plans_changed",
        })}
      />,
    );

    expect(
      screen.getByRole("heading", { name: "Plans changed" }),
    ).toBeInTheDocument();
    expect(
      screen.getByText("The pricing workshop would start at 3:30 PM."),
    ).toBeInTheDocument();
    expect(screen.getByText("The organizer moved it to 4:30 PM.")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Release" })).toBeInTheDocument();
  });

  it("invites explicit Closure from the evening chapter without starting it", () => {
    render(
      <HomeShell
        fixture={homeFixture("today", "ready", {
          dayPhase: "evening",
          contextFlow: "evening_review",
        })}
      />,
    );

    expect(
      screen.getByRole("heading", { name: "Good evening, Shivansh." }),
    ).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "Evening review" })).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Review the evening transition" }),
    ).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "Review the deck follow-up" }),
    ).not.toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Start the daily close" })).toHaveAttribute(
      "href",
      "#/home/daily-close",
    );
    expect(screen.getByPlaceholderText("Leave context for tomorrow…")).toBeInTheDocument();
  });

  it("lets quiet focus stay quiet when no intervention earns attention", () => {
    render(
      <HomeShell
        fixture={homeFixture("today", "ready", {
          dayPhase: "afternoon",
          contextFlow: "quiet_focus",
        })}
      />,
    );

    expect(screen.getByRole("heading", { name: "Quiet focus" })).toBeInTheDocument();
    expect(screen.getByText("Nothing needs your judgment right now.")).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "Review the deck follow-up" }),
    ).not.toBeInTheDocument();
  });

  it("keeps Home useful when capture is off without pressuring activation", () => {
    render(<HomeShell fixture={homeFixture("today", "capture_off")} />);

    expect(screen.getByText("Capture is off")).toBeInTheDocument();
    expect(
      screen.getByText(/Home still works from explicit notes and confirmed responsibilities/i),
    ).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: /enable capture/i }),
    ).not.toBeInTheDocument();
  });

  it("does not present unavailable Home facts as an empty Home", () => {
    render(<HomeShell fixture={homeFixture("today", "offline")} />);

    expect(screen.getByText("Home facts are unavailable")).toBeInTheDocument();
    expect(
      screen.getByText(/Waldo cannot tell whether nothing changed/i),
    ).toBeInTheDocument();
    expect(screen.queryByText("Nothing needs you")).not.toBeInTheDocument();
  });

  it.each([
    ["partial", "Some sources are unavailable"],
    ["capture_off", "Capture is off"],
    ["stale", "Home facts may be out of date"],
    ["offline", "Home facts are unavailable"],
  ] as const)("labels %s fixture facts truthfully", (availability, expected) => {
    render(<HomeShell fixture={homeFixture("today", availability)} />);

    expect(
      screen.getByRole("status", { name: "Home availability" }),
    ).toHaveTextContent(expected);
  });

  it("moves focus to the Needs you decision from the meaningful item", async () => {
    const user = userEvent.setup();
    render(<HomeShell fixture={homeFixture("today")} />);

    await user.click(
      screen.getByRole("button", { name: "Review the deck follow-up" }),
    );

    expect(screen.getByRole("heading", { name: "Needs you" })).toHaveFocus();
    expect(
      screen.getByText("I'll send Ashish the revised deck tomorrow."),
    ).toBeInTheDocument();
    expect(
      screen.getByText("Prepare the revised deck for Ashish; do not send it."),
    ).toBeInTheDocument();
    expect(screen.getByText("Known capture gap")).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Continue in Work" }),
    ).toBeInTheDocument();
  });

  it("keeps Continue in Work truthful and preview-only", async () => {
    const user = userEvent.setup();
    render(<HomeShell fixture={homeFixture("today")} />);

    await user.click(
      screen.getByRole("button", { name: "Review the deck follow-up" }),
    );
    await user.click(screen.getByRole("button", { name: "Continue in Work" }));

    expect(
      screen.getByRole("region", { name: "Work handoff preview" }),
    ).toHaveTextContent("Preview only");
    expect(
      screen.getByText(/No Outcome or responsibility link has been created/i),
    ).toBeInTheDocument();
  });

  it("keeps an unsaved Quick capture draft while reviewing the decision", async () => {
    const user = userEvent.setup();
    render(<HomeShell fixture={homeFixture("today")} />);

    await user.type(
      screen.getByRole("textbox", { name: "Quick capture" }),
      "Call Mum after dinner",
    );
    await user.click(
      screen.getByRole("button", { name: "Review the deck follow-up" }),
    );
    expect(screen.getByRole("textbox", { name: "Quick capture" })).toHaveValue(
      "Call Mum after dinner",
    );
  });

  it("returns focus and exact panel scroll after inspecting provenance", async () => {
    const user = userEvent.setup();
    render(<HomeShell fixture={homeFixture("today")} />);
    const panel = screen.getByRole("region", { name: "Home" });
    Object.defineProperty(panel, "scrollTop", {
      configurable: true,
      value: 248,
      writable: true,
    });

    const inspect = screen.getByRole("button", { name: "Inspect source" });
    await user.click(inspect);
    const dialog = screen.getByRole("dialog", { name: "Source provenance" });
    expect(dialog).toHaveFocus();
    expect(dialog).not.toHaveAttribute("aria-modal");
    await user.keyboard("{Escape}");

    expect(inspect).toHaveFocus();
    expect(panel.scrollTop).toBe(248);
  });

  it.each([
    ["open_loops", "Open loops"],
    ["memory", "Memory review"],
    ["daily_close", "Daily close"],
    ["history", "Insights"],
  ] as const)(
    "renders the %s destination without repeating Quick capture",
    (destination, heading) => {
      render(
        <HomeShell
          destination={destination}
          fixture={homeFixture(destination)}
        />,
      );

      expect(screen.getByRole("heading", { name: heading })).toBeInTheDocument();
      expect(
        screen.queryByRole("textbox", { name: "Quick capture" }),
      ).not.toBeInTheDocument();
    },
  );

  it("shows responsibility meaning and recheck context in Open Loops", () => {
    render(
      <HomeShell
        destination="open_loops"
        fixture={homeFixture("open_loops")}
      />,
    );

    const loop = screen.getByRole("article", { name: "Deck follow-up" });
    expect(loop).toHaveTextContent("Owner: You");
    expect(loop).toHaveTextContent("Recheck tomorrow morning");
    expect(loop).toHaveTextContent("User-confirmed");
  });

  it("keeps Daily Close distinct from responsibility closure", () => {
    render(
      <HomeShell
        destination="daily_close"
        fixture={homeFixture("daily_close")}
      />,
    );

    expect(screen.getByText("What became true")).toBeInTheDocument();
    expect(screen.getByText("What remains unresolved")).toBeInTheDocument();
    expect(
      screen.getByText("A review never closes a responsibility by itself."),
    ).toBeInTheDocument();
  });

  it("keeps continuity Records underneath Insights rather than activity scoring", async () => {
    const user = userEvent.setup();
    render(
      <HomeShell
        destination="history"
        fixture={homeFixture("history")}
        previewEnabled
      />,
    );

    await user.click(screen.getByRole("tab", { name: "Records" }));
    const history = screen.getByRole("list", { name: "Continuity history" });
    expect(within(history).getByText("User correction recorded")).toBeInTheDocument();
    expect(within(history).getByText("Work link proposed")).toBeInTheDocument();
    expect(within(history).getByText("Tomorrow re-entry selected")).toBeInTheDocument();
    expect(screen.queryByText(/productivity score/i)).not.toBeInTheDocument();
  });
});
