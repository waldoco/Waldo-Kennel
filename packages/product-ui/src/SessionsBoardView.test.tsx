import { fireEvent, render, screen, within } from "@testing-library/react";
import { createElement, type ComponentProps } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import {
	ARCHIVE_TOGGLE_HEIGHT_PX,
	SessionCardView,
	SessionsArchiveView,
	SessionsBoardGridView,
	archiveToggleHeightClassName,
	archiveToggleOffsetClassName,
	type BoardSessionPresentation,
	type BoardSplitLaneLabels,
} from "./SessionsBoardView";
import { SessionRowView, SessionsListView, SessionsViewSwitch } from "./SessionsListView";
import {
	boardAttentionZoneOrder,
	getAttentionZoneViewForZone,
} from "./session-presentation";
import type { ExternalLinkProps } from "./external-link";

const useReducedMotionMock = vi.hoisted(() => vi.fn(() => false));
const lastArchiveMotionTransition = vi.hoisted(() => ({
	current: undefined as { duration?: number } | undefined,
}));

vi.mock("motion/react", async (importOriginal) => {
	const actual = await importOriginal<typeof import("motion/react")>();
	function MotionDiv(props: ComponentProps<typeof actual.motion.div>) {
		lastArchiveMotionTransition.current = props.transition as { duration?: number } | undefined;
		return createElement(actual.motion.div, props);
	}
	return {
		...actual,
		useReducedMotion: useReducedMotionMock,
		motion: { ...actual.motion, div: MotionDiv },
	};
});

function ExternalLink({ ariaLabel, children, stopPropagation, ...props }: ExternalLinkProps) {
	return (
		<a
			{...props}
			aria-label={ariaLabel}
			onClick={stopPropagation ? (event) => event.stopPropagation() : undefined}
		>
			{children}
		</a>
	);
}

const splitLabels: BoardSplitLaneLabels = {
	columnAria: (label) => `${label} sessions`,
	countSessions: (count, label) => `${count} ${label} session${count === 1 ? "" : "s"}`,
	idleWorkingAria: "Idle / Working sessions",
	laneSummary: (primary, secondary) => `${primary} / ${secondary} lane summary`,
	readyMergedAria: "Ready / Merged sessions",
	tones: {
		idle: { countLabel: "idle", label: "Idle", regionLabel: "Idle sessions" },
		working: { countLabel: "working", label: "Working", regionLabel: "Working sessions" },
		ready: { countLabel: "ready", label: "Ready", regionLabel: "Ready sessions" },
		merged: { countLabel: "merged", label: "Merged", regionLabel: "Merged sessions" },
	},
};

const baseSession: BoardSessionPresentation = {
	id: "session-1",
	provider: "codex",
	status: "idle",
	title: "portable task",
	updatedAt: "2026-08-09T10:00:00Z",
};

