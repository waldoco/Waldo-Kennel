import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";

// Drives the Act & Observe stage against a mocked HTTP client only.
//
// Locked contract under test: the surface renders ONLY daemon-derived state
// (no local derivation of unconfirmed/ended), an unknown state is
// distinguishable from dead, provider completion is never presented as done,
// recovery verbs hit the custody-safe route, and no provider name is ever
// rendered as policy.
const { getMock, postMock } = vi.hoisted(() => ({
	getMock: vi.fn(),
	postMock: vi.fn(),
}));

vi.mock("../../lib/api-client", () => ({
	apiClient: { GET: getMock, POST: postMock },
	apiErrorCode: (error: unknown) =>
		typeof error === "object" && error !== null && "code" in error
			? String((error as { code: unknown }).code)
			: undefined,
	apiErrorMessage: (error: unknown) =>
		typeof error === "object" && error !== null && "message" in error
			? String((error as { message: unknown }).message)
			: error instanceof Error
				? error.message
				: "Request failed",
	hasTrustedApiBaseUrl: () => true,
}));

vi.mock("../../hooks/useEventsConnection", () => ({ useEventsConnection: () => "connected" }));

import { useUiStore } from "../../stores/ui-store";

import { OutcomeRunSurface } from "./OutcomeRunSurface";

function planEnvelope(status: string) {
	return {
		plan: {
			id: "plan-1",
			outcomeId: "out-1",
			number: 1,
			contractRevisionNumber: 1,
			status,
			summary: "One direct Work Unit",
			workUnits: [
				{
					id: "wu-1",
					kind: "direct",
					title: "Deliver Local Focus Ledger",
					contractRevisionNumber: 1,
					outputSummary: "Working feature",
					evidenceChecks: ["checks pass"],
					verificationRequirement: "Deterministic checks",
					stopConditions: ["stop before remote effects"],
				},
			],
			grants: [],
			runBriefCoreDigest: "a".repeat(64),
			createdAt: "2026-08-24T09:00:00Z",
		},
	};
}

function scheduleEnvelope() {
	const plan = planEnvelope("approved").plan;
	return {
		schedule: {
			outcomeId: "out-1",
			plan,
			workUnits: plan.workUnits.map((workUnit) => ({
				workUnit,
				state: "runnable",
				attempts: [],
				blockingDependencies: [],
				criterionReady: {},
			})),
			nextRunnableWorkUnitId: "wu-1",
		},
	};
}

function missionEnvelope() {
	const plan = planEnvelope("approved").plan;
	return {
		mission: {
			version: 1,
			outcomeId: "out-1",
			missionId: "out-1:mission",
			contractRevisionNumber: 1,
			planRevisionId: plan.id,
			planRevisionNumber: plan.number,
			topologyFingerprint: "fp-test",
			topologyGeneration: plan.number,
			generation: 1,
			updatedAt: "2026-08-24T09:00:00Z",
			nodes: plan.workUnits.map((workUnit) => ({
				workUnitId: workUnit.id,
				planRevisionId: plan.id,
				title: workUnit.title,
				dependsOn: [],
				scheduleState: "runnable",
				blockingDependencies: [],
				criterionIds: [],
				criterionReady: {},
				responsibility: "unconfirmed",
				updatedAt: "2026-08-24T09:00:00Z",
				generation: 1,
				nextAction: "start",
			})),
			edges: [],
			nextRunnableWorkUnitId: "wu-1",
		},
	};
}

type attemptOverrides = {
	id?: string;
	number?: number;
	status?: string;
	unconfirmed?: boolean;
	phase?: string;
	attention?: string;
	nextAction?: string;
	fence?: Record<string, unknown> | null;
};

