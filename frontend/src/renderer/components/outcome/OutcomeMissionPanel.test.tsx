// Focused render test for the stable selectors used by the packaged Outcome
// journey harness, updated for the locked 2026-09-17 surface: the focused
// Outcome view has four tabs (overview / plan / run / result) and Decision
// history is a drawer, never a fifth tab. Every child surface is stubbed
// here - this test is about OutcomeMissionPanel's own markup, not its
// children's, which already have their own focused tests.
vi.mock("./MissionContractEditor", () => ({ MissionContractEditor: () => <div>contract editor stub</div> }));
vi.mock("./OutcomeDocumentsPanel", () => ({ OutcomeDocumentsPanel: () => null }));
vi.mock("./OutcomeDecideAuthorizeSurface", () => ({ OutcomeDecideAuthorizeSurface: () => null }));
vi.mock("./OutcomeRunSurface", () => ({ OutcomeRunSurface: () => null }));
vi.mock("./OutcomeProveCloseSurface", () => ({ OutcomeProveCloseSurface: () => null }));
vi.mock("./OutcomeDeliveryPanel", () => ({ OutcomeDeliveryPanel: () => null }));
vi.mock("./OutcomeDeletionControls", () => ({ OutcomeDeletionControls: () => null }));
vi.mock("./MissionUsage", () => ({ MissionUsage: () => null }));
vi.mock("../settings/ReasoningSettingsSection", () => ({ ReasoningSettingsSection: () => null }));
vi.mock("../../hooks/useOutcomeRunState", () => ({
	useOutcomeRunState: () => ({ data: undefined, isLoading: false, error: undefined, refetch: vi.fn() }),
}));
vi.mock("../../hooks/useSettings", () => ({ useSettings: () => ({ settings: { reasoning: { ready: true } } }) }));
vi.mock("../../lib/mission-attention", () => ({ runStateAttention: () => ({ lane: "define", reason: undefined }) }));
vi.mock("../../hooks/useEventsConnection", () => ({ useEventsConnection: () => "connected" }));
vi.mock("../../hooks/useWorkspaceQuery", () => ({ useWorkspaceQuery: () => ({ data: [{ id: "project-1", name: "kennel" }] }) }));

const OUTCOME = {
	id: "outcome-1",
	title: "Ship the thing",
	updatedAt: "2026-09-16T00:00:00Z",
	currentRevisionNumber: 2,
	currentRevision: { goal: "Ship the thing", criteria: [], constraints: [], nonGoals: [], stopConditions: [], review: "", authorityCeiling: null },
	history: [{ id: "rev-1", number: 1, createdAt: "2026-09-15T00:00:00Z", goal: "Original goal" }],
};

vi.mock("../../hooks/useOutcome", () => ({
	useOutcome: () => ({ outcome: OUTCOME, refetch: vi.fn(), failure: undefined }),
	useOutcomePlan: () => ({ plan: undefined, refetch: vi.fn() }),
	useOutcomeProof: () => ({ proof: undefined }),
}));

import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { it, expect, vi } from "vitest";
import { OutcomeMissionPanel } from "./OutcomeMissionPanel";

function renderPanel() {
	return render(
		<OutcomeMissionPanel
			outcomeId="outcome-1"
			projectId="project-1"
			expanded={false}
			onExpand={() => undefined}
			onClose={() => undefined}
		/>,
	);
}

it("exposes stable tab-nav and next-action selectors for the packaged Outcome journey harness", () => {
	renderPanel();
	for (const view of ["overview", "plan", "run", "result"]) {
		expect(screen.getByTestId(`mission-tab-${view}`)).toBeInTheDocument();
	}
	expect(screen.getByTestId("mission-review-plan-cta")).toBeInTheDocument();
	expect(screen.getByTestId("mission-glance-cta")).toBeInTheDocument();
});

it("renders exactly four tabs and never a history tab", () => {
	renderPanel();
	expect(screen.queryByTestId("mission-tab-contract")).not.toBeInTheDocument();
	expect(screen.queryByTestId("mission-tab-execution")).not.toBeInTheDocument();
	expect(screen.queryByTestId("mission-tab-history")).not.toBeInTheDocument();
	expect(screen.getByTestId("mission-history-open")).toBeInTheDocument();
});

it("opens Decision history in a drawer with the revision list, and closes it", async () => {
	const user = userEvent.setup();
	renderPanel();
	await user.click(screen.getByTestId("mission-history-open"));
	const dialog = await screen.findByRole("dialog");
	expect(within(dialog).getByText("Decision history")).toBeInTheDocument();
	const drawer = within(dialog).getByTestId("mission-history-drawer");
	expect(within(drawer).getByText("Original goal")).toBeInTheDocument();
	await user.click(within(dialog).getByRole("button", { name: /close/i }));
	expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
	// The four-tab nav is unaffected by the drawer cycle.
	for (const view of ["overview", "plan", "run", "result"]) {
		expect(screen.getByTestId(`mission-tab-${view}`)).toBeInTheDocument();
	}
});

it("keeps tab navigation working around the drawer", async () => {
	const user = userEvent.setup();
	renderPanel();
	await user.click(screen.getByTestId("mission-tab-plan"));
	expect(screen.getByTestId("mission-tab-plan")).toHaveAttribute("aria-pressed", "true");
	await user.click(screen.getByTestId("mission-history-open"));
	await screen.findByRole("dialog");
	await user.keyboard("{Escape}");
	expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
	expect(screen.getByTestId("mission-tab-plan")).toHaveAttribute("aria-pressed", "true");
});
