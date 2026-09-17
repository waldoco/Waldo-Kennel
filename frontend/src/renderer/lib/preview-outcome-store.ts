import type { components } from "../../api/schema";

type OutcomeRecord = components["schemas"]["OutcomeResponse"];
type ContractRevisionRecord = components["schemas"]["ContractRevisionResponse"];
type PlanRecord = components["schemas"]["PlanRevisionResponse"];
type CreateOutcomeRequest = components["schemas"]["CreateOutcomeRequest"];
type ReviseOutcomeContractRequest = components["schemas"]["ReviseOutcomeContractRequest"];

const outcomes = new Map<string, OutcomeRecord>();
const projectOutcomeIds = new Map<string, string[]>();
const requestOutcomes = new Map<string, string>();
const plans = new Map<string, PlanRecord>();
const planNumbers = new Map<string, number>();
let sequence = 0;

function previewError(code: string, message: string): never {
	throw { code, message };
}

function nextId(prefix: string): string {
	sequence += 1;
	return `preview-${prefix}-${sequence}`;
}

function revisionFromInput(
	number: number,
	input: Pick<
		CreateOutcomeRequest,
		"clarification" | "constraints" | "goal" | "nonGoals" | "review" | "successCriteria"
	>,
): ContractRevisionRecord {
	const id = nextId("contract");
	return {
		id,
		number,
		goal: input.goal,
		criteria: input.successCriteria.map((text, index) => ({
			criterionId: `${id}:criterion:${index + 1}`,
			contractRevisionId: id,
			position: index + 1,
			text,
		})),
		successCriteria: [...input.successCriteria],
		review: input.review,
		constraints: [...(input.constraints ?? [])],
		nonGoals: [...(input.nonGoals ?? [])],
		...(input.clarification ? { clarification: input.clarification } : {}),
		createdAt: new Date().toISOString(),
	};
}

export function resetPreviewOutcomeStore(): void {
	outcomes.clear();
	projectOutcomeIds.clear();
	requestOutcomes.clear();
	plans.clear();
	planNumbers.clear();
	sequence = 0;
}

export function listPreviewOutcomes(projectId: string): OutcomeRecord[] {
	return (projectOutcomeIds.get(projectId) ?? [])
		.map((id) => outcomes.get(id))
		.filter((outcome): outcome is OutcomeRecord => Boolean(outcome));
}

export function getPreviewOutcome(outcomeId: string): OutcomeRecord | undefined {
	return outcomes.get(outcomeId);
}

export function createPreviewOutcome(projectId: string, input: CreateOutcomeRequest): OutcomeRecord {
	const replayId = requestOutcomes.get(`${projectId}:${input.requestKey}`);
	if (replayId) {
		const replay = outcomes.get(replayId);
		if (replay) return replay;
	}

	const now = new Date().toISOString();
	const revision = revisionFromInput(1, input);
	const outcome: OutcomeRecord = {
		id: nextId("outcome"),
		spaceId: `work-project:${projectId}`,
		title: input.title,
		currentRevisionNumber: 1,
		currentRevision: revision,
		history: [revision],
		createdAt: now,
		updatedAt: now,
	};
	outcomes.set(outcome.id, outcome);
	projectOutcomeIds.set(projectId, [...(projectOutcomeIds.get(projectId) ?? []), outcome.id]);
	requestOutcomes.set(`${projectId}:${input.requestKey}`, outcome.id);
	return outcome;
}

export function revisePreviewOutcome(outcomeId: string, input: ReviseOutcomeContractRequest): OutcomeRecord {
	const current = outcomes.get(outcomeId);
	if (!current) previewError("OUTCOME_NOT_FOUND", "The preview Outcome does not exist.");
	if (current.currentRevisionNumber !== input.expectedRevision) {
		previewError("OUTCOME_CONTRACT_CONFLICT", "The preview contract changed before this revision was saved.");
	}
	const revision = revisionFromInput(current.currentRevisionNumber + 1, input);
	const updated: OutcomeRecord = {
		...current,
		currentRevision: revision,
		currentRevisionNumber: revision.number,
		history: [...current.history, revision],
		latestPlan: undefined,
		updatedAt: new Date().toISOString(),
	};
	outcomes.set(outcomeId, updated);
	plans.delete(outcomeId);
	return updated;
}

