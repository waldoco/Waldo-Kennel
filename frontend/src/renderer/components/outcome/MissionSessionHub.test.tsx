import { fireEvent, render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

const { connectionState } = vi.hoisted(() => ({ connectionState: { value: "connected" as "connected" | "disconnected" | "idle" } }));
vi.mock("../../hooks/useEventsConnection", () => ({ useEventsConnection: () => connectionState.value }));

import type { useOutcomeMission } from "../../hooks/useOutcome";
import { dummyMissionProjection } from "../../lib/mission-canvas-dummy";
import { MissionSessionHub } from "./MissionSessionHub";

type MissionQuery = ReturnType<typeof useOutcomeMission>;
function query(): MissionQuery {
	return { mission: dummyMissionProjection(), isLoading: false, isFetching: false, failure: undefined, refetch: vi.fn() };
}

describe("MissionSessionHub", () => {
	beforeEach(() => { connectionState.value = "connected"; });
	it("keeps DAG drill-down and session navigation as separate one-click doors", () => {
		const onDrillDown = vi.fn();
		const onOpenSession = vi.fn();
		render(<MissionSessionHub missionQuery={query()} onDrillDown={onDrillDown} onOpenSession={onOpenSession} />);

		expect(screen.getByTestId("mission-session-harness-rollup")).toHaveTextContent("codex · 1");
		expect(screen.getByText(/Contract revision 7/)).toBeInTheDocument();
		fireEvent.click(screen.getByText("Refuse duplicate resume for one session id"));
		expect(onDrillDown).toHaveBeenCalledWith("wu-handshake");
		expect(onOpenSession).not.toHaveBeenCalled();

		fireEvent.click(screen.getByRole("button", { name: /Open session for Refuse duplicate resume/ }));
		expect(onOpenSession).toHaveBeenCalledWith("session-resume-2");
		expect(onDrillDown).toHaveBeenCalledTimes(1);
	});

	it("shows only current running bound sessions", () => {
		const mission = dummyMissionProjection();
		mission.nodes = mission.nodes.map((node) => node.currentAttempt ? { ...node, currentAttempt: { ...node.currentAttempt, status: "ended" } } : node);
		render(<MissionSessionHub missionQuery={{ ...query(), mission }} onDrillDown={vi.fn()} onOpenSession={vi.fn()} />);
		expect(screen.getByTestId("mission-session-hub-empty")).toBeInTheDocument();
		expect(screen.queryByRole("button", { name: /Open session for/ })).not.toBeInTheDocument();
	});
	it("shows an honest initial failure with retry, never the empty claim", () => {
		const refetch = vi.fn();
		render(<MissionSessionHub missionQuery={{ ...query(), mission: undefined, failure: { message: "mission unavailable" } as MissionQuery["failure"], refetch }} onDrillDown={vi.fn()} onOpenSession={vi.fn()} />);
		expect(screen.getByTestId("mission-session-hub-error")).toHaveTextContent("mission unavailable");
		expect(screen.queryByTestId("mission-session-hub-empty")).not.toBeInTheDocument();
		fireEvent.click(screen.getByTestId("mission-session-hub-retry"));
		expect(refetch).toHaveBeenCalledOnce();
	});

	it("keeps last confirmed cards with a refresh-failed reading", () => {
		const initial = query();
		const { rerender } = render(<MissionSessionHub missionQuery={initial} onDrillDown={vi.fn()} onOpenSession={vi.fn()} />);
		expect(screen.getByText("Refuse duplicate resume for one session id")).toBeInTheDocument();
		rerender(<MissionSessionHub missionQuery={{ ...initial, mission: undefined, failure: { message: "refresh failed" } as MissionQuery["failure"] }} onDrillDown={vi.fn()} onOpenSession={vi.fn()} />);
		expect(screen.getByTestId("mission-session-hub-refresh-failed")).toHaveTextContent("refresh failed");
		expect(screen.getByText("Refuse duplicate resume for one session id")).toBeInTheDocument();
		expect(screen.queryByTestId("mission-session-hub-empty")).not.toBeInTheDocument();
	});

	it("freezes cached cards on disconnect, suppresses refresh-failed and session doors", () => {
		const initial = query();
		const refetch = vi.fn();
		const { rerender } = render(<MissionSessionHub missionQuery={{ ...initial, refetch }} onDrillDown={vi.fn()} onOpenSession={vi.fn()} />);
		connectionState.value = "disconnected";
		rerender(<MissionSessionHub missionQuery={{ ...initial, mission: undefined, failure: { message: "refresh also failed" } as MissionQuery["failure"], refetch }} onDrillDown={vi.fn()} onOpenSession={vi.fn()} />);
		expect(screen.getByTestId("mission-session-hub-stale")).toHaveTextContent("showing the last confirmed Mission");
		expect(screen.getByText("Refuse duplicate resume for one session id")).toBeInTheDocument();
		expect(screen.queryByTestId("mission-session-hub-refresh-failed")).not.toBeInTheDocument();
		expect(screen.queryByTestId("mission-session-hub-empty")).not.toBeInTheDocument();
		expect(screen.getByRole("button", { name: /Open session for Refuse duplicate resume/ })).toBeDisabled();
		fireEvent.click(screen.getByTestId("mission-session-hub-stale-refresh"));
		expect(refetch).toHaveBeenCalledOnce();
	});

});
