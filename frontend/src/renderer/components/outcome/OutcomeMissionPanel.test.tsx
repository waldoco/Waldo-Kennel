// Focused render test for the stable selectors added for the packaged
// Outcome journey harness (docs/handoffs/2026-09-16-macos-outcome-journey-
// harness-plan.md §6, §0.3): the tab-nav buttons are the only way the real
// UI moves between Contract/Plan/Execution/Result/History, and had no
// data-testid before this change. Every child surface is stubbed here — this
// test is about OutcomeMissionPanel's own markup, not its children's, which
// already have their own focused tests.
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
	history: [],
};

vi.mock("../../hooks/useOutcome", () => ({
	useOutcome: () => ({ outcome: OUTCOME, refetch: vi.fn(), failure: undefined }),
	useOutcomePlan: () => ({ plan: undefined, refetch: vi.fn() }),
	useOutcomeProof: () => ({ proof: undefined }),
}));

import { render, screen } from "@testing-library/react";
import { it, expect, vi } from "vitest";
import { OutcomeMissionPanel } from "./OutcomeMissionPanel";

it("exposes stable tab-nav and next-action selectors for the packaged Outcome journey harness", () => {
	render(
		<OutcomeMissionPanel
			outcomeId="outcome-1"
			projectId="project-1"
			expanded={false}
			onExpand={() => undefined}
			onClose={() => undefined}
		/>,
	);
	for (const view of ["contract", "plan", "execution", "result", "history"]) {
		expect(screen.getByTestId(`mission-tab-${view}`)).toBeInTheDocument();
	}
	expect(screen.getByTestId("mission-review-plan-cta")).toBeInTheDocument();
	expect(screen.getByTestId("mission-glance-cta")).toBeInTheDocument();
});
