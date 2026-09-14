import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, it } from "vitest";
import { MissionPlanView } from "./MissionPlanView";
import type { components } from "../../../api/schema";

const units: components["schemas"]["PlanWorkUnitResponse"][] = [
	{
		id: "b",
		title: "Publish analysis",
		dependsOn: ["a"],
		outputSummary: "Final report",
		approvedChecks: [],
		evidenceChecks: ["Report matches source"],
		criterionIds: ["c1"],
		requiredCapabilities: ["worktree.write"],
		stopConditions: ["Source changed"],
		verificationRequirement: "Review report",
		kind: "direct",
		contractRevisionNumber: 1,
	},
	{
		id: "a",
		title: "Read source",
		dependsOn: [],
		outputSummary: "Source notes",
		approvedChecks: [],
		evidenceChecks: ["Quotes attributed"],
		criterionIds: ["c1"],
		requiredCapabilities: ["worktree.read"],
		stopConditions: [],
		verificationRequirement: "Check notes",
		kind: "direct",
		contractRevisionNumber: 1,
	},
];

it("keeps selection and zoom across graph/table toggles and reordered refreshes", async () => {
	const user = userEvent.setup();
	const { rerender } = render(<MissionPlanView workUnits={units} />);
	await user.click(screen.getByRole("button", { name: /Publish analysis —/ }));
	expect(within(screen.getByTestId("mission-unit-detail")).getByText("Final report")).toBeVisible();
	await user.click(screen.getByRole("button", { name: /zoom in/i }));
	const graphList = within(screen.getByTestId("mission-work-unit-graph")).getByRole("list");
	const transform = graphList.style.transform;
	await user.click(screen.getByRole("button", { name: "Table" }));
	expect(screen.getByRole("row", { name: /Publish analysis/ })).toHaveAttribute("aria-selected", "true");
	await user.click(screen.getByRole("button", { name: "Read source" }));
	expect(within(screen.getByTestId("mission-unit-detail")).getByText("Source notes")).toBeVisible();
	rerender(<MissionPlanView workUnits={[...units].reverse()} />);
	await user.click(screen.getByRole("button", { name: "Graph" }));
	expect(screen.getByRole("button", { name: /Read source —/ })).toHaveAttribute("aria-current", "true");
	expect(graphList.style.transform).toBe(transform);
});

it("removes obsolete unit detail when a new Plan no longer contains the selection", async () => {
	const user = userEvent.setup();
	const { rerender } = render(<MissionPlanView workUnits={units} />);
	await user.click(screen.getByRole("button", { name: /Publish analysis —/ }));
	rerender(<MissionPlanView workUnits={[units[1]]} />);
	expect(screen.queryByTestId("mission-unit-detail")).not.toBeInTheDocument();
	expect(screen.getByText(/Select a WorkUnit/)).toBeVisible();
});

it("shows daemon dependency reasons and unknown proof without inventing readiness", async () => {
	const user = userEvent.setup();
	const schedule = {
		workUnits: units.map((workUnit) => ({
			workUnit,
			state: "blocked",
			blockedReason: "awaiting_dependency_proof",
			blockingDependencies: ["a"],
			criterionReady: null,
			attempts: [],
		})),
	} as unknown as components["schemas"]["ScheduleResponse"];
	render(<MissionPlanView workUnits={units} schedule={schedule} />);
	await user.click(screen.getByRole("button", { name: "Table" }));
	expect(within(screen.getByRole("table")).getAllByText("Waiting for proof from: Read source")).toHaveLength(2);
	await user.click(screen.getByRole("button", { name: "Publish analysis" }));
	expect(screen.getByText(/Proof readiness not reported/)).toBeVisible();
	expect(screen.queryByRole("button", { name: /start|accept|authorize/i })).not.toBeInTheDocument();
});

it("shows exact approved check argument boundaries and timeout before graph selection", () => {
 const argv = ["python3", "script with spaces.py", "--label=a b"];
 render(<MissionPlanView workUnits={[{ ...units[0], approvedChecks: [{ id: "check-1", criterionId: "c1", argv, timeoutSeconds: 17 }] }]} criterionText={() => "Every source attributed"} />);
 expect(screen.getByText(JSON.stringify(argv))).toBeVisible();
 expect(screen.getByText("Timeout: 17 seconds")).toBeVisible();
 expect(screen.getAllByText("Every source attributed").length).toBeGreaterThan(0);
});

