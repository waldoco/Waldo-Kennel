import { describe, expect, it } from "vitest";
import { parseAttemptReplacementProposal } from "./owner-command";

const valid = {
	outcomeId: " out-1 ", predecessorAttemptId: "attempt-1", planRevisionId: "plan-1", workUnitId: "unit-1",
	runIntentGeneration: 2, contractRevisionNumber: 3, action: "replace", requestKey: " key-1 ",
};

describe("parseAttemptReplacementProposal", () => {
	it("returns the exact normalized closed command", () => {
		expect(parseAttemptReplacementProposal(valid)).toEqual({ ...valid, outcomeId: "out-1", requestKey: "key-1" });
	});
	it.each([
		null, [], {}, { ...valid, action: "cancel" }, { ...valid, extra: true },
		{ ...valid, outcomeId: " " }, { ...valid, requestKey: "x".repeat(257) },
		{ ...valid, runIntentGeneration: 0 }, { ...valid, runIntentGeneration: 1.5 },
		{ ...valid, contractRevisionNumber: Number.MAX_SAFE_INTEGER + 1 },
	])("rejects an expanded or invalid command: %j", (input) => {
		expect(parseAttemptReplacementProposal(input)).toBeNull();
	});
});
