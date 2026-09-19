import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { OutcomeInspector } from "./OutcomeInspector";
import type { MissionNodeRecord } from "../../hooks/useOutcome";
import type { MissionNodeView } from "./mission-presentation";

function record(overrides: Partial<MissionNodeRecord> = {}): MissionNodeRecord {
	return {
		workUnitId: "wu-1",
		planRevisionId: "plan-1",
		title: "Ship the thing",
		dependsOn: [],
		scheduleState: "runnable",
		blockingDependencies: [],
		criterionIds: ["c1", "c2"],
		criterionReady: { c1: true, c2: false },
		inputs: [],
		links: [],
		role: "worker",
		responsibility: "unconfirmed",
		updatedAt: "2026-09-17T00:00:00Z",
		generation: 1,
		...overrides,
	};
}

function view(overrides: Partial<MissionNodeView> = {}): MissionNodeView {
	return {
		workUnitId: "wu-1",
		title: "Ship the thing",
		status: { status: "runnable", label: "Ready", className: "text-status-ready", indicatorClassName: "bg-status-ready" },
		criteria: { kind: "ready", ready: 1, total: 2 },
		responsibilityLabel: "Unconfirmed",
		dependencyUnavailable: false,
		dependencyCount: 0,
		dependentCount: 0,
		generation: 1,
		...overrides,
	};
}

describe("OutcomeInspector", () => {
	it("renders objective, current state, and contract coverage by position (never a raw criterion id)", () => {
		render(<OutcomeInspector node={record()} onClose={() => {}} view={view()} />);
		expect(screen.getAllByText("Ship the thing").length).toBeGreaterThan(0);
		expect(screen.getByTestId("outcome-inspector-state")).toHaveTextContent("Ready");
		expect(screen.getByText("Criterion 1")).toBeInTheDocument();
		expect(screen.getByText("Criterion 2")).toBeInTheDocument();
		expect(screen.queryByText("c1")).not.toBeInTheDocument();
	});

	it("shows an explicit unavailable reading when criterionReady is null", () => {
		render(<OutcomeInspector node={record({ criterionReady: null })} onClose={() => {}} view={view()} />);
		expect(screen.getByTestId("contract-coverage-unavailable")).toBeInTheDocument();
	});

	it("renders typed Needs You content only when the node carries attention", () => {
		render(
			<OutcomeInspector
				node={record()}
				onClose={() => {}}
				view={view({
					attention: {
						kind: "needs_input",
						visualCategory: "input",
						label: "Needs input",
						summary: "The agent asked a question",
						className: "",
						indicatorClassName: "",
						markerKey: "wu-1:g:needs_input",
					},
				})}
			/>,
		);
		expect(screen.getByTestId("outcome-inspector-needs-you")).toHaveTextContent("The agent asked a question");
	});

	it("shows no attempt when the node has none, and the attempt label with session status when it does", () => {
		const { rerender } = render(<OutcomeInspector node={record()} onClose={() => {}} view={view()} />);
		expect(screen.getByText("No attempt yet")).toBeInTheDocument();

		rerender(
			<OutcomeInspector
				node={record({ currentAttempt: { attemptId: "attempt-1", number: 2, status: "running", createdAt: "", updatedAt: "" } })}
				onClose={() => {}}
				view={view({ attemptLabel: "Attempt 2", sessionStatusLabel: "Unknown" })}
			/>,
		);
		expect(screen.getByTestId("outcome-inspector-attempt")).toHaveTextContent("Attempt 2 — Unknown");
		expect(screen.queryByText(/Running/)).not.toBeInTheDocument();
	});

	it("offers the existing session destination only for a running bound session", async () => {
		const user = userEvent.setup();
		const onOpenSession = vi.fn();
		const runningBound = record({
			currentAttempt: {
				attemptId: "attempt-1",
				number: 1,
				status: "running",
				createdAt: "",
				updatedAt: "",
				session: { sessionId: "session-1", harness: "codex", mode: "tui", state: "live" },
			},
		});
		const { rerender } = render(<OutcomeInspector node={runningBound} onClose={() => {}} onOpenSession={onOpenSession} view={view()} />);
		await user.click(screen.getByTestId("outcome-inspector-open-session"));
		expect(onOpenSession).toHaveBeenCalledWith("session-1");

		rerender(<OutcomeInspector node={record({ currentAttempt: { ...runningBound.currentAttempt!, status: "ended" } })} onClose={() => {}} onOpenSession={onOpenSession} view={view()} />);
		expect(screen.queryByTestId("outcome-inspector-open-session")).not.toBeInTheDocument();
	});

	it("does not expose session navigation without an explicit destination callback", () => {
		render(<OutcomeInspector node={record({ currentAttempt: { attemptId: "attempt-1", number: 1, status: "running", createdAt: "", updatedAt: "", session: { sessionId: "session-1", harness: "codex", mode: "tui", state: "live" } } })} onClose={() => {}} view={view()} />);
		expect(screen.queryByTestId("outcome-inspector-open-session")).not.toBeInTheDocument();
	});

	it("keeps raw IDs confined to the collapsed technical-details section", () => {
		render(
			<OutcomeInspector
				node={record({ currentAttempt: { attemptId: "attempt-raw-id", number: 1, status: "running", createdAt: "", updatedAt: "" } })}
				onClose={() => {}}
				view={view({ attemptLabel: "Attempt 1" })}
			/>,
		);
		const technical = screen.getByTestId("outcome-inspector-technical");
		expect(technical).toHaveTextContent("wu-1");
		expect(technical).toHaveTextContent("attempt-raw-id");
	});

	it("calls onClose from the close control", async () => {
		const user = userEvent.setup();
		const onClose = vi.fn();
		render(<OutcomeInspector node={record()} onClose={onClose} view={view()} />);
		await user.click(screen.getByTestId("outcome-inspector-close"));
		expect(onClose).toHaveBeenCalledTimes(1);
	});
});
