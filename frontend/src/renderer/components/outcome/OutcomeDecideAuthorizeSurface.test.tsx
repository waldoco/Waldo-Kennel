import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";

// Drives the Decide & Authorize stage against a mocked HTTP client only.
//
// Locked contract under test: the surface renders only daemon answers (no
// optimistic plan state), proposing is replayable, approval carries the
// revision the approver was looking at, and both stale-contract and narrowed
// authority refusals render as their own states instead of generic errors.
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

import { OutcomeDecideAuthorizeSurface } from "./OutcomeDecideAuthorizeSurface";

function outcomeEnvelope(currentRevision = 1) {
	return {
		outcome: {
			id: "out-1",
			spaceId: "space-1",
			title: "Local Focus Ledger",
			currentRevisionNumber: currentRevision,
			currentRevision: {
				id: `cr-${currentRevision}`,
				outcomeId: "out-1",
				number: currentRevision,
				goal: "Record focus locally.",
				successCriteria: ["Blocks"],
				review: "checks",
				constraints: [],
				nonGoals: [],
				createdAt: "2026-08-23T09:00:00Z",
			},
			history: [],
			createdAt: "2026-08-23T09:00:00Z",
			updatedAt: "2026-08-23T09:00:00Z",
		},
	};
}

function planEnvelope(overrides: Record<string, unknown> = {}) {
	return {
		plan: {
			id: "plan-1",
			outcomeId: "out-1",
			number: 1,
			contractRevisionNumber: 1,
			status: "proposed",
			summary: "One direct Work Unit executing this contract locally.",
			workUnits: [
				{
					id: "wu-1",
					kind: "direct",
					title: 'Deliver "Local Focus Ledger"',
					contractRevisionNumber: 1,
					outputSummary: "The finished result inside the isolated worktree.",
					evidenceChecks: ["Positive minutes create one block."],
					verificationRequirement: "Deterministic checks.",
					stopConditions: ["Stop before an unapproved dependency"],
				},
			],
			grants: [
				{ id: "cg-read", name: "worktree.read", scope: "worktree/*" },
				{ id: "cg-write", name: "worktree.write", scope: "worktree/*" },
				{ id: "cg-exec", name: "worktree.exec", scope: "worktree/*" },
			],
			runBriefCoreDigest: "a".repeat(64),
			createdAt: "2026-08-23T09:30:00Z",
			...overrides,
		},
	};
}

function renderSurface(props: { onReviewWork?: () => void } = {}) {
	const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
	return render(
		<QueryClientProvider client={queryClient}>
			<OutcomeDecideAuthorizeSurface outcomeId="out-1" {...props} />
		</QueryClientProvider>,
	);
}

beforeEach(() => {
	vi.clearAllMocks();
	getMock.mockImplementation(async (url: string) => {
		if (url === "/api/v1/outcomes/{outcomeId}") return { data: outcomeEnvelope(1), error: undefined };
		if (url === "/api/v1/outcomes/{outcomeId}/plan") {
			return { data: undefined, error: { code: "PLAN_NOT_FOUND", message: "no plan yet" } };
		}
		return { data: undefined, error: undefined };
	});
});