export function getPreviewPlan(outcomeId: string): PlanRecord | undefined {
	return plans.get(outcomeId);
}

export function proposePreviewPlan(outcomeId: string, expectedContractRevision: number): PlanRecord {
	const outcome = outcomes.get(outcomeId);
	if (!outcome) previewError("OUTCOME_NOT_FOUND", "The preview Outcome does not exist.");
	if (outcome.currentRevisionNumber !== expectedContractRevision) {
		previewError("PLAN_CONTRACT_STALE", "The preview contract changed before this plan was proposed.");
	}
	const existing = plans.get(outcomeId);
	if (existing?.contractRevisionNumber === expectedContractRevision) return existing;

	const planNumber = (planNumbers.get(outcomeId) ?? 0) + 1;
	const plan: PlanRecord = {
		id: nextId("plan"),
		outcomeId,
		number: planNumber,
		contractRevisionNumber: expectedContractRevision,
		status: "proposed",
		summary: "One bounded Work Unit prepared from the confirmed Outcome contract.",
		assumptions: [],
		blockers: [],
		routingDecisions: [],
		workUnits: [
			{
				id: nextId("work-unit"),
				position: 1,
				kind: "direct",
				title: `Deliver “${outcome.title}”`,
				contractRevisionNumber: expectedContractRevision,
				dependsOn: [],
				criterionIds: [...outcome.currentRevision.criteria.map((criterion) => criterion.criterionId)],
				provider: "codex",
				modelSelection: "provider_default",
				requiredCapabilities: ["worktree.read", "worktree.write", "worktree.exec"],
				approvedChecks: [],
				outputSummary: "Produce the agreed result inside the selected project without exceeding the approved scope.",
				evidenceChecks: [...outcome.currentRevision.successCriteria],
				verificationRequirement: outcome.currentRevision.review,
				stopConditions: [
					"Stop before an unapproved external effect or dependency.",
					"Stop when the contract becomes ambiguous or contradictory.",
				],
			},
		],
		grants: [
			{ id: nextId("grant"), name: "worktree.read", scope: "selected project" },
			{ id: nextId("grant"), name: "worktree.write", scope: "approved Work Unit" },
			{ id: nextId("grant"), name: "worktree.exec", scope: "local verification commands" },
		],
		runBriefCoreDigest: "preview-only-not-a-provider-run-brief",
		createdAt: new Date().toISOString(),
	};
	plans.set(outcomeId, plan);
	planNumbers.set(outcomeId, planNumber);
	outcomes.set(outcomeId, { ...outcome, latestPlan: plan, updatedAt: new Date().toISOString() });
	return plan;
}

export function approvePreviewPlan(outcomeId: string, planId: string, expectedContractRevision: number): PlanRecord {
	const outcome = outcomes.get(outcomeId);
	const plan = plans.get(outcomeId);
	if (!outcome) previewError("OUTCOME_NOT_FOUND", "The preview Outcome does not exist.");
	if (!plan || plan.id !== planId) previewError("PLAN_NOT_FOUND", "The preview plan does not exist.");
	if (
		outcome.currentRevisionNumber !== expectedContractRevision ||
		plan.contractRevisionNumber !== expectedContractRevision
	) {
		previewError("PLAN_CONTRACT_STALE", "The preview contract changed before this plan was authorized.");
	}
	const approved = { ...plan, status: "approved" };
	plans.set(outcomeId, approved);
	outcomes.set(outcomeId, { ...outcome, latestPlan: approved, updatedAt: new Date().toISOString() });
	return approved;
}
