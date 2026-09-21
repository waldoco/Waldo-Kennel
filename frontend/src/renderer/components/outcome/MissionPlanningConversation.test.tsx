import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";

const { getMock, postMock } = vi.hoisted(() => ({ getMock: vi.fn(), postMock: vi.fn() }));

vi.mock("../../lib/api-client", () => ({
	apiClient: { GET: getMock, POST: postMock },
	apiErrorCode: (error: unknown) => (typeof error === "object" && error !== null && "code" in error ? String((error as { code: unknown }).code) : undefined),
	apiErrorMessage: (error: unknown) => (error instanceof Error ? error.message : typeof error === "object" && error !== null && "message" in error ? String((error as { message: unknown }).message) : "Request failed"),
}));

import { MissionPlanningConversation } from "./MissionPlanningConversation";

function renderConversation({ disabled = false }: { disabled?: boolean } = {}) {
	return render(
		<QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
			<MissionPlanningConversation contractRevision={3} disabled={disabled} onReviewContract={vi.fn()} outcomeId="out-1" />
		</QueryClientProvider>,
	);
}

const candidate = { id: "direct_api|openai|explicit|gpt-5.6", ready: true, binding: { mode: "direct_api", provider: "openai", modelSelection: "explicit", model: "gpt-5.6" } };

beforeEach(() => {
	getMock.mockReset();
	postMock.mockReset();
	getMock.mockImplementation(async (url: string) => {
		if (url.endsWith("/planning-candidates")) return { data: { candidates: [candidate] }, error: undefined };
		return { data: undefined, error: { code: "PLANNING_SESSION_NOT_FOUND", message: "none" } };
	});
});

