import { render, screen, within } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

vi.mock("@tanstack/react-router", () => ({
  Link: ({ children, to, ...props }: React.AnchorHTMLAttributes<HTMLAnchorElement> & { to: string }) => (
    <a href={`#${to}`} {...props}>{children}</a>
  ),
}));

import { HomeNavigation } from "./HomeNavigation";

describe("HomeNavigation", () => {
  it("keeps Today, Chat, and Open Loops primary while exposing continuity screens", () => {
    render(<HomeNavigation destination="memory" variant="sidebar" />);

    const primary = screen.getByRole("group", { name: "Primary Home destinations" });
    expect(within(primary).getByRole("link", { name: "Today" })).toBeInTheDocument();
    expect(within(primary).getByRole("link", { name: "Chat" })).toHaveAttribute("href", "#/home/chat");
    expect(within(primary).getByRole("link", { name: "Open loops" })).toBeInTheDocument();

    const continuity = screen.getByRole("group", { name: "Review and continuity" });
    expect(within(continuity).getByRole("link", { name: "Daily close" })).toHaveAttribute(
      "href",
      "#/home/daily-close",
    );
    expect(within(continuity).getByRole("link", { name: "Memory review" })).toHaveAttribute(
      "aria-current",
      "page",
    );
    expect(within(continuity).getByRole("link", { name: "Insights" })).toHaveAttribute(
      "href",
      "#/home/history",
    );
  });

  it("does not promote supporting routes into the compact horizontal navigation", () => {
    render(<HomeNavigation destination="today" variant="panel" />);

    expect(screen.getByRole("link", { name: "Today" })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Chat" })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Open loops" })).toBeInTheDocument();
    expect(
      screen.queryByRole("group", { name: "Review and continuity" }),
    ).not.toBeInTheDocument();
    expect(screen.queryByRole("link", { name: "Memory review" })).not.toBeInTheDocument();
  });
});
