import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import {
	GovernedDispatchBlockedBanner,
	McpServerBanner,
	ReauthBanner,
	ThreadStateBanner,
} from "./ChatStatusBanners";

// Each of these answers a question the timeline structurally cannot, so the tests are
// about what is said and when it is withheld — a banner for an ordinary state is noise
// that teaches readers to ignore the row.

describe("ReauthBanner", () => {
	it("names the command, because re-authenticating is not something Kennel can do", () => {
		render(
			<ReauthBanner
				account={{
					reauthRequiredAt: "2026-08-03T00:00:00Z",
					reauthReason: "The stored session expired.",
				}}
				harness="codex"
			/>,
		);
		expect(screen.getByRole("alert")).toBeInTheDocument();
		expect(screen.getByText("codex login")).toBeInTheDocument();
		expect(screen.getByText(/The stored session expired/)).toBeInTheDocument();
	});

	it("says the worktree is untouched, since nothing else about the session works", () => {
		render(
			<ReauthBanner account={{ reauthRequiredAt: "2026-08-03T00:00:00Z" }} harness="codex" />,
		);
		expect(screen.getByText(/worktree is untouched/i)).toBeInTheDocument();
	});

	it("names Claude Code's non-interactive authentication command", () => {
		render(
			<ReauthBanner account={{ reauthRequiredAt: "2026-08-03T00:00:00Z" }} harness="claude-code" />,
		);
		expect(screen.getByText("claude auth login")).toBeInTheDocument();
	});

	it("falls back to generic wording rather than guessing a command", () => {
		render(
			<ReauthBanner account={{ reauthRequiredAt: "2026-08-03T00:00:00Z" }} harness="opencode" />,
		);
		expect(screen.queryByText(/login$/)).not.toBeInTheDocument();
		expect(screen.getByText(/agent’s own CLI/)).toBeInTheDocument();
	});

	it("stays silent for an account with no credential demand", () => {
		const { container } = render(
			<ReauthBanner account={{ authMode: "chatgpt", planLabel: "Pro" }} harness="codex" />,
		);
		expect(container).toBeEmptyDOMElement();
	});
});