describe("OutcomeDecideAuthorizeSurface", () => {
	it("uses the sole admitted planning provider and defaults to repository read scope", async () => {
		getMock.mockImplementation(async (url: string) => {
			if (url === "/api/v1/outcomes/{outcomeId}") return { data: outcomeEnvelope(1), error: undefined };
			if (url === "/api/v1/outcomes/{outcomeId}/plan") return { data: undefined, error: { code: "PLAN_NOT_FOUND", message: "no plan yet" } };
			if (url === "/api/v1/outcomes/{outcomeId}/planning-candidates") {
				return { data: { candidates: [{ id: "candidate-1", ready: true, binding: { mode: "direct_api", provider: "openai", modelSelection: "explicit", model: "gpt-5.6" } }] }, error: undefined };
			}
			if (url === "/api/v1/outcomes/{outcomeId}/planning-session") return { data: undefined, error: { code: "PLANNING_SESSION_NOT_FOUND", message: "no planning session yet" } };
			return { data: undefined, error: undefined };
		});
		postMock.mockResolvedValue({
			data: {
				planning: {
					session: { id: "planning-1", outcomeId: "out-1", contractRevisionId: "cr-1", contractRevisionNumber: 1, revision: 1, status: "active", waitingOn: "owner", contextMode: "repository_read", contextDigest: "ctx", planningGrantDigest: "grant", binding: { mode: "direct_api", provider: "openai", modelSelection: "explicit", model: "gpt-5.6" }, createdAt: "2026-09-11T00:00:00Z", updatedAt: "2026-09-11T00:00:00Z" },
				turns: [{ id: "turn-1", sequence: 1, role: "planner", kind: "clarification", text: "What should the first slice prove?", createdAt: "2026-09-11T00:00:00Z", clarification: { question: "What should the first slice prove?", reason: "Keep the plan bounded.", recommendation: "Use one vertical slice.", alternatives: ["One vertical slice"] } }],
				},
			},
			error: undefined,
		});
		renderSurface();

		expect(await screen.findByRole("radio", { name: /openai/i })).toBeChecked();
		expect(screen.getByTestId("planning-start")).toBeEnabled();
		await userEvent.click(screen.getByTestId("planning-start"));

		const [url, init] = postMock.mock.calls[0];
		expect(url).toBe("/api/v1/outcomes/{outcomeId}/planning-sessions");
		expect(init.body).toEqual({ expectedContractRevision: 1, candidateId: "candidate-1", contextMode: "repository_read", requestKey: expect.any(String) });
		expect(await screen.findByTestId("planning-turns")).toHaveTextContent("What should the first slice prove?");
	});

	it("keeps planning proposal separate from owner Plan approval", async () => {
		getMock.mockImplementation(async (url: string) => {
			if (url === "/api/v1/outcomes/{outcomeId}") return { data: outcomeEnvelope(1), error: undefined };
			if (url === "/api/v1/outcomes/{outcomeId}/plan") return { data: planEnvelope(), error: undefined };
			if (url === "/api/v1/outcomes/{outcomeId}/planning-session") return { data: undefined, error: { code: "PLANNING_SESSION_NOT_FOUND", message: "none" } };
			return { data: undefined, error: undefined };
		});
		renderSurface();
		const card = await screen.findByTestId("outcome-plan-card");
		expect(card).toHaveTextContent(/proposed/i);
		expect(screen.getByTestId("outcome-approve-plan")).toBeInTheDocument();
		expect(screen.queryByText(/start execution/i)).not.toBeInTheDocument();
	});

	it("keeps the readable Plan and dependency graph as separate views", async () => {
		getMock.mockImplementation(async (url: string) => {
			if (url === "/api/v1/outcomes/{outcomeId}") return { data: outcomeEnvelope(1), error: undefined };
			if (url === "/api/v1/outcomes/{outcomeId}/plan") return { data: planEnvelope(), error: undefined };
			return { data: undefined, error: undefined };
		});
		renderSurface();

		expect(await screen.findByTestId("outcome-plan-details")).toBeInTheDocument();
		expect(screen.queryByTestId("outcome-plan-graph")).not.toBeInTheDocument();

		await userEvent.click(screen.getByRole("button", { name: "Graph" }));
		expect(screen.getByTestId("outcome-plan-graph")).toBeInTheDocument();
		expect(screen.getByTestId("mission-work-unit-graph")).toBeInTheDocument();
		expect(screen.queryByTestId("outcome-plan-details")).not.toBeInTheDocument();
		expect(screen.queryByTestId("outcome-plan-work-units")).not.toBeInTheDocument();

		await userEvent.click(screen.getByRole("button", { name: "Plan" }));
		expect(screen.getByTestId("outcome-plan-details")).toBeInTheDocument();
		expect(screen.queryByTestId("outcome-plan-graph")).not.toBeInTheDocument();
	});

	it("approves with the revision the approver was looking at", async () => {
		getMock.mockImplementation(async (url: string) => {
			if (url === "/api/v1/outcomes/{outcomeId}") return { data: outcomeEnvelope(1), error: undefined };
			if (url === "/api/v1/outcomes/{outcomeId}/plan") return { data: planEnvelope(), error: undefined };
			return { data: undefined, error: undefined };
		});
		postMock.mockResolvedValue({
			data: planEnvelope({ status: "approved" }),
			error: undefined,
		});
		renderSurface();

		await userEvent.click(await screen.findByTestId("outcome-approve-plan"));

		const [url, init] = postMock.mock.calls[0];
		expect(url).toBe("/api/v1/outcomes/{outcomeId}/plans/{planId}/approval");
		expect(init.params.path).toEqual({ outcomeId: "out-1", planId: "plan-1" });
		expect(init.body).toEqual({ expectedContractRevision: 1 });
		await waitFor(() =>
			expect(screen.getByTestId("outcome-plan-card")).toHaveTextContent(/authorized/i),
		);
		expect(screen.queryByTestId("outcome-approve-plan")).not.toBeInTheDocument();
	});

	it("authorizes as a smooth in-place refresh — same plan card node, no reload or remount flash", async () => {
		getMock.mockImplementation(async (url: string) => {
			if (url === "/api/v1/outcomes/{outcomeId}") return { data: outcomeEnvelope(1), error: undefined };
			if (url === "/api/v1/outcomes/{outcomeId}/plan") return { data: planEnvelope(), error: undefined };
			return { data: undefined, error: undefined };
		});
		postMock.mockResolvedValue({
			data: planEnvelope({ status: "approved" }),
			error: undefined,
		});
		renderSurface();

		const planCardBeforeApproval = await screen.findByTestId("outcome-plan-card");
		expect(screen.getByTestId("outcome-plan-card").textContent).toMatch(/proposed/i);
		expect(screen.queryByTestId("outcome-review-work")).not.toBeInTheDocument();

		await userEvent.click(screen.getByTestId("outcome-approve-plan"));

		await waitFor(() => expect(screen.getByTestId("outcome-plan-card").textContent).toMatch(/authorized/i));
		// approve() resolves onSuccess by writing straight into the query cache
		// (queryClient.setQueryData in useApproveOutcomePlan) rather than
		// invalidating and refetching, so the same PlanReviewCard element updates
		// in place — never torn down and rebuilt, and never a moment with no
		// plan card at all while a refetch is in flight.
		expect(screen.getByTestId("outcome-plan-card")).toBe(planCardBeforeApproval);
		expect(screen.queryByTestId("outcome-approve-plan")).not.toBeInTheDocument();
		expect(screen.queryByTestId("outcome-plan-update")).not.toBeInTheDocument();
	});

	it("renders a typed stale conflict instead of transferring authority silently", async () => {
		getMock.mockImplementation(async (url: string) => {
			if (url === "/api/v1/outcomes/{outcomeId}") return { data: outcomeEnvelope(2), error: undefined };
			if (url === "/api/v1/outcomes/{outcomeId}/plan") return { data: planEnvelope(), error: undefined };
			return { data: undefined, error: undefined };
		});
		postMock.mockResolvedValue({
			data: undefined,
			error: {
				code: "PLAN_CONTRACT_STALE",
				message: "Plan binds contract revision 1; the Outcome is at 2",
				details: { planRevisionBinding: 1, currentRevision: 2 },
			},
		});
		renderSurface();

		expect(screen.queryByTestId("outcome-approve-plan")).not.toBeInTheDocument();
		expect(postMock).not.toHaveBeenCalled();

		expect(await screen.findByTestId("outcome-plan-conflict")).toBeInTheDocument();
		expect(screen.getByTestId("outcome-plan-reload")).toBeInTheDocument();
	});

	it("shows authority narrowing as blocked without offering a blind retry", async () => {
		getMock.mockImplementation(async (url: string) => {
			if (url === "/api/v1/outcomes/{outcomeId}") return { data: outcomeEnvelope(1), error: undefined };
			if (url === "/api/v1/outcomes/{outcomeId}/plan") return { data: planEnvelope(), error: undefined };
			return { data: undefined, error: undefined };
		});
		postMock.mockResolvedValue({
			data: undefined,
			error: {
				code: "PLAN_CAPABILITY_UNAUTHORIZED",
				message: `capability "${"worktree.exec"}" is not authorized by every authority layer`,
			},
		});
		renderSurface();

		await userEvent.click(await screen.findByTestId("outcome-approve-plan"));

		expect(await screen.findByTestId("outcome-authority-blocked")).toBeInTheDocument();
		expect(screen.queryByRole("button", { name: /retry/i })).not.toBeInTheDocument();
	});


	it("renders the stacked review cards in the contract anatomy order with expand/collapse", async () => {
		getMock.mockImplementation(async (url: string) => {
			if (url === "/api/v1/outcomes/{outcomeId}") return { data: outcomeEnvelope(1), error: undefined };
			if (url === "/api/v1/outcomes/{outcomeId}/plan") return { data: planEnvelope(), error: undefined };
			return { data: undefined, error: undefined };
		});
		renderSurface();

		const details = await screen.findByTestId("outcome-plan-details");
		const headings = ["Summary", "Desired state", "Evidence", "Verification through", "Pause Trigger", "Agent Permissions", "Run Brief Digest"];
		const rendered = headings.map((label) => screen.getByRole("button", { name: new RegExp(label) }));
		for (let i = 0; i < rendered.length - 1; i += 1) {
			expect(rendered[i].compareDocumentPosition(rendered[i + 1]) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
		}
		// Every card starts expanded and still toggles closed.
		const evidence = screen.getByRole("button", { name: /Evidence/ });
		expect(evidence).toHaveAttribute("aria-expanded", "true");
		await userEvent.click(evidence);
		expect(evidence).toHaveAttribute("aria-expanded", "false");
	});

	it("keeps Update on the revise path and Authorize on the approve mutation", async () => {
		const onReviewContract = vi.fn();
		getMock.mockImplementation(async (url: string) => {
			if (url === "/api/v1/outcomes/{outcomeId}") return { data: outcomeEnvelope(1), error: undefined };
			if (url === "/api/v1/outcomes/{outcomeId}/plan") return { data: planEnvelope(), error: undefined };
			return { data: undefined, error: undefined };
		});
		postMock.mockResolvedValue({ data: planEnvelope({ status: "approved" }), error: undefined });
		const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
		render(
			<QueryClientProvider client={queryClient}>
				<OutcomeDecideAuthorizeSurface onReviewContract={onReviewContract} outcomeId="out-1" />
			</QueryClientProvider>,
		);

		await userEvent.click(await screen.findByTestId("outcome-plan-update"));
		expect(onReviewContract).toHaveBeenCalledOnce();
		expect(postMock).not.toHaveBeenCalled();

		await userEvent.click(screen.getByTestId("outcome-approve-plan"));
		const [url, init] = postMock.mock.calls[0];
		expect(url).toBe("/api/v1/outcomes/{outcomeId}/plans/{planId}/approval");
		expect(init.body).toEqual({ expectedContractRevision: 1 });
	});

	it("reports missing grants honestly instead of fabricating permissions", async () => {
		getMock.mockImplementation(async (url: string) => {
			if (url === "/api/v1/outcomes/{outcomeId}") return { data: outcomeEnvelope(1), error: undefined };
			if (url === "/api/v1/outcomes/{outcomeId}/plan") return { data: planEnvelope({ grants: [] }), error: undefined };
			return { data: undefined, error: undefined };
		});
		renderSurface();

		expect(await screen.findByTestId("outcome-plan-grants-empty")).toHaveTextContent("Not reported");
		expect(screen.queryByText("worktree.read")).not.toBeInTheDocument();
	});

	it("shows an already-authorized plan without proposal or approval controls", async () => {
		const onReviewWork = vi.fn();
		getMock.mockImplementation(async (url: string) => {
			if (url === "/api/v1/outcomes/{outcomeId}") return { data: outcomeEnvelope(1), error: undefined };
			if (url === "/api/v1/outcomes/{outcomeId}/plan") {
				return { data: planEnvelope({ status: "approved" }), error: undefined };
			}
			return { data: undefined, error: undefined };
		});
		renderSurface({ onReviewWork });

		const card = await screen.findByTestId("outcome-plan-card");
		expect(card).toHaveTextContent(/authorized/i);
		expect(screen.queryByTestId("outcome-propose-plan")).not.toBeInTheDocument();
		expect(screen.queryByTestId("outcome-approve-plan")).not.toBeInTheDocument();
		await userEvent.click(screen.getByTestId("outcome-review-work"));
		expect(onReviewWork).toHaveBeenCalledOnce();
	});
});