function attemptEnvelope(overrides: attemptOverrides = {}) {
	const status = overrides.status ?? "running";
	const phase = overrides.phase ?? (status === "running" ? "executing" : status);
	return {
		attempt: {
			id: overrides.id ?? "att-1",
			outcomeId: "out-1",
			planRevisionId: "plan-1",
			workUnitId: "wu-1",
			number: overrides.number ?? 1,
			status,
			contractRevisionNumber: 1,
			sessions: [{ id: "asr-1", seq: 1, sessionId: "provider-x", harness: "codex", mode: "tui", runBriefCoreDigest: "b".repeat(64), boundAt: "2026-08-24T09:00:00Z" }],
			observations: [{ id: "obs-1", seq: 1, kind: "contained", createdAt: "2026-08-24T09:00:00Z" }],
			receipts: [],
			fence: overrides.fence === null ? undefined : overrides.fence ?? { id: "fence-1", subject: "project:mer", issuedAt: "2026-08-24T09:00:00Z" },
			presentation: {
				phase,
				unconfirmed: overrides.unconfirmed ?? false,
				endedUnclassified: false,
				attention: overrides.attention,
				nextAction: overrides.nextAction ?? "Waiting — observe.",
			},
			createdAt: "2026-08-24T09:00:00Z",
			updatedAt: "2026-08-24T09:00:00Z",
		},
	};
}

function renderSurface(props: { onReviewProof?: () => void } = {}) {
	const queryClient = new QueryClient({
		defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
	});
	return render(
		<QueryClientProvider client={queryClient}>
			<OutcomeRunSurface onReviewProof={props.onReviewProof} outcomeId="out-1" />
		</QueryClientProvider>,
	);
}

beforeEach(() => {
	getMock.mockReset();
	postMock.mockReset();
	// The Mission List/Graph reading is sticky: every test starts from the
	// List default with nothing stored.
	useUiStore.setState({ missionViewMode: "list" });
	window.localStorage.removeItem("kennel.mission.viewMode");
});