describe("ThreadStateBanner", () => {
	it("reports a provider-side fault as the provider's, not Kennel's connection", () => {
		render(<ThreadStateBanner threadState={{ status: "system_error" }} />);
		expect(screen.getByText(/thread hit an internal error/i)).toBeInTheDocument();
		expect(screen.getByText(/not in Kennel's connection to it/)).toBeInTheDocument();
	});

	it("reports a closed thread as history Kennel kept and the agent did not", () => {
		render(<ThreadStateBanner threadState={{ status: "closed" }} />);
		expect(screen.getByText(/closed this thread/i)).toBeInTheDocument();
	});

	it("lists what the provider says it is waiting on", () => {
		render(
			<ThreadStateBanner threadState={{ status: "system_error", waitingOn: ["user_input"] }} />,
		);
		expect(screen.getByText(/Waiting on: user_input/)).toBeInTheDocument();
	});

	// active, idle and not_loaded are the ordinary run of a session.
	it.each(["active", "idle", "not_loaded"] as const)("says nothing for %s", (status) => {
		const { container } = render(<ThreadStateBanner threadState={{ status }} />);
		expect(container).toBeEmptyDOMElement();
	});
});

describe("McpServerBanner", () => {
	const broken = [
		{
			name: "playwright",
			status: "failed" as const,
			failureReason: "startup_timeout",
			error: "did not report ready within 30s",
		},
	];

	it("says the agent will work around the missing tools silently", () => {
		render(<McpServerBanner servers={broken} />);
		expect(screen.getByText("A tool server did not start")).toBeInTheDocument();
		expect(screen.getByText(/works around them\s+silently/)).toBeInTheDocument();
	});

	it("names the server, its classification and the provider's own text", () => {
		render(<McpServerBanner servers={broken} />);
		expect(screen.getByText("playwright")).toBeInTheDocument();
		expect(screen.getByText(/startup_timeout/)).toBeInTheDocument();
		expect(screen.getByText(/did not report ready within 30s/)).toBeInTheDocument();
	});

	it("offers a reload", async () => {
		const onReload = vi.fn();
		render(<McpServerBanner servers={broken} onReload={onReload} />);
		await userEvent.click(screen.getByRole("button", { name: /Reload/ }));
		expect(onReload).toHaveBeenCalledOnce();
	});

	// The daemon refuses a reload mid-turn, so the control explains itself rather than
	// being allowed to fail.
	it("disables the reload mid-turn and says why", () => {
		render(<McpServerBanner servers={broken} onReload={vi.fn()} turnInFlight />);
		const button = screen.getByRole("button", { name: /Reload/ });
		expect(button).toBeDisabled();
		expect(button).toHaveAttribute(
			"title",
			expect.stringContaining("Finish or stop the current turn"),
		);
	});

	it("draws no control at all when the harness cannot reload", () => {
		render(<McpServerBanner servers={broken} />);
		expect(screen.queryByRole("button")).not.toBeInTheDocument();
	});

	it("surfaces a failed reload", () => {
		render(<McpServerBanner servers={broken} onReload={vi.fn()} error="controller not ready" />);
		expect(screen.getByText("controller not ready")).toBeInTheDocument();
	});

	// A healthy server is not news. The caller filters, and an empty list must not
	// leave a permanent bar above the conversation saying nothing is wrong.
	it("says nothing when no server is broken", () => {
		const { container } = render(<McpServerBanner servers={[]} />);
		expect(container).toBeEmptyDOMElement();
	});
});

describe("GovernedDispatchBlockedBanner", () => {
	it("says nothing when neither list has a claim", () => {
		const { container } = render(
			<GovernedDispatchBlockedBanner turnBlocks={[]} controlBlocks={[]} />,
		);
		expect(container).toBeEmptyDOMElement();
	});

	it("reads as waiting, not uncertain, for a claimed/dispatching turn", () => {
		render(
			<GovernedDispatchBlockedBanner
				turnBlocks={[
					{
						kind: "turn",
						turnId: "turn-1",
						state: "dispatching",
						quiescence: "not_applicable",
						since: "2026-09-16T12:00:00Z",
					},
				]}
				controlBlocks={[]}
			/>,
		);
		expect(screen.getByRole("alert")).toBeInTheDocument();
		expect(screen.getByText(/Waiting for the provider to acknowledge/)).toBeInTheDocument();
		expect(screen.queryByText(/failed/i)).not.toBeInTheDocument();
		expect(screen.queryByText(/safe to retry/i)).not.toBeInTheDocument();
	});

	// A control claim blocks every turn's dispatch without living on any turn --
	// this is the exact false negative a per-turn note alone would miss.
	it("surfaces a control claim even with no turn claim at all", () => {
		render(
			<GovernedDispatchBlockedBanner
				turnBlocks={[]}
				controlBlocks={[
					{
						kind: "steer",
						id: "control-1",
						state: "delivery_unknown",
						quiescence: "not_applicable",
						providerTurnId: "provider-turn-1",
						since: "2026-09-16T12:00:00Z",
					},
				]}
			/>,
		);
		expect(screen.getByText(/Steer/)).toBeInTheDocument();
		expect(screen.getByText(/Whether the provider received this is not known/)).toBeInTheDocument();
		expect(screen.queryByText(/failed/i)).not.toBeInTheDocument();
		expect(screen.queryByText(/safe to retry/i)).not.toBeInTheDocument();
	});

	// Regression for the reviewer's HIGH finding: a containment-failed
	// interrupt can be provider-acknowledged and still land in
	// delivery_unknown purely because the process-tree check failed. The copy
	// must state only the proven fact -- verification failed -- and must not
	// speculate either way on whether the provider received the stop.
	it("states only the proven fact for a containment-failed interrupt", () => {
		render(
			<GovernedDispatchBlockedBanner
				turnBlocks={[]}
				controlBlocks={[
					{
						kind: "interrupt",
						id: "control-1",
						state: "delivery_unknown",
						quiescence: "pending",
						providerTurnId: "provider-turn-1",
						since: "2026-09-16T12:00:00Z",
					},
				]}
			/>,
		);
		expect(screen.getByText(/Interrupt/)).toBeInTheDocument();
		expect(
			screen.getByText(
				/This stop has not settled because Kennel could not verify that the process stopped\. Nothing else will send until this is resolved\./,
			),
		).toBeInTheDocument();
		expect(screen.queryByText(/[Dd]elivery is uncertain/)).not.toBeInTheDocument();
		expect(screen.queryByText(/whether the provider received/i)).not.toBeInTheDocument();
		expect(screen.queryByText(/failed/i)).not.toBeInTheDocument();
		expect(screen.queryByText(/safe to retry/i)).not.toBeInTheDocument();
	});

	// Regression for the reviewer's second HIGH finding: an answer has no
	// provider acceptance step (Resolve's evidence is Kennel's own write
	// completeness), so its copy must never invent one.
	it("never invents a provider acceptance step for an answer claim", () => {
		render(
			<GovernedDispatchBlockedBanner
				turnBlocks={[]}
				controlBlocks={[
					{
						kind: "answer",
						id: "control-1",
						state: "claimed",
						quiescence: "not_applicable",
						requestInstanceId: "request-instance-1",
						since: "2026-09-16T12:00:00Z",
					},
				]}
			/>,
		);
		expect(screen.getByText(/Answer/)).toBeInTheDocument();
		expect(screen.getByText(/no provider acceptance step/)).toBeInTheDocument();
		expect(screen.queryByText(/waiting on the provider to accept/i)).not.toBeInTheDocument();
	});

	it("says an answer's delivery is unknown without inventing acceptance either", () => {
		render(
			<GovernedDispatchBlockedBanner
				turnBlocks={[]}
				controlBlocks={[
					{
						kind: "answer",
						id: "control-1",
						state: "delivery_unknown",
						quiescence: "not_applicable",
						requestInstanceId: "request-instance-1",
						since: "2026-09-16T12:00:00Z",
					},
				]}
			/>,
		);
		expect(
			screen.getByText(/recorded locally, but whether the provider ever received it is not known/),
		).toBeInTheDocument();
	});

	// Two different kinds, two different unknowns, on screen at once: neither
	// line may borrow the other's wording.
	it("gives each block its own line when the two lists disagree", () => {
		render(
			<GovernedDispatchBlockedBanner
				turnBlocks={[
					{
						kind: "turn",
						turnId: "turn-1",
						state: "claimed",
						quiescence: "not_applicable",
						since: "2026-09-16T12:00:00Z",
					},
				]}
				controlBlocks={[
					{
						kind: "interrupt",
						id: "control-1",
						state: "delivery_unknown",
						quiescence: "pending",
						providerTurnId: "provider-turn-1",
						since: "2026-09-16T12:00:00Z",
					},
				]}
			/>,
		);
		expect(screen.getByText(/Waiting for the provider to acknowledge/)).toBeInTheDocument();
		expect(screen.getByText(/could not verify that the process stopped/)).toBeInTheDocument();
		expect(screen.queryByText(/[Dd]elivery is uncertain/)).not.toBeInTheDocument();
		expect(screen.queryByText(/whether the provider received/i)).not.toBeInTheDocument();
	});
});