describe("SessionsBoardView", () => {
	beforeEach(() => {
		useReducedMotionMock.mockReturnValue(false);
		lastArchiveMotionTransition.current = undefined;
	});

	it("renders four unsplit lanes with aggregate counts and the exact board gutter", () => {
		const sessions: BoardSessionPresentation[] = [
			baseSession,
			{ ...baseSession, id: "working", status: "working", title: "working task" },
			{ ...baseSession, id: "ready", status: "mergeable", title: "ready task" },
			{ ...baseSession, id: "merged", status: "merged", title: "merged task" },
		];
		render(
			<SessionsBoardGridView
				columns={boardAttentionZoneOrder.map((zone) => getAttentionZoneViewForZone(zone))}
				labels={splitLabels}
				renderSessionCard={(session) => <div data-testid={`card-${session.id}`}>{session.title}</div>}
				sessions={sessions}
			/>,
		);

		const workLane = screen.getByRole("region", { name: "Running sessions" });
		expect(workLane).toHaveTextContent("portable task");
		expect(workLane).toHaveTextContent("working task");
		expect(within(workLane).getByLabelText("2 Running sessions")).toHaveTextContent("2");
		expect(workLane.querySelectorAll(".overflow-y-auto")).toHaveLength(1);

		const mergeLane = screen.getByRole("region", { name: "Ready sessions" });
		expect(mergeLane).toHaveTextContent("ready task");
		expect(mergeLane).toHaveTextContent("merged task");
		expect(within(mergeLane).getByLabelText("2 Ready sessions")).toHaveTextContent("2");

		const scroller = screen.getByTestId("board-horizontal-scroll");
		expect(scroller).toHaveClass("board-horizontal-scrollbar");
		expect(scroller.firstElementChild).toHaveClass("min-w-[74rem]", "grid-cols-4", "gap-2", "px-2.5");
		expect(screen.getAllByTestId("board-column")).toHaveLength(4);
	});

	it("keeps all four list lanes visible and aligns rows to the reference columns", () => {
		const sessions: BoardSessionPresentation[] = [
			{ ...baseSession, id: "ready", status: "mergeable", title: "ready task" },
			{ ...baseSession, id: "merged", status: "merged", title: "merged task" },
		];
		render(
			<SessionsListView
				columns={boardAttentionZoneOrder.map((zone) => getAttentionZoneViewForZone(zone))}
				labels={splitLabels}
				renderSessionRow={(session) => (
					<SessionRowView
						artifact={<a href="https://example.com/artifact">Investor Memo</a>}
						externalLink={ExternalLink}
						interactive={false}
						labels={{
							formatTime: () => "6m ago",
							intakeIssue: (id) => `Issue ${id}`,
							pr: {
								short: "PR",
								states: { closed: "closed", draft: "draft", merged: "merged", open: "open" },
							},
							updatedAt: (timestamp) => `Updated ${timestamp}`,
						}}
						renderAvatar={(provider) => <span aria-label={provider}>C</span>}
						session={session}
					/>
				)}
				sessions={sessions}
			/>,
		);

		expect(screen.getAllByTestId("board-list-lane")).toHaveLength(4);
		expect(screen.getByRole("region", { name: "Needs Choice sessions" })).toHaveTextContent("0");
		const readyLane = screen.getByRole("region", { name: "Ready sessions" });
		expect(within(readyLane).getByLabelText("2 Ready sessions")).toHaveTextContent("2");
		expect(within(readyLane).getAllByText("Investor Memo")).toHaveLength(2);
		const row = within(readyLane).getAllByTestId("board-session-row")[0];
		expect(row).toHaveClass("min-h-row-list", "gap-2.5", "px-2.5", "py-1");
		const cells = row.querySelectorAll(".grow-0");
		expect(cells[0]).toHaveClass("basis-[16.25rem]");
		expect(cells[1]).toHaveClass("basis-[13.5rem]");
		expect(cells[2]).toHaveClass("basis-[8.5rem]");
		expect(cells[3]).toHaveClass("basis-[11rem]");
	});

	it("renders a neutral card with grouped multi-PR, usage, and action presentation", () => {
		const onOpen = vi.fn();
		render(
			<SessionCardView
				action={<button type="button">Restore</button>}
				branchAction={<button type="button">Copy branch</button>}
				externalLink={ExternalLink}
				labels={{
					formatTime: () => "5m ago",
					intakeIssue: (id) => `Issue ${id}`,
					pr: {
						short: "PR",
						states: { closed: "closed", draft: "draft", merged: "merged", open: "open" },
					},
					updatedAt: (timestamp) => `Updated ${timestamp}`,
				}}
				onOpen={onOpen}
				prs={[
					{ number: 10, state: "open", url: "https://example.com/pull/10" },
					{ number: 11, state: "open", url: "https://example.com/pull/11" },
					{ number: 12, state: "merged", url: "https://example.com/pull/12" },
				]}
				renderAvatar={(provider) => <span role="img" aria-label={provider}>C</span>}
				session={{ ...baseSession, branch: "feat/portable", trackerIssueId: "github:42" }}
				usage={{ accessibleLabel: "12,400 tokens", compactLabel: "12.4K tok" }}
			/>,
		);

		expect(screen.getByLabelText("#10, #11 open")).toHaveTextContent("PR#10,#11open");
		expect(screen.getByLabelText("#12 merged")).toHaveTextContent("PR#12merged");
		expect(screen.getByText("12.4K tok")).toHaveAccessibleName("12,400 tokens");
		expect(screen.getByText("5m ago")).toHaveAttribute("title", "Updated 2026-08-09T10:00:00Z");
		expect(screen.getByText("github:42")).toHaveAttribute("title", "Issue github:42");

		fireEvent.click(screen.getByRole("button", { name: "portable task" }));
		expect(onOpen).toHaveBeenCalledOnce();
	});

	it("truncates the status before card metrics can collide", () => {
		render(
			<SessionCardView
				externalLink={ExternalLink}
				labels={{
					formatTime: () => "1h ago",
					intakeIssue: (id) => `Issue ${id}`,
					pr: {
						short: "PR",
						states: { closed: "closed", draft: "draft", merged: "merged", open: "open" },
					},
					updatedAt: (timestamp) => `Updated ${timestamp}`,
				}}
				renderAvatar={(provider) => <span role="img" aria-label={provider}>C</span>}
				session={{ ...baseSession, status: "review_pending" }}
				usage={{ accessibleLabel: "24,600,000 tokens", compactLabel: "24.6M tok" }}
			/>,
		);

		// The state line owns its own row and truncates there; the usage metric
		// sits in the provenance row and never wraps. Neither can push the other.
		const statusLabel = screen.getByText("Review pending");
		expect(statusLabel).toHaveClass("min-w-0", "truncate");
		expect(screen.getByText("24.6M tok")).toHaveClass("shrink-0", "whitespace-nowrap");
	});

	it("keeps archive toggle height and board offset classes in lockstep", () => {
		expect(archiveToggleHeightClassName).toBe(`h-[${ARCHIVE_TOGGLE_HEIGHT_PX}px]`);
		expect(archiveToggleOffsetClassName).toBe(`pb-[${ARCHIVE_TOGGLE_HEIGHT_PX}px]`);
	});

	it("overlays the archive, keeps cards mounted after collapse, and resets on resetKey", async () => {
		const { rerender } = render(
			<SessionsArchiveView
				labels={{ archive: "Archive", archiveAria: "Archive, 1 session", archivedSessions: "Archived sessions" }}
				renderSessionCard={(session) => <div role="listitem">{session.title}</div>}
				resetKey="p1"
				sessions={[baseSession]}
			/>,
		);

		const archiveButton = screen.getByRole("button", { name: "Archive, 1 session" });
		expect(archiveButton).toHaveClass(archiveToggleHeightClassName, "w-full", "py-0");
		expect(archiveButton.parentElement).toHaveClass("absolute", "inset-x-0", "bottom-0", "bg-background");
		expect(within(archiveButton).getByText("Archive")).toHaveClass("text-2xs", "font-medium");
		expect(within(archiveButton).getByText("Archive")).not.toHaveClass("font-mono", "uppercase");

		fireEvent.click(archiveButton);
		const archive = await screen.findByRole("list", { name: "Archived sessions" });
		expect(archive).toHaveClass("scrollbar-none", "grid", "overflow-y-auto", "max-h-[28vh]");
		const card = within(archive).getByText("portable task");

		fireEvent.click(archiveButton);
		expect(archiveButton).toHaveAttribute("aria-expanded", "false");
		expect(archive).toBeInTheDocument();
		expect(archive).toHaveAttribute("aria-hidden", "true");
		expect(archive).toHaveAttribute("inert");
		expect(archive).toHaveClass("pointer-events-none");
		expect(screen.queryByRole("list", { name: "Archived sessions" })).not.toBeInTheDocument();

		fireEvent.click(archiveButton);
		const reopened = screen.getByRole("list", { name: "Archived sessions" });
		expect(reopened).toBe(archive);
		expect(within(reopened).getByText("portable task")).toBe(card);

		rerender(
			<SessionsArchiveView
				labels={{ archive: "Archive", archiveAria: "Archive, 1 session", archivedSessions: "Archived sessions" }}
				renderSessionCard={(session) => <div role="listitem">{session.title}</div>}
				resetKey="p2"
				sessions={[baseSession]}
			/>,
		);
		expect(screen.getByRole("button", { name: "Archive, 1 session" })).toHaveAttribute("aria-expanded", "false");
		expect(screen.queryByRole("list", { name: "Archived sessions" })).not.toBeInTheDocument();
	});

	it("skips archive motion when the user prefers reduced motion", async () => {
		useReducedMotionMock.mockReturnValue(true);
		render(
			<SessionsArchiveView
				labels={{ archive: "Archive", archiveAria: "Archive, 1 session", archivedSessions: "Archived sessions" }}
				renderSessionCard={(session) => <div role="listitem">{session.title}</div>}
				sessions={[baseSession]}
			/>,
		);

		const archiveButton = screen.getByRole("button", { name: "Archive, 1 session" });
		expect(archiveButton.querySelector("svg")).toHaveClass("transition-none");

		fireEvent.click(archiveButton);
		await screen.findByRole("list", { name: "Archived sessions" });
		expect(lastArchiveMotionTransition.current).toEqual({ duration: 0 });
		expect(archiveButton.querySelector("svg")).toHaveClass("transition-none");
	});
});

describe("SessionsViewSwitch", () => {
	const labels = { ariaLabel: "View", board: "Board", list: "List" };

	it("calls onChange for the clicked mode when enabled", () => {
		const onChange = vi.fn();
		render(<SessionsViewSwitch labels={labels} onChange={onChange} value="list" />);
		fireEvent.click(screen.getByRole("tab", { name: "Board" }));
		expect(onChange).toHaveBeenCalledWith("board");
	});

	it("disabled renders both segments non-interactive and never calls onChange, rather than silently ignoring the click", () => {
		const onChange = vi.fn();
		render(<SessionsViewSwitch disabled labels={labels} onChange={onChange} value="list" />);
		expect(screen.getByTestId("sessions-view-switch")).toHaveAttribute("aria-disabled", "true");
		expect(screen.getByRole("tab", { name: "Board" })).toBeDisabled();
		expect(screen.getByRole("tab", { name: "List" })).toBeDisabled();
		fireEvent.click(screen.getByRole("tab", { name: "Board" }));
		expect(onChange).not.toHaveBeenCalled();
	});
});
