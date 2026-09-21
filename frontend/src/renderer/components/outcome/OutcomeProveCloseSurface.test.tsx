import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";

const { getMock, postMock } = vi.hoisted(() => ({ getMock: vi.fn(), postMock: vi.fn() }));

vi.mock("../../lib/api-client", () => ({
	apiClient: { GET: getMock, POST: postMock },
	apiErrorCode: (error: unknown) =>
		typeof error === "object" && error !== null && "code" in error ? String((error as { code: unknown }).code) : undefined,
	apiErrorMessage: (error: unknown) =>
		typeof error === "object" && error !== null && "message" in error ? String((error as { message: unknown }).message) : "Request failed",
}));

import { OutcomeProveCloseSurface } from "./OutcomeProveCloseSurface";

function proofEnvelope(status = "ready_for_acceptance"): { proof: import("../../../api/schema").components["schemas"]["OutcomeProofResponse"] } {
	return {
		proof: {
			outcomeId: "out-1",
			contractRevision: {
				id: "cr-1", number: 1, goal: "Record focus.",
				criteria: [{ criterionId: "crit-1", contractRevisionId: "cr-1", position: 1, text: "One block survives restart." }],
				successCriteria: ["One block survives restart."], review: "Owner walkthrough.", constraints: [], nonGoals: [], createdAt: "2026-08-25T10:00:00Z",
			},
			status,
			nextAction: status === "accepted" ? "Accepted. Reopen explicitly if needed." : "Review the current proof and explicitly accept or request rework.",
			result: {
				artifacts: [{ revision: "artifact-v2", sourceRef: "README.md", digest: "b".repeat(64) }],
				checks: [{ criterionId: "crit-1", command: "grep -Fx review README.md", artifactRevision: "artifact-v2", verdict: "failed", detail: "check exited 1", uncertain: false }],
				changes: [{
					attemptId: "attempt-2", workUnitId: "wu-1", artifactVersion: "artifact-v2", retentionState: "retained", truncated: false,
					files: [
						{ path: "README.md", changeKind: "modified", digest: "b".repeat(64) },
						{ path: "notes/todo.md", changeKind: "added", digest: "c".repeat(64) },
					],
				}],
				uncertainty: [], nextSafeAction: "Review the changed artifact and request rework if needed.",
			},
			reentryTargets: [
				{ targetType: "contract", targetId: "cr-1", label: "Contract revision 1" },
				{ targetType: "plan", targetId: "plan-1", label: "Current plan" },
				{ targetType: "work_unit", targetId: "wu-1", label: "Record one block" },
				{ targetType: "attempt", targetId: "attempt-2", label: "Record one block (succeeded)" },
			],
			criteria: [{
				criterionId: "crit-1", contractRevisionId: "cr-1", position: 1, text: "One block survives restart.", ready: true,
				evidence: [{
					id: "ev-1", contractRevisionId: "cr-1", criterionId: "crit-1", subjectType: "outcome", subjectId: "out-1", subjectRevision: "cr-1",
					kind: "supporting", sourceType: "owner_walkthrough", sourceRef: "restart-walkthrough", producerType: "user", producerRef: "owner",
					summary: "The block remained after restart.", contentDigest: "a".repeat(64), createdAt: "2026-08-25T10:01:00Z",
				}],
				verifications: [{
					id: "ver-1", contractRevisionId: "cr-1", criterionId: "crit-1", subjectType: "outcome", subjectId: "out-1", subjectRevision: "cr-1",
					evidenceItemIds: ["ev-1"], method: "Owner restart walkthrough.", independenceClass: "owner_walkthrough", independent: true,
					result: "passed", verifierRef: "owner", createdAt: "2026-08-25T10:02:00Z",
				}],
			}],
			decisions: status === "accepted" ? [{ id: "acc-1", contractRevisionId: "cr-1", kind: "accept", actorType: "user", summary: "Accepted.", resourceDisposition: "retain", createdAt: "2026-08-25T10:03:00Z" }] : [],
			corrections: [],
		},
	};
}

function renderSurface() {
	const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
	return render(<QueryClientProvider client={queryClient}><OutcomeProveCloseSurface outcomeId="out-1" /></QueryClientProvider>);
}

