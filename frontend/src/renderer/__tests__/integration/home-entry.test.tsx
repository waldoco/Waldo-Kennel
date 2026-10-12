import { act, render, screen } from "@testing-library/react";
import type { ComponentType } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ShellProvider, type ShellContextValue } from "../../lib/shell-context";

vi.mock("@tanstack/react-router", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@tanstack/react-router")>()),
  createFileRoute: (id: string) => (options: unknown) => ({ id, options }),
}));

import { Route } from "../../routes/_shell.home";
import { Route as OpenLoopsRouteDefinition } from "../../routes/_shell.home_.open-loops";
import { Route as MemoryRouteDefinition } from "../../routes/_shell.home_.memory";
import { Route as DailyCloseRouteDefinition } from "../../routes/_shell.home_.daily-close";
import { Route as HistoryRouteDefinition } from "../../routes/_shell.home_.history";

function shellContext(state: "ready" | "stopped" | "error"): ShellContextValue {
  return {
    daemonStatus: { state } as ShellContextValue["daemonStatus"],
    workspaceStartupState: "ready",
    createProject: async () => undefined,
    initializeProjectRepository: async () => undefined,
  };
}

async function renderHomeRoute(state: "ready" | "stopped" | "error") {
  const Component = Route.options.component as ComponentType;
  await act(async () => {
    render(
      <ShellProvider value={shellContext(state)}>
        <Component />
      </ShellProvider>,
    );
  });
}

describe("Home entry route", () => {
  afterEach(() => {
    vi.useRealTimers();
  });

  it("registers Home as a directly selectable destination", () => {
    expect(Route.id).toBe("/_shell/home");
  });

  it("keeps Today and the four Home destinations on stable routes", () => {
    expect(Route.id).toBe("/_shell/home");
    expect(OpenLoopsRouteDefinition.id).toBe("/_shell/home_/open-loops");
    expect(MemoryRouteDefinition.id).toBe("/_shell/home_/memory");
    expect(DailyCloseRouteDefinition.id).toBe("/_shell/home_/daily-close");
    expect(HistoryRouteDefinition.id).toBe("/_shell/home_/history");
  });

	it("renders the split Today brief and Catch Up workspace when the daemon is ready", async () => {
		vi.useFakeTimers({ toFake: ["Date"] });
		vi.setSystemTime(new Date(2026, 7, 22, 9, 0, 0));
		await renderHomeRoute("ready");
		expect(
			await screen.findByRole("heading", { name: "Home" }),
		).toBeInTheDocument();
		expect(
			screen.getByRole("heading", { name: "Good morning, Shivansh." }),
		).toBeInTheDocument();
		expect(screen.getByRole("heading", { name: "Catch up" })).toBeInTheDocument();
    expect(
      screen.getByRole("textbox", { name: "Quick capture" }),
    ).toBeInTheDocument();
  });

	it("selects the default Today chapter once from local opening time", async () => {
		vi.useFakeTimers({ toFake: ["Date"] });
		vi.setSystemTime(new Date(2026, 7, 22, 14, 0, 0));
		await renderHomeRoute("ready");

		expect(
			screen.getByRole("heading", { name: "Good afternoon, Shivansh." }),
		).toBeInTheDocument();
		expect(
			screen.getByRole("heading", { name: "Before your next thing" }),
		).toBeInTheDocument();
	});

  it.each(["stopped", "error"] as const)(
    "renders Home unavailable when the daemon is %s",
    async (state) => {
      await renderHomeRoute(state);

      expect(
        await screen.findByText("Home facts are unavailable"),
      ).toBeInTheDocument();
      expect(
        screen.queryByRole("heading", { name: "What matters now" }),
      ).not.toBeInTheDocument();
    },
  );
});
