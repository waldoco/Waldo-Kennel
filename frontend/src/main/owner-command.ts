export type AttemptReplacementProposal = {
	outcomeId: string;
	predecessorAttemptId: string;
	planRevisionId: string;
	workUnitId: string;
	runIntentGeneration: number;
	contractRevisionNumber: number;
	action: "replace";
	requestKey: string;
};

const replacementKeys = [
	"outcomeId", "predecessorAttemptId", "planRevisionId", "workUnitId",
	"runIntentGeneration", "contractRevisionNumber", "action", "requestKey",
] as const;
const boundedStrings = ["outcomeId", "predecessorAttemptId", "planRevisionId", "workUnitId", "requestKey"] as const;

export function parseAttemptReplacementProposal(input: unknown): AttemptReplacementProposal | null {
	if (!input || typeof input !== "object" || Array.isArray(input)) return null;
	const record = input as Record<string, unknown>;
	if (Object.keys(record).length !== replacementKeys.length || replacementKeys.some((key) => !Object.hasOwn(record, key))) return null;
	if (record.action !== "replace") return null;
	for (const key of boundedStrings) {
		if (typeof record[key] !== "string" || !record[key].trim() || record[key].length > 256) return null;
	}
	if (!Number.isSafeInteger(record.runIntentGeneration) || Number(record.runIntentGeneration) < 1 ||
		!Number.isSafeInteger(record.contractRevisionNumber) || Number(record.contractRevisionNumber) < 1) return null;
	return {
		outcomeId: String(record.outcomeId).trim(),
		predecessorAttemptId: String(record.predecessorAttemptId).trim(),
		planRevisionId: String(record.planRevisionId).trim(),
		workUnitId: String(record.workUnitId).trim(),
		runIntentGeneration: Number(record.runIntentGeneration),
		contractRevisionNumber: Number(record.contractRevisionNumber),
		action: "replace",
		requestKey: String(record.requestKey).trim(),
	};
}