describe("OutcomeRunSurface", () => {
	it("hands an observed attempt into Prove & Close without claiming completion is acceptance", async () => {
		const user = userEvent.setup();
		const onReviewProof = vi.fn();
		getMock.mockImplementation((url: string) => {
			if (url === "/api/v1/outcomes/{outcomeId}/plans/{planId}/schedule") return Promise.resolve({ data: scheduleEnvelope(), error: undefined });
			if (url === "/api/v1/outcomes/{outcomeId}/plans/{planId}/mission") return Promise.resolve({ data: missionEnvelope(), error: undefined });
			if (url === "/api/v1/outcomes/{outcomeId}/plan") {
				return Promise.resolve({ data: planEnvelope("approved"), error: undefined });
			}
			if (url === "/api/v1/outcomes/{outcomeId}/attempts") {
				return Promise.resolve({ data: { attempts: [attemptEnvelope({ status: "succeeded", phase: "succeeded" }).attempt] }, error: undefined });
			}
			return Promise.resolve({ data: undefined, error: { code: "NOT_FOUND", message: url } });
		});

		renderSurface({ onReviewProof });
		const proofCard = await screen.findByTestId("outcome-run-proof-card");
		expect(proofCard.textContent).toMatch(/completion is only a fact/i);
		await user.click(screen.getByTestId("outcome-run-review-proof"));
		expect(onReviewProof).toHaveBeenCalledOnce();
	});

	it("shows the waiting-for-plan card when no approved plan exists and never offers start", async () => {
		getMock.mockImplementation((url: string) => {
			if (url === "/api/v1/outcomes/{outcomeId}/plans/{planId}/schedule") return Promise.resolve({ data: scheduleEnvelope(), error: undefined });
			if (url === "/api/v1/outcomes/{outcomeId}/plans/{planId}/mission") return Promise.resolve({ data: missionEnvelope(), error: undefined });
			if (url === "/api/v1/outcomes/{outcomeId}/plan") {
				return Promise.resolve({ data: planEnvelope("proposed"), error: undefined });
			}
			if (url === "/api/v1/outcomes/{outcomeId}/attempts") {
				return Promise.resolve({ data: { attempts: [] }, error: undefined });
			}
			return Promise.resolve({ data: undefined, error: { code: "NOT_FOUND", message: url } });
		});
		renderSurface();
		expect(await screen.findByTestId("outcome-run-needs-plan")).toBeDefined();
		expect(screen.queryByTestId("outcome-run-start")).toBeNull();
	});

	it("offers the governed start once the plan is approved and no attempt exists", async () => {
		const user = userEvent.setup();
		getMock.mockImplementation((url: string) => {
 if(url.endsWith("/run")) return Promise.resolve({data:{runState:{outcomeId:"out-1", projectId:"p", state:"needs_you", freshness:{contractRevisionNumber:1, planRevisionId:"plan-1", proofGeneration:0}, eligibleActions:[{action:"start",available:true}]}}});
			if (url === "/api/v1/outcomes/{outcomeId}/plans/{planId}/schedule") return Promise.resolve({ data: scheduleEnvelope(), error: undefined });
			if (url === "/api/v1/outcomes/{outcomeId}/plans/{planId}/mission") return Promise.resolve({ data: missionEnvelope(), error: undefined });
			if (url === "/api/v1/outcomes/{outcomeId}/plan") {
				return Promise.resolve({ data: planEnvelope("approved"), error: undefined });
			}
			if (url === "/api/v1/outcomes/{outcomeId}/attempts") {
				return Promise.resolve({ data: { attempts: [] }, error: undefined });
			}
			return Promise.resolve({ data: undefined, error: { code: "NOT_FOUND", message: url } });
		});
		postMock.mockResolvedValue({
			data: attemptEnvelope(),
			error: undefined,
		});
		renderSurface();
		const button = await screen.findByTestId("outcome-run-start");
		await user.click(button);
		await waitFor(() => {
			expect(postMock).toHaveBeenCalledWith(
				"/api/v1/outcomes/{outcomeId}/run",
				expect.objectContaining({
					params: { path: { outcomeId: "out-1" } },
					body: expect.objectContaining({ planRevisionId: "plan-1", requestKey: expect.any(String) }),
				}),
			);
		});
	});

	it("renders a healthy run as Waiting without any Needs You banner", async () => {
		getMock.mockImplementation((url: string) => {
			if (url === "/api/v1/outcomes/{outcomeId}/plans/{planId}/schedule") return Promise.resolve({ data: scheduleEnvelope(), error: undefined });
			if (url === "/api/v1/outcomes/{outcomeId}/plans/{planId}/mission") return Promise.resolve({ data: missionEnvelope(), error: undefined });
			if (url === "/api/v1/outcomes/{outcomeId}/plan") {
				return Promise.resolve({ data: planEnvelope("approved"), error: undefined });
			}
			if (url === "/api/v1/outcomes/{outcomeId}/attempts") {
				return Promise.resolve({ data: { attempts: [attemptEnvelope().attempt] }, error: undefined });
			}
			return Promise.resolve({ data: undefined, error: { code: "NOT_FOUND", message: url } });
		});
		renderSurface();
		expect(await screen.findByTestId("outcome-run-waiting")).toBeDefined();
		expect(screen.queryByTestId("outcome-run-needs-you")).toBeNull();
		expect(screen.queryByTestId("outcome-run-action-required")).toBeNull();
		// Zero client-side provider-name policy: the recorded session fact may
		// exist in the envelope but never surfaces as UI copy.
		expect(screen.queryByText(/codex/i)).toBeNull();
	});

	it("shows only the execution graph before the Attempt lineage, without repeating Plan detail", async () => {
		getMock.mockImplementation((url: string) => {
			if (url === "/api/v1/outcomes/{outcomeId}/plans/{planId}/schedule") return Promise.resolve({ data: scheduleEnvelope(), error: undefined });
			if (url === "/api/v1/outcomes/{outcomeId}/plans/{planId}/mission") return Promise.resolve({ data: missionEnvelope(), error: undefined });
			if (url === "/api/v1/outcomes/{outcomeId}/plan") return Promise.resolve({ data: planEnvelope("approved"), error: undefined });
			if (url === "/api/v1/outcomes/{outcomeId}/attempts") return Promise.resolve({ data: { attempts: [attemptEnvelope().attempt] }, error: undefined });
			return Promise.resolve({ data: undefined, error: { code: "NOT_FOUND", message: url } });
		});
		renderSurface();

		const schedule = await screen.findByTestId("outcome-run-schedule");
		expect(within(schedule).getByTestId("mission-work-unit-graph")).toBeInTheDocument();
		expect(within(schedule).queryByRole("button", { name: "Table" })).not.toBeInTheDocument();
		expect(within(schedule).queryByTestId("mission-unit-detail")).not.toBeInTheDocument();
		expect(await screen.findByTestId("mission-work-unit-list")).toBeInTheDocument();
	});

	it("opens the selected WorkUnit's execution detail with attempt lineage and result navigation", async () => {
		const user = userEvent.setup();
		const onReviewProof = vi.fn();
		getMock.mockImplementation((url: string) => {
			if (url === "/api/v1/outcomes/{outcomeId}/plans/{planId}/schedule") return Promise.resolve({ data: scheduleEnvelope(), error: undefined });
			if (url === "/api/v1/outcomes/{outcomeId}/plans/{planId}/mission") return Promise.resolve({ data: missionEnvelope(), error: undefined });
			if (url === "/api/v1/outcomes/{outcomeId}/plan") return Promise.resolve({ data: planEnvelope("approved"), error: undefined });
			if (url === "/api/v1/outcomes/{outcomeId}/attempts") return Promise.resolve({ data: { attempts: [attemptEnvelope().attempt] }, error: undefined });
			if (url === "/api/v1/outcomes/{outcomeId}/proof") {
				return Promise.resolve({
					data: {
						proof: {
							outcomeId: "out-1",
							result: {
							changes: [
								{ workUnitId: "wu-1", attemptId: "att-1", files: [{ path: "src/ledger.ts", changeKind: "modified", digest: "d1" }], truncated: false, retentionState: "available", artifactVersion: 1 },
								{ workUnitId: "wu-2", attemptId: "att-2", files: [{ path: "docs/notes.md", changeKind: "added", digest: "d2" }], truncated: false, retentionState: "available", artifactVersion: 1 },
								],
							},
						},
					},
					error: undefined,
				});
			}
			return Promise.resolve({ data: undefined, error: { code: "NOT_FOUND", message: url } });
		});
		renderSurface({ onReviewProof });

		const schedule = await screen.findByTestId("outcome-run-schedule");
		expect(within(schedule).queryByTestId("mission-unit-execution-detail")).not.toBeInTheDocument();
		await user.click(within(schedule).getByRole("button", { name: /Deliver Local Focus Ledger —/ }));
		const detail = await screen.findByTestId("mission-unit-execution-detail");
		expect(within(detail).getByText(/Attempt #1 · running/)).toBeInTheDocument();
		expect(within(detail).getByRole("button", { name: "Engage" })).toBeInTheDocument();
		// Only this WorkUnit's measured changes surface; other units' files stay out.
		const changes = within(detail).getByTestId("mission-unit-changes");
		expect(within(changes).getByText(/src\/ledger\.ts · modified/)).toBeInTheDocument();
		expect(within(changes).queryByText(/docs\/notes\.md/)).toBeNull();
		await user.click(within(detail).getByRole("button", { name: "View Outcome result and receipts" }));
		expect(onReviewProof).toHaveBeenCalledTimes(1);
	});

	it("distinguishes unconfirmed from dead and routes contain/reconcile through recovery", async () => {
		const user = userEvent.setup();
		getMock.mockImplementation((url: string) => {
			if (url === "/api/v1/outcomes/{outcomeId}/plans/{planId}/schedule") return Promise.resolve({ data: scheduleEnvelope(), error: undefined });
			if (url === "/api/v1/outcomes/{outcomeId}/plans/{planId}/mission") return Promise.resolve({ data: missionEnvelope(), error: undefined });
			if (url === "/api/v1/outcomes/{outcomeId}/plan") {
				return Promise.resolve({ data: planEnvelope("approved"), error: undefined });
			}
			if (url === "/api/v1/outcomes/{outcomeId}/attempts") {
				return Promise.resolve({
					data: {
						attempts: [
							attemptEnvelope({
								unconfirmed: true,
								phase: "unconfirmed",
								nextAction: "Liveness is unproven — contain and reconcile before replacing.",
							}).attempt,
						],
					},
					error: undefined,
				});
			}
			return Promise.resolve({ data: undefined, error: { code: "NOT_FOUND", message: url } });
		});
		postMock.mockResolvedValue({ data: attemptEnvelope(), error: undefined });
		renderSurface();

		const needsYou = await screen.findByTestId("outcome-run-needs-you");
		expect(needsYou.textContent).toMatch(/Liveness unproven/);
		await user.click(screen.getByTestId("outcome-run-contain"));
		await waitFor(() => {
			expect(postMock).toHaveBeenCalledWith(
				"/api/v1/outcomes/{outcomeId}/attempts/{attemptId}/recovery",
				expect.objectContaining({ body: { action: "contain", confirmProviderStopped: false } }),
			);
		});
		await user.click(await screen.findByTestId("outcome-run-reconcile"));
		await waitFor(() => {
			expect(postMock).toHaveBeenCalledWith(
				"/api/v1/outcomes/{outcomeId}/attempts/{attemptId}/recovery",
				expect.objectContaining({ body: { action: "reconcile", confirmProviderStopped: false } }),
			);
		});
		// Unconfirmed is NOT declared dead.
		expect(screen.queryByTestId("outcome-run-action-required")).toBeNull();
	});

	it("releases custody only behind an explicit two-step owner-containment assertion", async () => {
		const user = userEvent.setup();
		getMock.mockImplementation((url: string) => {
			if (url === "/api/v1/outcomes/{outcomeId}/plans/{planId}/schedule") return Promise.resolve({ data: scheduleEnvelope(), error: undefined });
			if (url === "/api/v1/outcomes/{outcomeId}/plans/{planId}/mission") return Promise.resolve({ data: missionEnvelope(), error: undefined });
			if (url === "/api/v1/outcomes/{outcomeId}/plan") {
				return Promise.resolve({ data: planEnvelope("approved"), error: undefined });
			}
			if (url === "/api/v1/outcomes/{outcomeId}/attempts") {
				return Promise.resolve({
					data: {
						attempts: [
							attemptEnvelope({ unconfirmed: true, phase: "unconfirmed" }).attempt,
						],
					},
					error: undefined,
				});
			}
			return Promise.resolve({ data: undefined, error: { code: "NOT_FOUND", message: url } });
		});
		postMock.mockResolvedValue({ data: attemptEnvelope(), error: undefined });
		renderSurface();
		await screen.findByTestId("outcome-run-needs-you");

		// The serious action is present but INERT until armed.
		expect(screen.queryByTestId("outcome-run-owner-stop-confirm")).toBeNull();
		await user.click(screen.getByTestId("outcome-run-owner-stop"));

		const confirmPanel = await screen.findByTestId("outcome-run-owner-stop-confirm");
		// Serious copy states the cost: custody releases on the owner's word.
		expect(confirmPanel.textContent).toMatch(/cannot prove|release custody/i);
		await user.click(screen.getByTestId("outcome-run-owner-stop-back"));
		expect(screen.queryByTestId("outcome-run-owner-stop-confirm")).toBeNull();

		await user.click(screen.getByTestId("outcome-run-owner-stop"));
		await screen.findByTestId("outcome-run-owner-stop-confirm");
		await user.click(screen.getByTestId("outcome-run-owner-stop-assert"));
		await waitFor(() => {
			expect(postMock).toHaveBeenCalledWith(
				"/api/v1/outcomes/{outcomeId}/attempts/{attemptId}/recovery",
				expect.objectContaining({ body: { action: "reconcile", confirmProviderStopped: true } }),
			);
		});
	});

	it("presents an ended attempt as result-unclassified, never as success", async () => {
		getMock.mockImplementation((url: string) => {
			if (url === "/api/v1/outcomes/{outcomeId}/plans/{planId}/schedule") return Promise.resolve({ data: scheduleEnvelope(), error: undefined });
			if (url === "/api/v1/outcomes/{outcomeId}/plans/{planId}/mission") return Promise.resolve({ data: missionEnvelope(), error: undefined });
			if (url === "/api/v1/outcomes/{outcomeId}/plan") {
				return Promise.resolve({ data: planEnvelope("approved"), error: undefined });
			}
			if (url === "/api/v1/outcomes/{outcomeId}/attempts") {
				return Promise.resolve({
					data: {
						attempts: [
							attemptEnvelope({
								status: "reconciled",
								phase: "ended_unclassified",
							}).attempt,
						],
					},
					error: undefined,
				});
			}
			return Promise.resolve({ data: undefined, error: { code: "NOT_FOUND", message: url } });
		});
		renderSurface();
		const card = await screen.findByTestId("outcome-run-ended-unclassified");
		expect(card.textContent).toMatch(/not final acceptance|nicht die endgültige|nunca es la aceptación|n'est jamais|最終受入では|최종 승인이 아닙니다|não é a aceitação|绝不是最终验收/i);
		// No success badge for an ended-unclassified attempt.
		expect(screen.getByTestId("outcome-run-status").textContent).not.toMatch(/succeeded|erfolgreich|exitoso|réussie|成功|성공|bem-sucedida/);
		// The daemon says a replacement requires owner authorization, so the
		// same detail that explains the blocker must expose that exact action.
		expect(screen.getByTestId("outcome-run-replace")).toBeInTheDocument();
	});

	it("offers replacement for a lost attempt through the recovery route", async () => {
		const user = userEvent.setup();
		getMock.mockImplementation((url: string) => {
			if (url === "/api/v1/outcomes/{outcomeId}/plans/{planId}/schedule") return Promise.resolve({ data: scheduleEnvelope(), error: undefined });
			if (url === "/api/v1/outcomes/{outcomeId}/plans/{planId}/mission") return Promise.resolve({ data: missionEnvelope(), error: undefined });
			if (url === "/api/v1/outcomes/{outcomeId}/plan") {
				return Promise.resolve({ data: planEnvelope("approved"), error: undefined });
			}
			if (url === "/api/v1/outcomes/{outcomeId}/attempts") {
				return Promise.resolve({
					data: {
						attempts: [
							attemptEnvelope({ id: "att-0", number: 1, status: "running", phase: "executing" }).attempt,
							attemptEnvelope({ id: "att-lost", number: 2, status: "lost", phase: "suspect_lost", fence: null }).attempt,
						],
					},
					error: undefined,
				});
			}
			return Promise.resolve({ data: undefined, error: { code: "NOT_FOUND", message: url } });
		});
		postMock.mockResolvedValue({ data: attemptEnvelope(), error: undefined });
		renderSurface();

		await screen.findByTestId("outcome-run-attempt-att-lost");
		await user.click(screen.getByTestId("outcome-run-replace-confirm"));
		await waitFor(() => {
			expect(postMock).toHaveBeenCalledWith(
				"/api/v1/outcomes/{outcomeId}/attempts/{attemptId}/recovery",
				expect.objectContaining({ body: { action: "replace", confirmProviderStopped: true } }),
			);
		});
	});

	it("renders Needs You with decision-specific copy per attention kind", async () => {
		getMock.mockImplementation((url: string) => {
			if (url === "/api/v1/outcomes/{outcomeId}/plans/{planId}/schedule") return Promise.resolve({ data: scheduleEnvelope(), error: undefined });
			if (url === "/api/v1/outcomes/{outcomeId}/plans/{planId}/mission") return Promise.resolve({ data: missionEnvelope(), error: undefined });
			if (url === "/api/v1/outcomes/{outcomeId}/plan") {
				return Promise.resolve({ data: planEnvelope("approved"), error: undefined });
			}
			if (url === "/api/v1/outcomes/{outcomeId}/attempts") {
				return Promise.resolve({
					data: {
						attempts: [
							attemptEnvelope({ status: "running", phase: "needs_input", attention: "blocked" }).attempt,
						],
					},
					error: undefined,
				});
			}
			return Promise.resolve({ data: undefined, error: { code: "NOT_FOUND", message: url } });
		});
		renderSurface();
		const blocked = await screen.findByTestId("outcome-run-needs-input");
		expect(blocked.textContent).toMatch(/Approval required/);
		expect(blocked.textContent).toMatch(/permission|approval|dialog/i);
		expect(blocked.textContent).not.toMatch(/Liveness unproven/);
		expect(screen.queryByTestId("outcome-run-waiting")).toBeNull();

		getMock.mockImplementation((url: string) => {
			if (url === "/api/v1/outcomes/{outcomeId}/plans/{planId}/schedule") return Promise.resolve({ data: scheduleEnvelope(), error: undefined });
			if (url === "/api/v1/outcomes/{outcomeId}/plans/{planId}/mission") return Promise.resolve({ data: missionEnvelope(), error: undefined });
			if (url === "/api/v1/outcomes/{outcomeId}/plan") {
				return Promise.resolve({ data: planEnvelope("approved"), error: undefined });
			}
			if (url === "/api/v1/outcomes/{outcomeId}/attempts") {
				return Promise.resolve({
					data: {
						attempts: [
							attemptEnvelope({ status: "running", phase: "needs_input", attention: "waiting_input" }).attempt,
						],
					},
					error: undefined,
				});
			}
			return Promise.resolve({ data: undefined, error: { code: "NOT_FOUND", message: url } });
		});
		const queryClient2 = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
		const view2 = render(
			<QueryClientProvider client={queryClient2}>
				<OutcomeRunSurface outcomeId="out-2" />
			</QueryClientProvider>,
		);
		await within(view2.container).findByTestId("outcome-run-needs-input");
		expect(within(view2.container).getByTestId("outcome-run-needs-input").textContent).toMatch(/asked you something/);
		expect(within(view2.container).getByTestId("outcome-run-needs-input").textContent).toMatch(/asked for input|question/i);
	});

	it("gives cancelled attempts a reconcile/confirm custody path", async () => {
		getMock.mockImplementation((url: string) => {
			if (url === "/api/v1/outcomes/{outcomeId}/plans/{planId}/schedule") return Promise.resolve({ data: scheduleEnvelope(), error: undefined });
			if (url === "/api/v1/outcomes/{outcomeId}/plans/{planId}/mission") return Promise.resolve({ data: missionEnvelope(), error: undefined });
			if (url === "/api/v1/outcomes/{outcomeId}/plan") {
				return Promise.resolve({ data: planEnvelope("approved"), error: undefined });
			}
			if (url === "/api/v1/outcomes/{outcomeId}/attempts") {
				return Promise.resolve({
					data: {
						attempts: [
							attemptEnvelope({ status: "cancelled", phase: "halted_cancelled" }).attempt,
						],
					},
					error: undefined,
				});
			}
			return Promise.resolve({ data: undefined, error: { code: "NOT_FOUND", message: url } });
		});
		renderSurface();
		expect(await screen.findByTestId("outcome-run-replace-confirm")).toBeDefined();
	});
	it("surfaces the daemon's refusal when admission fails closed instead of spinning", async () => {
		getMock.mockImplementation((url: string) => {
 if(url.endsWith("/run")) return Promise.resolve({data:{runState:{outcomeId:"out-1", projectId:"p", state:"needs_you", freshness:{contractRevisionNumber:1, planRevisionId:"plan-1", proofGeneration:0}, eligibleActions:[{action:"start",available:true}]}}});
			if (url === "/api/v1/outcomes/{outcomeId}/plans/{planId}/schedule") return Promise.resolve({ data: scheduleEnvelope(), error: undefined });
			if (url === "/api/v1/outcomes/{outcomeId}/plans/{planId}/mission") return Promise.resolve({ data: missionEnvelope(), error: undefined });
			if (url === "/api/v1/outcomes/{outcomeId}/plan") {
				return Promise.resolve({ data: planEnvelope("approved"), error: undefined });
			}
			if (url === "/api/v1/outcomes/{outcomeId}/attempts") {
				return Promise.resolve({ data: { attempts: [] }, error: undefined });
			}
			return Promise.resolve({ data: undefined, error: { code: "NOT_FOUND", message: url } });
		});
		postMock.mockResolvedValue({
			data: undefined,
			error: { code: "ATTEMPT_FENCE_HELD", message: "Another attempt holds custody", details: {} },
		});
		renderSurface();
		const user = userEvent.setup();
		await user.click(await screen.findByTestId("outcome-run-start"));
		const failure = await screen.findByRole("alert");
		expect(failure.textContent).toContain("custody");
	});
});

describe("Mission List/Graph switch", () => {
	function mockApprovedRun() {
		getMock.mockImplementation((url: string) => {
			if (url === "/api/v1/outcomes/{outcomeId}/plans/{planId}/schedule") return Promise.resolve({ data: scheduleEnvelope(), error: undefined });
			if (url === "/api/v1/outcomes/{outcomeId}/plans/{planId}/mission") return Promise.resolve({ data: missionEnvelope(), error: undefined });
			if (url === "/api/v1/outcomes/{outcomeId}/plan") return Promise.resolve({ data: planEnvelope("approved"), error: undefined });
			if (url === "/api/v1/outcomes/{outcomeId}/attempts") return Promise.resolve({ data: { attempts: [] }, error: undefined });
			return Promise.resolve({ data: undefined, error: { code: "NOT_FOUND", message: url } });
		});
	}

	it("switches the Mission reading between List and Graph and remembers the choice", async () => {
		mockApprovedRun();
		renderSurface();

		// List is the default reading; nothing is stored until a person chooses.
		await screen.findByTestId("mission-work-unit-list");
		expect(screen.getByTestId("mission-view-list")).toHaveAttribute("aria-selected", "true");
		expect(window.localStorage.getItem("kennel.mission.viewMode")).toBeNull();

		fireEvent.click(screen.getByTestId("mission-view-graph"));
		expect(await screen.findByTestId("mission-canvas")).toBeInTheDocument();
		expect(screen.queryByTestId("mission-work-unit-list")).not.toBeInTheDocument();
		expect(window.localStorage.getItem("kennel.mission.viewMode")).toBe("graph");
		expect(screen.getByTestId("mission-view-graph")).toHaveAttribute("aria-selected", "true");

		// Switching back restores the complete List reading.
		fireEvent.click(screen.getByTestId("mission-view-list"));
		expect(await screen.findByTestId("mission-work-unit-list")).toBeInTheDocument();
		expect(screen.queryByTestId("mission-canvas")).not.toBeInTheDocument();
		expect(window.localStorage.getItem("kennel.mission.viewMode")).toBe("list");
	});

	it("opens on the remembered Graph reading when a preference is stored", async () => {
		window.localStorage.setItem("kennel.mission.viewMode", "graph");
		useUiStore.setState({ missionViewMode: "graph" });
		mockApprovedRun();
		renderSurface();

		expect(await screen.findByTestId("mission-canvas")).toBeInTheDocument();
		expect(screen.queryByTestId("mission-work-unit-list")).not.toBeInTheDocument();
		expect(screen.getByTestId("mission-view-graph")).toHaveAttribute("aria-selected", "true");
	});
});
