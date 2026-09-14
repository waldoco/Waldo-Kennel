import type { components } from "../../../api/schema";

/** The daemon's canonical Attempt presentation phases, named once for every surface. */
export const ATTEMPT_PHASES = {
	awaitingStart: "awaiting_start",
	executing: "executing",
	suspended: "suspended",
	unconfirmed: "unconfirmed",
	needsInput: "needs_input",
	endedUnclassified: "ended_unclassified",
	haltedFailed: "halted_failed",
	haltedCancelled: "halted_cancelled",
	suspectLost: "suspect_lost",
	succeeded: "succeeded",
} as const;

type AttemptPhase = components["schemas"]["AttemptPresentationResponse"]["phase"];

/** Phases where the Attempt has ended; engagement is inspect-only from here. */
export const ENDED_ATTEMPT_PHASES: ReadonlySet<AttemptPhase> = new Set([
	ATTEMPT_PHASES.endedUnclassified,
	ATTEMPT_PHASES.haltedFailed,
	ATTEMPT_PHASES.haltedCancelled,
	ATTEMPT_PHASES.suspectLost,
	ATTEMPT_PHASES.succeeded,
]);
