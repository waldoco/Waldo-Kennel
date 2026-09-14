import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import type { AttemptRecord, PlanRecord } from "../../hooks/useOutcome";
import { AttemptCardAdapter, AttemptRowAdapter, newestAttemptSession, toAttemptBoardPresentation } from "./OutcomeRunBoardAdapters";

// The Ready lane (a succeeded attempt) should offer a real Merge action next
// to Engage/Instruct when its bound session has a real pull request — never
// a decorative or dead control. An attempt carries no PR of its own; only
// its bound WorkspaceSession does, so this cross-references the exact
// hooks/helpers the generic Sessions board already uses for the same job.
const { workspaceQueryMock, scmSummaryMock } = vi.hoisted(() => ({
	workspaceQueryMock: vi.fn(),
	scmSummaryMock: vi.fn(),
}));

vi.mock("../../hooks/useWorkspaceQuery", () => ({
	useWorkspaceQuery: workspaceQueryMock,
}));

vi.mock("../../hooks/useSessionScmSummary", () => ({
	useSessionScmSummary: scmSummaryMock,
}));

function attempt(overrides: Partial<AttemptRecord> = {}): AttemptRecord {
	return {
		id: "att-1",
		outcomeId: "out-1",
		planRevisionId: "plan-1",
		workUnitId: "wu-1",
		number: 1,
		status: "succeeded",
		contractRevisionNumber: 1,
		sessions: [{ id: "ref-1", seq: 1, sessionId: "sess-1", harness: "codex", runBriefCoreDigest: "a".repeat(64), boundAt: "2026-08-29T00:00:00Z" }],
		observations: [],
		receipts: [],
		presentation: { phase: "succeeded", unconfirmed: false, endedUnclassified: false, nextAction: "Review the result" },
		createdAt: "2026-08-29T00:00:00Z",
		updatedAt: "2026-08-29T00:00:00Z",
		...overrides,
	} as AttemptRecord;
}

const plan = undefined as PlanRecord | undefined;

