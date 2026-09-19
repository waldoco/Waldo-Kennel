import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import type { useOutcomeMission } from "../../hooks/useOutcome";
import { dummyMissionProjection } from "../../lib/mission-canvas-dummy";
import { MissionSessionHub } from "./MissionSessionHub";

type MissionQuery = ReturnType<typeof useOutcomeMission>;
function query(): MissionQuery {
	return { mission: dummyMissionProjection(), isLoading: false, isFetching: false, failure: undefined, refetch: vi.fn() };
}

describe("MissionSessionHub", () => {
	it("keeps DAG drill-down and session navigation as separate one-click doors", () => {
		const onDrillDown = vi.fn();
		const onOpenSession = vi.fn();
		render(<MissionSessionHub missionQuery={query()} onDrillDown={onDrillDown} onOpenSession={onOpenSession} />);

		expect(screen.getByTestId("mission-session-harness-rollup")).toHaveTextContent("codex · 1");
		expect(screen.getByText(/Contract r7/)).toBeInTheDocument();
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
});
