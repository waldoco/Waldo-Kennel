import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import type { MissionRecord } from "../../hooks/useOutcome";
import { MissionWorkUnitList } from "./MissionWorkUnitList";

vi.mock("../../hooks/useEventsConnection", () => ({ useEventsConnection: () => "connected" }));

// Security canary (evidence bar item, invariant L): no bearer/pairing-secret/
// verifier/owner-token value can reach the DOM through this surface, even if
// a future `/mission` response carries one as an extra field this renderer
// was never taught. The adapter only ever reads named fields off the raw
// record into typed view objects — nothing here spreads or serializes the
// raw record into markup — so a stray secret field is structurally inert.
const SECRET_VALUE = "sk-live-should-never-render-ABCDEF123456";

function missionWithSecretField(): MissionRecord {
	const node = {
		workUnitId: "wu-1",
		planRevisionId: "plan-1",
		title: "Ship the thing",
		dependsOn: [],
		scheduleState: "runnable",
		blockingDependencies: [],
		criterionIds: [],
		criterionReady: {},
		responsibility: "unconfirmed",
		updatedAt: "2026-09-17T00:00:00Z",
		generation: 1,
		nextAction: "start",
		// Simulated schema drift: an authority-shaped field this renderer was
		// never taught to read. It must never reach the DOM.
		bearerToken: SECRET_VALUE,
		pairingSecret: SECRET_VALUE,
		ownerVerifier: SECRET_VALUE,
	} as unknown as MissionRecord["nodes"][number];

	return {
		version: 1,
		outcomeId: "out-1",
		missionId: "mission-1",
		contractRevisionNumber: 1,
		planRevisionId: "plan-1",
		planRevisionNumber: 1,
		topologyFingerprint: "fp-1",
		topologyGeneration: 1,
		generation: 1,
		updatedAt: "2026-09-17T00:00:00Z",
		nodes: [node],
		edges: [],
	};
}

describe("Mission security canary", () => {
	it("never renders a bearer/pairing-secret/verifier-shaped field into the DOM", () => {
		const mission = missionWithSecretField();
		const { container } = render(
			<MissionWorkUnitList
				missionQuery={{ mission, isLoading: false, isFetching: false, failure: undefined, refetch: vi.fn() }}
				planApproved
			/>,
		);
		expect(container.innerHTML).not.toContain(SECRET_VALUE);
		expect(screen.queryByText(SECRET_VALUE)).not.toBeInTheDocument();
	});

	it("never renders a secret-shaped field into the inspector, including its collapsed technical-details section", async () => {
		const mission = missionWithSecretField();
		const { container, getByTestId } = render(
			<MissionWorkUnitList
				missionQuery={{ mission, isLoading: false, isFetching: false, failure: undefined, refetch: vi.fn() }}
				planApproved
			/>,
		);
		getByTestId("mission-list-row-wu-1").click();
		expect(await screen.findByTestId("outcome-inspector")).toBeInTheDocument();
		expect(container.innerHTML).not.toContain(SECRET_VALUE);
	});
});
