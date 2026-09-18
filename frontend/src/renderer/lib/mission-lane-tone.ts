// Canon lanes (locked 2026-09-17, ruling 2026-09-18): review is its own
// "Ready" lane - finished work awaiting acceptance, not an input ask.
// Unrecognized lanes (including "unavailable") fall into needsYou, matching
// the overview's filter behavior. One canon tone per board lane, shared by
// the lane heading dot, the card status sentence, and the mission detail
// readout, so a state always reads the same color everywhere it appears.
export const boardLane = (lane?: string) =>
	lane === "accepted"
		? "accepted"
		: lane === "review"
			? "review"
			: lane === "observe"
				? "observe"
				: lane === "define" || lane === "authorize"
					? "define"
					: "needsYou";

export const BOARD_LANE_TONE: Record<string, { dot: string; text: string }> = {
	define: { dot: "bg-status-needs-you", text: "text-status-needs-you" },
	needsYou: { dot: "bg-status-in-review", text: "text-status-in-review" },
	review: { dot: "bg-status-ready", text: "text-status-ready" },
	observe: { dot: "bg-status-working", text: "text-status-working" },
	accepted: { dot: "bg-muted-foreground", text: "text-muted-foreground" },
};