it("engages the selected WorkUnit's newest Attempt instead of the global newest Attempt", async () => {
	const user = userEvent.setup();
	const selectedAttempt = {
		id: "attempt-for-b",
		number: 2,
		workUnitId: "b",
		sessions: [{ id: "ref-b", seq: 1, sessionId: "session-b", harness: "codex", mode: "tui", boundAt: "2026-08-30T00:00:00Z", runBriefCoreDigest: "b" }],
	} as unknown as components["schemas"]["AttemptResponse"];
	const globalAttempt = { ...selectedAttempt, id: "attempt-for-a", number: 3, workUnitId: "a" };
	const onOpenAttempt = vi.fn();
	render(<MissionPlanView attempts={[selectedAttempt, globalAttempt]} onOpenAttempt={onOpenAttempt} schedule={{ workUnits: units.map((workUnit) => ({ workUnit, state: "runnable", attempts: [], blockingDependencies: [], criterionReady: {} })) } as never} workUnits={units} />);
	await user.click(screen.getByRole("button", { name: /Publish analysis —/ }));
	await user.click(screen.getByRole("button", { name: "Engage" }));
	expect(onOpenAttempt).toHaveBeenCalledWith(selectedAttempt);
	expect(screen.queryByText("session-b")).not.toBeInTheDocument();
});