describe("OutcomeRunBoardAdapters — Ready lane Merge action", () => {
	it("uses the newest bound session for board attribution", () => {
		const current = attempt({
			sessions: [
				{ id: "ref-old", seq: 1, sessionId: "sess-old", harness: "codex", runBriefCoreDigest: "a".repeat(64), boundAt: "2026-08-29T00:00:00Z" },
				{ id: "ref-new", seq: 2, sessionId: "sess-new", harness: "claude-code", runBriefCoreDigest: "b".repeat(64), boundAt: "2026-08-30T00:00:00Z" },
			],
		});
		expect(newestAttemptSession(current)?.sessionId).toBe("sess-new");
		expect(toAttemptBoardPresentation(current, plan, true, ((key: string) => key) as never).provider).toBe("claude-code");
	});

	it("offers Engage and a real Merge link when the bound session has an open PR", () => {
		workspaceQueryMock.mockReturnValue({
			data: [{ id: "proj-1", sessions: [{ id: "sess-1", prs: [] }] }],
		});
		scmSummaryMock.mockReturnValue({
			data: [{ number: 42, url: "https://github.com/acme/repo/pull/42", htmlUrl: "https://github.com/acme/repo/pull/42", state: "open", ci: { state: "unknown" }, review: { decision: "none" }, mergeability: { state: "unknown" } }],
		});
		const onEngage = vi.fn();
		const presentation = toAttemptBoardPresentation(attempt(), plan, true, ((key: string) => key) as never);

		render(<AttemptCardAdapter onEngage={onEngage} presentation={presentation} />);

		const mergeLink = screen.getByRole("link", { name: "Merge" });
		expect(mergeLink).toHaveAttribute("href", "https://github.com/acme/repo/pull/42");
		expect(screen.getByRole("button", { name: "Engage" })).toBeInTheDocument();
	});

	it("renders the recorded harness logo instead of a generic or Claude mark", () => {
		workspaceQueryMock.mockReturnValue({ data: [{ id: "proj-1", sessions: [{ id: "sess-1", prs: [] }] }] });
		scmSummaryMock.mockReturnValue({ data: [] });
		const presentation = toAttemptBoardPresentation(attempt(), plan, true, ((key: string) => key) as never);

		render(<AttemptCardAdapter onEngage={vi.fn()} presentation={presentation} />);

		expect(screen.getByRole("img", { name: "codex" })).toBeInTheDocument();
		expect(screen.queryByRole("img", { name: "claude-code" })).not.toBeInTheDocument();
	});

	it("offers only Engage — no Merge control at all — when there is no resolvable pull request", () => {
		workspaceQueryMock.mockReturnValue({ data: [{ id: "proj-1", sessions: [{ id: "sess-1", prs: [] }] }] });
		scmSummaryMock.mockReturnValue({ data: [] });
		const onEngage = vi.fn();
		const presentation = toAttemptBoardPresentation(attempt(), plan, true, ((key: string) => key) as never);

		render(<AttemptRowAdapter onEngage={onEngage} presentation={presentation} />);

		expect(screen.queryByRole("link", { name: "Merge" })).toBeNull();
		expect(screen.getByRole("button", { name: "Engage" })).toBeInTheDocument();
	});

	it("never offers Merge for a non-succeeded (still executing) attempt, even with an open PR", () => {
		workspaceQueryMock.mockReturnValue({ data: [{ id: "proj-1", sessions: [{ id: "sess-1", prs: [] }] }] });
		scmSummaryMock.mockReturnValue({
			data: [{ number: 42, url: "https://github.com/acme/repo/pull/42", htmlUrl: "https://github.com/acme/repo/pull/42", state: "open", ci: { state: "unknown" }, review: { decision: "none" }, mergeability: { state: "unknown" } }],
		});
		const onEngage = vi.fn();
		const presentation = toAttemptBoardPresentation(
			attempt({
				status: "running",
				presentation: { phase: "executing", unconfirmed: false, endedUnclassified: false, nextAction: "Working" },
			}),
			plan,
			true,
			((key: string) => key) as never,
		);

		render(<AttemptCardAdapter onEngage={onEngage} presentation={presentation} />);

		expect(screen.queryByRole("link", { name: "Merge" })).toBeNull();
	});

	it("engaging still calls through — Merge does not replace Engage's own click handler", async () => {
		workspaceQueryMock.mockReturnValue({ data: [{ id: "proj-1", sessions: [{ id: "sess-1", prs: [] }] }] });
		scmSummaryMock.mockReturnValue({
			data: [{ number: 42, url: "https://github.com/acme/repo/pull/42", htmlUrl: "https://github.com/acme/repo/pull/42", state: "open", ci: { state: "unknown" }, review: { decision: "none" }, mergeability: { state: "unknown" } }],
		});
		const onEngage = vi.fn();
		const presentation = toAttemptBoardPresentation(attempt(), plan, true, ((key: string) => key) as never);
		const user = userEvent.setup();

		render(<AttemptCardAdapter onEngage={onEngage} presentation={presentation} />);
		await user.click(screen.getByRole("button", { name: "Engage" }));
		expect(onEngage).toHaveBeenCalledTimes(1);
	});
});

describe("OutcomeRunBoardAdapters — historical Attempt inspection", () => {
	it("opens a non-current (historical) Attempt card for inspection without offering the Engage action", async () => {
		workspaceQueryMock.mockReturnValue({ data: [{ id: "proj-1", sessions: [] }] });
		scmSummaryMock.mockReturnValue({ data: [] });
		const onEngage = vi.fn();
		const historical = toAttemptBoardPresentation(
			attempt({ status: "succeeded", presentation: { phase: "succeeded", unconfirmed: false, endedUnclassified: false, nextAction: "Inspect" } }),
			plan,
			false,
			((key: string) => key) as never,
		);
		const user = userEvent.setup();
		render(<AttemptCardAdapter onEngage={onEngage} presentation={historical} />);
		expect(screen.queryByRole("button", { name: "Engage" })).not.toBeInTheDocument();
		await user.click(screen.getByTestId("board-session-card"));
		expect(onEngage).toHaveBeenCalledTimes(1);
	});

	it("opens a non-current (historical) Attempt row for inspection", async () => {
		workspaceQueryMock.mockReturnValue({ data: [{ id: "proj-1", sessions: [] }] });
		scmSummaryMock.mockReturnValue({ data: [] });
		const onEngage = vi.fn();
		const historical = toAttemptBoardPresentation(attempt({ status: "failed" }), plan, false, ((key: string) => key) as never);
		const user = userEvent.setup();
		render(<AttemptRowAdapter onEngage={onEngage} presentation={historical} />);
		await user.click(screen.getByTestId("board-session-row"));
		expect(onEngage).toHaveBeenCalledTimes(1);
	});
});