beforeEach(() => {
	getMock.mockReset();
	postMock.mockReset();
	getMock.mockResolvedValue({ data: proofEnvelope(), error: undefined });
	postMock.mockResolvedValue({ data: proofEnvelope(), error: undefined });
});

describe("OutcomeProveCloseSurface", () => {
	it("renders exact criterion proof and sends only explicit user acceptance", async () => {
		const user = userEvent.setup();
		renderSurface();
		expect(await screen.findByText("One block survives restart.")).toBeDefined();
		expect(screen.getByTestId("proof-evidence-ev-1").textContent).toContain("restart-walkthrough");
		expect(screen.getByTestId("proof-verification-ver-1").textContent).toMatch(/owner walkthrough/i);
		expect(screen.getByTestId("outcome-result-artifacts").textContent).toContain("README.md");
		expect(screen.getByTestId("outcome-result-checks").textContent).toContain("grep -Fx review README.md");

		await user.type(screen.getByTestId("proof-decision-summary"), "I reviewed the current criterion.");
		await user.click(screen.getByTestId("proof-accept"));
		await waitFor(() => expect(postMock).toHaveBeenCalledWith(
			"/api/v1/outcomes/{outcomeId}/acceptance-decisions",
			expect.objectContaining({
				params: { path: { outcomeId: "out-1" } },
				body: expect.objectContaining({
					expectedContractRevision: 1, contractRevisionId: "cr-1", kind: "accept",
					resourceDisposition: "retain", requestKey: expect.any(String),
				}),
			}),
		));
	});

	it("renders repeated criterion gaps without duplicate React keys", async () => {
		const warn = vi.spyOn(console, "error").mockImplementation(() => undefined);
		const envelope = proofEnvelope("active");
		envelope.proof.criteria = [
			{ ...envelope.proof.criteria[0], criterionId: "crit-1", ready: false, gap: "Add supporting Evidence for this criterion.", evidence: [], verifications: [] },
			{ ...envelope.proof.criteria[0], criterionId: "crit-2", ready: false, gap: "Add supporting Evidence for this criterion.", evidence: [], verifications: [] },
		];
		getMock.mockResolvedValue({ data: envelope, error: undefined });

		renderSurface();
		expect(await screen.findAllByText("Add supporting Evidence for this criterion.")).toHaveLength(4);
		expect(warn.mock.calls.flat().join(" ")).not.toContain("same key");
	});

	it("binds new Evidence and Verification to stable criterion identity", async () => {
		const user = userEvent.setup();
		renderSurface();
		await screen.findByText("One block survives restart.");

		await user.type(screen.getByTestId("proof-evidence-summary-crit-1"), "A second restart retained it.");
		await user.type(screen.getByTestId("proof-evidence-source-crit-1"), "restart-walkthrough-2");
		await user.click(screen.getByTestId("proof-add-evidence-crit-1"));
		await waitFor(() => expect(postMock).toHaveBeenCalledWith(
			"/api/v1/outcomes/{outcomeId}/evidence",
			expect.objectContaining({ body: expect.objectContaining({
				contractRevisionId: "cr-1", criterionId: "crit-1", subjectId: "out-1", subjectRevision: "cr-1", kind: "supporting",
			}) }),
		));

		await user.type(screen.getByTestId("proof-verification-method-crit-1"), "Fresh owner walkthrough.");
		await user.click(screen.getByTestId("proof-add-verification-crit-1"));
		await waitFor(() => expect(postMock).toHaveBeenCalledWith(
			"/api/v1/outcomes/{outcomeId}/verifications",
			expect.objectContaining({ body: expect.objectContaining({
				contractRevisionId: "cr-1", criterionId: "crit-1", evidenceItemIds: ["ev-1"], independenceClass: "owner_walkthrough",
			}) }),
		));
	});

	it("records declared evidence provenance and separate-session verification identities", async () => {
		const user = userEvent.setup();
		renderSurface();
		await screen.findByText("One block survives restart.");

		await user.type(screen.getByTestId("proof-evidence-summary-crit-1"), "The provider emitted an artifact.");
		await user.type(screen.getByTestId("proof-evidence-source-crit-1"), "artifact://run-42");
		await user.selectOptions(screen.getByLabelText("Producer type"), "provider");
		const producerIdentity = screen.getByLabelText("Producer identity");
		await user.clear(producerIdentity);
		await user.type(producerIdentity, "session-producer");
		await user.click(screen.getByTestId("proof-add-evidence-crit-1"));
		await waitFor(() => expect(postMock).toHaveBeenCalledWith(
			"/api/v1/outcomes/{outcomeId}/evidence",
			expect.objectContaining({ body: expect.objectContaining({
				producerType: "provider", producerRef: "session-producer",
			}) }),
		));

		await user.type(screen.getByTestId("proof-verification-method-crit-1"), "A fresh session replayed the check.");
		await user.selectOptions(screen.getByLabelText("Independence class"), "separate_session");
		await user.type(screen.getByPlaceholderText("Producer session or identity"), "session-producer");
		await user.click(screen.getByTestId("proof-add-verification-crit-1"));
		await waitFor(() => expect(postMock).toHaveBeenCalledWith(
			"/api/v1/outcomes/{outcomeId}/verifications",
			expect.objectContaining({ body: expect.objectContaining({
				independenceClass: "separate_session", producerRef: "session-producer", verifierRef: "owner",
			}) }),
		));
	});

	it("offers Reopen only after acceptance and records explicit correction lineage", async () => {
		const user = userEvent.setup();
		getMock.mockResolvedValue({ data: proofEnvelope("accepted"), error: undefined });
		postMock.mockResolvedValue({ data: proofEnvelope("accepted"), error: undefined });
		renderSurface();
		expect(await screen.findByTestId("proof-reopen")).toBeDefined();
		expect(screen.queryByTestId("proof-accept")).toBeNull();
		await user.type(screen.getByTestId("proof-decision-summary"), "The midnight boundary needs another run.");
		await user.click(screen.getByTestId("proof-reopen"));
		await waitFor(() => expect(postMock).toHaveBeenCalledWith(
			"/api/v1/outcomes/{outcomeId}/acceptance-decisions",
			expect.objectContaining({ body: expect.objectContaining({
				kind: "reopen", reentryTargetType: "contract", reentryTargetId: "cr-1",
			}) }),
		));
	});

	it("shows the measured change list and offers only daemon-derived re-entry targets", async () => {
		const user = userEvent.setup();
		renderSurface();
		const changes = await screen.findByTestId("outcome-result-changes");
		expect(changes.textContent).toContain("README.md");
		expect(changes.textContent).toContain("modified");
		expect(changes.textContent).toContain("notes/todo.md");
		expect(changes.textContent).toContain("added");

		await user.type(screen.getByTestId("proof-decision-summary"), "Replay the failed attempt.");
		await user.selectOptions(screen.getByLabelText("Re-enter at"), "attempt");
		const identitySelect = screen.getByLabelText("Target identity") as HTMLSelectElement;
		const options = Array.from(identitySelect.options).map((option) => option.textContent);
		expect(options).toContain("Record one block (succeeded)");
		expect(options.some((label) => label === "Current plan")).toBe(false);
	});

	it("requires the exact non-contract re-entry identity before reopening", async () => {
		const user = userEvent.setup();
		getMock.mockResolvedValue({ data: proofEnvelope("accepted"), error: undefined });
		postMock.mockResolvedValue({ data: proofEnvelope("accepted"), error: undefined });
		renderSurface();
		const reopen = await screen.findByTestId("proof-reopen") as HTMLButtonElement;
		await user.type(screen.getByTestId("proof-decision-summary"), "Replay the failed attempt.");
		await user.selectOptions(screen.getByLabelText("Re-enter at"), "attempt");
		expect(reopen.disabled).toBe(true);

		await user.selectOptions(screen.getByLabelText("Target identity"), "attempt-2");
		expect(reopen.disabled).toBe(false);
		await user.click(reopen);
		await waitFor(() => expect(postMock).toHaveBeenCalledWith(
			"/api/v1/outcomes/{outcomeId}/acceptance-decisions",
			expect.objectContaining({ body: expect.objectContaining({
				kind: "reopen", reentryTargetType: "attempt", reentryTargetId: "attempt-2",
			}) }),
		));
	});
});
