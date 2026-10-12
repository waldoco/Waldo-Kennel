import type { StateTone } from "@pin4sf/kennel-product-ui";

// Canon lanes (locked 2026-09-17, ruling 2026-09-18): review is its own
// "Ready" lane - finished work awaiting acceptance, not an input ask.
// Unrecognized lanes (including "unavailable") fall into needsYou, matching
// the overview's filter behavior.
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

export type BoardLane = ReturnType<typeof boardLane>;

/**
 * One Waldo state tone per board lane, shared by the lane heading, the card's
 * state badge and the mission readout, so a state reads the same everywhere:
 * the owner's lanes carry the Needs you dot, Running the live accent dot,
 * Ready the good check, Finished stays neutral. Colour never travels alone —
 * each tone has its shape cue (StateMark) and the lane word.
 */
export const BOARD_LANE_STATE: Record<BoardLane, StateTone> = {
	define: "needsYou",
	needsYou: "needsYou",
	review: "good",
	observe: "live",
	accepted: "neutral",
};

const CRITICAL_REASONS = new Set(["failed", "lost"]);
const NEUTRAL_REASONS = new Set(["paused", "attempt_paused", "cancelled"]);

/**
 * The tone for one Outcome's own state word. It follows its lane, except
 * where the lane would overstate it: an unreadable status is Unknown
 * (neutral), a failed or lost Attempt is critical, a paused one is neutral.
 */
export function attentionTone(attention?: { lane: string; reason?: string }): StateTone {
	if (!attention || attention.lane === "unavailable") return "neutral";
	if (attention.reason && CRITICAL_REASONS.has(attention.reason)) return "critical";
	if (attention.reason && NEUTRAL_REASONS.has(attention.reason)) return "neutral";
	return BOARD_LANE_STATE[boardLane(attention.lane)];
}