describe("MissionPlanningConversation", () => {
	it("defaults to repository packet scope and selects the sole admitted candidate", async () => {
		postMock.mockResolvedValue({ data: { planning: { session: { id: "planning-1", outcomeId: "out-1", contractRevisionId: "cr-3", contractRevisionNumber: 3, revision: 1, status: "active", waitingOn: "owner", contextMode: "repository_read", contextDigest: "ctx", planningGrantDigest: "grant", binding: candidate.binding, createdAt: "2026-09-11T00:00:00Z", updatedAt: "2026-09-11T00:00:00Z" }, turns: [] } }, error: undefined });
		const user = userEvent.setup();
		renderConversation();
		const start = await screen.findByTestId("planning-start");
		await vi.waitFor(() => expect(start).toBeEnabled());
		expect(screen.getByRole("radio", { name: /openai/i })).toBeChecked();
		await user.click(start);
		expect(postMock).toHaveBeenCalledWith("/api/v1/outcomes/{outcomeId}/planning-sessions", expect.objectContaining({ body: { expectedContractRevision: 3, candidateId: candidate.id, contextMode: "repository_read", requestKey: expect.any(String) } }));
		expect(screen.getByText(/bounded repository context packet/i)).toBeInTheDocument();
	});

	it("marks Start planning disabled and explains a disconnected fact stream", async () => {
		renderConversation({ disabled: true });
		const start = await screen.findByTestId("planning-start");
		await vi.waitFor(() => expect(screen.getByRole("radio", { name: /openai/i })).toBeChecked());
		expect(start).toBeDisabled();
		expect(await screen.findByTestId("planning-start-disabled-reason")).toHaveTextContent(/updates disconnected/i);
	});

	it("explains why Start planning is disabled when no planning agent is ready", async () => {
		getMock.mockImplementation(async (url: string) => {
			if (url.endsWith("/planning-candidates")) return { data: { candidates: [] }, error: undefined };
			return { data: undefined, error: { code: "PLANNING_SESSION_NOT_FOUND", message: "none" } };
		});
		renderConversation();
		const start = await screen.findByTestId("planning-start");
		expect(start).toBeDisabled();
		expect(await screen.findByTestId("planning-start-disabled-reason")).toHaveTextContent(/no planning agent is available yet/i);
	});

	it("explains that an agent must be selected when several are ready", async () => {
		const second = { id: "native_harness|codex|provider_default", ready: true, binding: { mode: "native_harness", provider: "codex", modelSelection: "provider_default" } };
		getMock.mockImplementation(async (url: string) => {
			if (url.endsWith("/planning-candidates")) return { data: { candidates: [candidate, second] }, error: undefined };
			return { data: undefined, error: { code: "PLANNING_SESSION_NOT_FOUND", message: "none" } };
		});
		renderConversation();
		const start = await screen.findByTestId("planning-start");
		expect(await screen.findByTestId("planning-start-disabled-reason")).toHaveTextContent(/select an available planning agent/i);
		expect(start).toBeDisabled();
	});

	it("shows no disabled reason once the start gate clears", async () => {
		renderConversation();
		const start = await screen.findByTestId("planning-start");
		await vi.waitFor(() => expect(start).toBeEnabled());
		expect(screen.queryByTestId("planning-start-disabled-reason")).not.toBeInTheDocument();
	});

	it("describes native planning as bounded packet reasoning", async () => {
		const nativeCandidate = { id: "native_harness|codex|provider_default", ready: true, binding: { mode: "native_harness", provider: "codex", modelSelection: "provider_default" } };
		getMock.mockImplementation(async (url: string) => {
			if (url.endsWith("/planning-candidates")) return { data: { candidates: [nativeCandidate] }, error: undefined };
			return { data: undefined, error: { code: "PLANNING_SESSION_NOT_FOUND", message: "none" } };
		});
		renderConversation();
		await vi.waitFor(() => expect(screen.getByRole("radio", { name: /codex/i })).toBeChecked());
		const user = userEvent.setup();
		await user.click(screen.getByRole("button", { name: "Change" }));
		expect(screen.getByText(/bounded repository context packet/i)).toBeInTheDocument();
		expect(screen.queryByText(/local tools and Kennel skills/i)).not.toBeInTheDocument();
	});

	it("renders a clarification and Contract-change proposal without mutating the Contract", async () => {
		getMock.mockImplementation(async (url: string) => {
			if (url.endsWith("/planning-candidates")) return { data: { candidates: [] }, error: undefined };
			return {
				data: { planning: { session: { id: "planning-1", outcomeId: "out-1", contractRevisionId: "cr-3", contractRevisionNumber: 3, revision: 2, status: "active", waitingOn: "owner", contextMode: "repository_read", contextDigest: "ctx", planningGrantDigest: "grant", binding: candidate.binding, createdAt: "2026-09-11T00:00:00Z", updatedAt: "2026-09-11T00:00:00Z" }, turns: [
					{ id: "turn-1", sequence: 1, role: "planner", kind: "clarification", text: "internal planner text", createdAt: "2026-09-11T00:00:00Z", clarification: { question: "Which evidence should be checked first?", reason: "Keep scope bounded.", recommendation: "Use the existing check.", alternatives: ["Use the existing check"] } },
					{ id: "turn-2", sequence: 2, role: "planner", kind: "contract_change_proposal", text: "The review criterion may need clarification.", createdAt: "2026-09-11T00:00:00Z", contractChange: { summary: "Clarify the review criterion", changedFields: ["review"] } },
				] } }, error: undefined,
			};
		});
		renderConversation();
		expect(await screen.findByText("Which evidence should be checked first?")).toBeInTheDocument();
		expect(screen.getByTestId("planning-contract-change")).toHaveTextContent("Clarify the review criterion");
		expect(screen.getByRole("button", { name: "Review Contract" })).toBeInTheDocument();
	});

	it("keeps the draft and request key when sending fails", async () => {
		getMock.mockImplementation(async (url: string) => {
			if (url.endsWith("/planning-candidates")) return { data: { candidates: [] }, error: undefined };
			return { data: { planning: { session: { id: "planning-1", outcomeId: "out-1", contractRevisionId: "cr-3", contractRevisionNumber: 3, revision: 2, status: "active", waitingOn: "owner", contextMode: "repository_read", contextDigest: "ctx", planningGrantDigest: "grant", binding: candidate.binding, createdAt: "2026-09-11T00:00:00Z", updatedAt: "2026-09-11T00:00:00Z" }, turns: [] } }, error: undefined };
		});
		postMock.mockRejectedValue({ code: "PLANNING_PROVIDER_FAILED", message: "agent unavailable" });
		const user = userEvent.setup();
		renderConversation();
		const input = await screen.findByRole("textbox", { name: "Message the planning agent" });
		await user.type(input, "Keep the evidence check bounded");
		await user.click(screen.getByRole("button", { name: "Send" }));
		await screen.findByRole("alert");
		expect(input).toHaveValue("Keep the evidence check bounded");
		const firstCall = postMock.mock.calls.at(-1);
		await user.click(screen.getByRole("button", { name: "Send" }));
		const secondCall = postMock.mock.calls.at(-1);
		expect(firstCall?.[1]?.body.requestKey).toBe(secondCall?.[1]?.body.requestKey);
	});

	it("does not expose start while the current session is still loading", async () => {
		let resolveSession!: (value: unknown) => void;
		const sessionPending = new Promise((resolve) => { resolveSession = resolve; });
		getMock.mockImplementation((url: string) => {
			if (url.endsWith("/planning-candidates")) return Promise.resolve({ data: { candidates: [candidate] }, error: undefined });
			return sessionPending;
		});
		renderConversation();
		await screen.findByText("Loading the current planning session…");
		expect(screen.queryByTestId("planning-start")).not.toBeInTheDocument();
		resolveSession({ data: undefined, error: { code: "PLANNING_SESSION_NOT_FOUND", message: "none" } });
	});

	it("keeps cancellation available while the provider is in flight", async () => {
		getMock.mockImplementation(async (url: string) => {
			if (url.endsWith("/planning-candidates")) return { data: { candidates: [] }, error: undefined };
			return { data: { planning: { session: { id: "planning-1", outcomeId: "out-1", contractRevisionId: "cr-3", contractRevisionNumber: 3, revision: 2, status: "active", waitingOn: "provider", contextMode: "repository_read", contextDigest: "ctx", planningGrantDigest: "grant", binding: candidate.binding, createdAt: "2026-09-11T00:00:00Z", updatedAt: "2026-09-11T00:00:00Z" }, turns: [] } }, error: undefined };
		});
		postMock.mockResolvedValue({ data: { planning: { session: { id: "planning-1", outcomeId: "out-1", contractRevisionId: "cr-3", contractRevisionNumber: 3, revision: 3, status: "cancelled", waitingOn: "none", contextMode: "repository_read", contextDigest: "ctx", planningGrantDigest: "grant", binding: candidate.binding, createdAt: "2026-09-11T00:00:00Z", updatedAt: "2026-09-11T00:00:00Z" }, turns: [] } }, error: undefined });
		renderConversation();
		expect(await screen.findByRole("button", { name: "Prepare plan proposal" })).toBeDisabled();
		expect(screen.getByRole("button", { name: "Cancel" })).toBeEnabled();
	});

	it("renders a durable provider failure and offers a fresh session without hiding the old lineage", async () => {
		getMock.mockImplementation(async (url: string) => {
			if (url.endsWith("/planning-candidates")) return { data: { candidates: [candidate] }, error: undefined };
			return { data: { planning: { session: { id: "failed-session", outcomeId: "out-1", contractRevisionId: "cr-3", contractRevisionNumber: 3, revision: 3, status: "active", waitingOn: "owner", contextMode: "repository_read", contextDigest: "ctx", planningGrantDigest: "grant", binding: candidate.binding, lastFailureCode: "PLANNING_REPLY_AMBIGUOUS", lastFailureDetail: "The daemon restarted before the planning reply was recorded.", createdAt: "2026-09-11T00:00:00Z", updatedAt: "2026-09-11T00:00:00Z" }, turns: [] } }, error: undefined };
		});
		const user = userEvent.setup();
		renderConversation();
		expect(await screen.findByTestId("planning-provider-failure")).toHaveTextContent("The daemon restarted before the planning reply was recorded.");
		await user.click(screen.getByRole("button", { name: "Start another planning session" }));
		expect(screen.queryByTestId("planning-provider-failure")).not.toBeInTheDocument();
		expect(screen.getByTestId("planning-start")).toBeInTheDocument();
	});

	it("starts a fresh session when the cached session belongs to an older Contract revision", async () => {
		getMock.mockImplementation(async (url: string) => {
			if (url.endsWith("/planning-candidates")) return { data: { candidates: [candidate] }, error: undefined };
			return { data: { planning: { session: { id: "old-session", outcomeId: "out-1", contractRevisionId: "cr-2", contractRevisionNumber: 2, revision: 4, status: "superseded", waitingOn: "none", contextMode: "repository_read", contextDigest: "ctx", planningGrantDigest: "grant", binding: candidate.binding, createdAt: "2026-09-11T00:00:00Z", updatedAt: "2026-09-11T00:00:00Z" }, turns: [] } }, error: undefined };
		});
		postMock.mockResolvedValue({ data: { planning: { session: { id: "new-session", outcomeId: "out-1", contractRevisionId: "cr-3", contractRevisionNumber: 3, revision: 1, status: "active", waitingOn: "owner", contextMode: "repository_read", contextDigest: "ctx", planningGrantDigest: "grant", binding: candidate.binding, createdAt: "2026-09-11T00:00:00Z", updatedAt: "2026-09-11T00:00:00Z" }, turns: [] } }, error: undefined });
		const user = userEvent.setup();
		renderConversation();
		const start = await screen.findByTestId("planning-start");
		await user.click(screen.getByRole("radio", { name: /openai/i }));
		await user.click(start);
		expect(await screen.findByTestId("planning-session-status")).toHaveTextContent("Planning");
		expect(postMock).toHaveBeenCalledWith("/api/v1/outcomes/{outcomeId}/planning-sessions", expect.objectContaining({ body: expect.objectContaining({ expectedContractRevision: 3 }) }));
	});

	it("can start a new session after cancellation", async () => {
		getMock.mockImplementation(async (url: string) => {
			if (url.endsWith("/planning-candidates")) return { data: { candidates: [candidate] }, error: undefined };
			return { data: { planning: { session: { id: "active-session", outcomeId: "out-1", contractRevisionId: "cr-3", contractRevisionNumber: 3, revision: 1, status: "active", waitingOn: "owner", contextMode: "repository_read", contextDigest: "ctx", planningGrantDigest: "grant", binding: candidate.binding, createdAt: "2026-09-11T00:00:00Z", updatedAt: "2026-09-11T00:00:00Z" }, turns: [] } }, error: undefined };
		});
		postMock.mockImplementation(async (url: string) => {
			if (url.endsWith("/cancel")) return { data: { planning: { session: { id: "active-session", outcomeId: "out-1", contractRevisionId: "cr-3", contractRevisionNumber: 3, revision: 2, status: "cancelled", waitingOn: "none", contextMode: "repository_read", contextDigest: "ctx", planningGrantDigest: "grant", binding: candidate.binding, createdAt: "2026-09-11T00:00:00Z", updatedAt: "2026-09-11T00:00:00Z" }, turns: [] } }, error: undefined };
			return { data: { planning: { session: { id: "new-session", outcomeId: "out-1", contractRevisionId: "cr-3", contractRevisionNumber: 3, revision: 1, status: "active", waitingOn: "owner", contextMode: "repository_read", contextDigest: "ctx", planningGrantDigest: "grant", binding: candidate.binding, createdAt: "2026-09-11T00:00:00Z", updatedAt: "2026-09-11T00:00:00Z" }, turns: [] } }, error: undefined };
		});
		const user = userEvent.setup();
		renderConversation();
		await user.click(await screen.findByRole("button", { name: "Cancel" }));
		await user.click(await screen.findByRole("button", { name: "Start another planning session" }));
		await user.click(screen.getByRole("radio", { name: /openai/i }));
		await user.click(screen.getByTestId("planning-start"));
		expect(postMock.mock.calls.map(([url]) => url)).toEqual([
			"/api/v1/outcomes/{outcomeId}/planning-sessions/{planningSessionId}/cancel",
			"/api/v1/outcomes/{outcomeId}/planning-sessions",
		]);
	});
});
