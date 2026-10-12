import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { homeFixture } from "../../lib/home-fixture";
import { HomeShell } from "./HomeShell";

describe("HomeDailyClose", () => {
  it("reviews evidence, unresolved responsibility, and source gaps before Closure", () => {
    render(
      <HomeShell
        destination="daily_close"
        fixture={homeFixture("daily_close")}
      />,
    );

    expect(
      screen.getByRole("heading", { name: "Close the day deliberately" }),
    ).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "What became true" })).toBeInTheDocument();
    expect(
      screen.getByRole("heading", { name: "What remains unresolved" }),
    ).toBeInTheDocument();
    expect(
      screen.getByText("Meeting audio was unavailable from 3:10–3:24 PM."),
    ).toBeInTheDocument();
    expect(screen.getByRole("radiogroup", { name: "Disposition for Deck follow-up" })).toBeInTheDocument();
  });

  it("produces only a local Daily Close preview receipt", async () => {
    const user = userEvent.setup();
    const fixture = homeFixture("daily_close");
    render(<HomeShell destination="daily_close" fixture={fixture} />);

    const disposition = screen.getByRole("radiogroup", {
      name: "Disposition for Deck follow-up",
    });
    await user.click(within(disposition).getByRole("radio", { name: "Resume tomorrow" }));
    await user.click(screen.getByRole("button", { name: "Add close note" }));
    await user.type(screen.getByRole("textbox", { name: "Close note" }), "Open the latest deck first.");
    await user.click(
      screen.getByRole("button", { name: "Preview closing the day" }),
    );

    expect(screen.getByRole("heading", { name: "Daily close preview" })).toBeInTheDocument();
    expect(
      screen.getByText("Preview receipt. Nothing was saved or closed."),
    ).toBeInTheDocument();
    expect(screen.getByText("Resume from the corrected deck follow-up")).toBeInTheDocument();
    expect(screen.getByText("Open the latest deck first.")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Return to Today" })).toHaveAttribute("href", "#/home");
    expect(screen.getByRole("link", { name: "Open Insights" })).toHaveAttribute(
      "href",
      "#/home/history",
    );
    expect(fixture.closureReview.unresolved[0]).not.toHaveProperty("disposition");
  });
});