it("opens daemon execution detail for the selected WorkUnit on the execution surface", async () => {
	const user = userEvent.setup();
	const attempt = {
		id: "attempt-a1",
		number: 1,
		workUnitId: "a",
		status: "running",
		updatedAt: "2026-08-30T00:00:00Z",
		sessions: [{ id: "ref-a", seq: 1, sessionId: "session-a", harness: "codex", mode: "tui", boundAt: "2026-08-30T00:00:00Z", runBriefCoreDigest: "b" }],
	} as unknown as components["schemas"]["AttemptResponse"];
	const onOpenAttempt = vi.fn();
	const onReviewProof = vi.fn();
	const schedule = {
		workUnits: units.map((workUnit) => ({
			workUnit,
			state: workUnit.id === "a" ? "executing" : "blocked",
			blockedReason: workUnit.id === "b" ? "awaiting_dependency_proof" : undefined,
			blockingDependencies: workUnit.id === "b" ? ["a"] : [],
			criterionReady: { c1: workUnit.id === "a" },
			attempts: [],
		})),
	} as unknown as components["schemas"]["ScheduleResponse"];
	render(
		<MissionPlanView
			attempts={[attempt]}
			criterionText={() => "Every source attributed"}
			graphOnly
			onOpenAttempt={onOpenAttempt}
			onReviewProof={onReviewProof}
			schedule={schedule}
			workUnits={units}
		/>,
	);
	expect(screen.queryByTestId("mission-unit-execution-detail")).not.toBeInTheDocument();
	await user.click(screen.getByRole("button", { name: /Read source —/ }));
	const detail = screen.getByTestId("mission-unit-execution-detail");
	expect(within(detail).getByText(/Proof ready/)).toBeVisible();
	expect(within(detail).getByText(/Attempt #1 · running/)).toBeVisible();
	expect(within(detail).getByText(/Codex · tui/)).toBeVisible();
	await user.click(within(detail).getByRole("button", { name: "Engage" }));
	expect(onOpenAttempt).toHaveBeenCalledWith(attempt);
	await user.click(within(detail).getByRole("button", { name: "View Outcome result and receipts" }));
	expect(onReviewProof).toHaveBeenCalledTimes(1);
});

it("labels the CTA Inspect for an ended Attempt and Engage for a live one", async () => {
	const user = userEvent.setup();
	const mk = (id: string, phase: string, status: string) => ({
		id,
		number: id === "live" ? 2 : 1,
		workUnitId: "a",
		status,
		updatedAt: "2026-08-30T00:00:00Z",
		presentation: { phase, unconfirmed: false, endedUnclassified: false, nextAction: "x" },
		sessions: [{ id: `ref-${id}`, seq: 1, sessionId: `session-${id}`, harness: "codex", mode: "tui", boundAt: "2026-08-30T00:00:00Z", runBriefCoreDigest: "b" }],
	}) as unknown as components["schemas"]["AttemptResponse"];
	const schedule = {
		workUnits: units.map((workUnit) => ({
			workUnit,
			state: workUnit.id === "a" ? "executing" : "blocked",
			blockedReason: workUnit.id === "b" ? "awaiting_dependency_proof" : undefined,
			blockingDependencies: workUnit.id === "b" ? ["a"] : [],
			criterionReady: {},
			attempts: [],
		})),
	} as unknown as components["schemas"]["ScheduleResponse"];
	const onOpenAttempt = vi.fn();
	render(<MissionPlanView attempts={[mk("ended", "succeeded", "succeeded"), mk("live", "executing", "running")]} graphOnly onOpenAttempt={onOpenAttempt} schedule={schedule} workUnits={units} />);
	await user.click(screen.getByRole("button", { name: /Read source —/ }));
	const detail = screen.getByTestId("mission-unit-execution-detail");
	expect(within(detail).getByRole("button", { name: "Engage" })).toBeVisible();
	expect(within(detail).queryByRole("button", { name: "Inspect" })).not.toBeInTheDocument();
});

it("labels the CTA Inspect when the newest Attempt has ended", async () => {
	const user = userEvent.setup();
	const ended = {
		id: "ended",
		number: 1,
		workUnitId: "a",
		status: "succeeded",
		updatedAt: "2026-08-30T00:00:00Z",
		presentation: { phase: "succeeded", unconfirmed: false, endedUnclassified: false, nextAction: "x" },
		sessions: [{ id: "ref-ended", seq: 1, sessionId: "session-ended", harness: "codex", mode: "tui", boundAt: "2026-08-30T00:00:00Z", runBriefCoreDigest: "b" }],
	} as unknown as components["schemas"]["AttemptResponse"];
	const schedule = {
		workUnits: units.map((workUnit) => ({
			workUnit,
			state: workUnit.id === "a" ? "proven" : "runnable",
			blockedReason: undefined,
			blockingDependencies: [],
			criterionReady: {},
			attempts: [],
		})),
	} as unknown as components["schemas"]["ScheduleResponse"];
	render(<MissionPlanView attempts={[ended]} graphOnly onOpenAttempt={vi.fn()} schedule={schedule} workUnits={units} />);
	await user.click(screen.getByRole("button", { name: /Read source —/ }));
	const detail = screen.getByTestId("mission-unit-execution-detail");
	expect(within(detail).getByRole("button", { name: "Inspect" })).toBeVisible();
	expect(within(detail).queryByRole("button", { name: "Engage" })).not.toBeInTheDocument();
});

it("shows the daemon blocker and unreported proof readiness without inventing state", async () => {
	const user = userEvent.setup();
	const schedule = {
		workUnits: units.map((workUnit) => ({
			workUnit,
			state: "blocked",
			blockedReason: "awaiting_dependency_proof",
			blockedDetail: "Receipt att-9 holds the workspace lease.",
			blockingDependencies: ["a"],
			criterionReady: null,
			attempts: [],
		})),
	} as unknown as components["schemas"]["ScheduleResponse"];
	render(<MissionPlanView criterionText={() => "Every source attributed"} graphOnly schedule={schedule} workUnits={units} />);
	await user.click(screen.getByRole("button", { name: /Publish analysis —/ }));
	const detail = screen.getByTestId("mission-unit-execution-detail");
	expect(within(detail).getByText(/Waiting for proof from: Read source/)).toBeVisible();
	expect(within(detail).getByText("Receipt att-9 holds the workspace lease.")).toBeVisible();
	expect(within(detail).getByText(/Proof readiness not reported/)).toBeVisible();
	expect(within(detail).getByText("No Attempts recorded")).toBeVisible();
	expect(within(detail).queryByRole("button", { name: "Engage" })).not.toBeInTheDocument();
});

it("shows no execution detail before authorization because there are no execution facts", async () => {
	const user = userEvent.setup();
	render(<MissionPlanView graphOnly workUnits={units} />);
	await user.click(screen.getByRole("button", { name: /Read source —/ }));
	expect(screen.queryByTestId("mission-unit-execution-detail")).not.toBeInTheDocument();
});
