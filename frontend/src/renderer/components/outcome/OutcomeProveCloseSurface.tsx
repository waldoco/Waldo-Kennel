import { EvidenceRow, FactList, StateBadge, StateMark, evidenceVerdictTone, type EvidenceVerdict } from "@pin4sf/kennel-product-ui";
import { CheckCircle2, CircleAlert, RotateCcw } from "lucide-react";
import { type ReactNode, useState } from "react";
import { useTranslation } from "react-i18next";

import type { MessageKey } from "../../i18n/messages";
import {
	useDecideOutcomeAcceptance,
	useOutcomeProof,
	useRecordOutcomeEvidence,
	useRecordOutcomeVerification,
	type OutcomeProofRecord,
} from "../../hooks/useOutcome";
import { formatTimeCompact } from "../../lib/format-time";
import { cn } from "../../lib/utils";
import { Button } from "../ui/button";
import { Input } from "../ui/input";
import { Label } from "../ui/label";

type Props = { outcomeId: string };

type EvidenceDraft = {
	summary: string;
	sourceRef: string;
	kind: "supporting" | "contradicting";
	sourceType: "artifact" | "deterministic_check" | "provider_output" | "owner_walkthrough";
	producerType: "user" | "provider" | "tool";
	producerRef: string;
};
type VerificationDraft = {
	method: string;
	independenceClass: "deterministic" | "producer_self_check" | "separate_session" | "cross_provider" | "owner_walkthrough";
	result: "passed" | "failed" | "inconclusive" | "exception";
	producerRef: string;
	verifierRef: string;
	producerProvider: string;
	verifierProvider: string;
	detail: string;
};

const STATUS_LABEL_KEYS: Record<string, MessageKey> = {
	active: "outcome.proof.statusActive",
	ready_for_acceptance: "outcome.proof.statusReady",
	accepted: "outcome.proof.statusAccepted",
	rework_required: "outcome.proof.statusRework",
};

const INDEPENDENCE_LABEL_KEYS: Record<string, MessageKey> = {
	owner_walkthrough: "outcome.proof.ownerWalkthrough",
	deterministic: "outcome.proof.deterministic",
	producer_self_check: "outcome.proof.producerSelfCheck",
	separate_session: "outcome.proof.separateSession",
	cross_provider: "outcome.proof.crossProvider",
};

const defaultEvidence: EvidenceDraft = {
	summary: "",
	sourceRef: "",
	kind: "supporting",
	sourceType: "owner_walkthrough",
	producerType: "user",
	producerRef: "owner",
};
const defaultVerification: VerificationDraft = {
	method: "", independenceClass: "owner_walkthrough", result: "passed", producerRef: "", verifierRef: "owner",
	producerProvider: "", verifierProvider: "", detail: "",
};

export function OutcomeProveCloseSurface({ outcomeId }: Props) {
	const { t } = useTranslation();
	const proofQuery = useOutcomeProof(outcomeId);
	const evidenceMutation = useRecordOutcomeEvidence(outcomeId);
	const verificationMutation = useRecordOutcomeVerification(outcomeId);
	const decisionMutation = useDecideOutcomeAcceptance(outcomeId);
	const [evidenceDrafts, setEvidenceDrafts] = useState<Record<string, EvidenceDraft>>({});
	const [verificationDrafts, setVerificationDrafts] = useState<Record<string, VerificationDraft>>({});
	const [decisionSummary, setDecisionSummary] = useState("");
	const [resourceDisposition, setResourceDisposition] = useState<"retain" | "cleanup_later" | "not_applicable">("retain");
	const [reentryTargetType, setReentryTargetType] = useState<"attempt" | "work_unit" | "plan" | "contract">("contract");
	const [reentryTargetId, setReentryTargetId] = useState("");

	const proof = proofQuery.proof;
	const pending = evidenceMutation.pending || verificationMutation.pending || decisionMutation.pending;
	const failure = evidenceMutation.failure ?? verificationMutation.failure ?? decisionMutation.failure ?? proofQuery.failure;
	const reentryReady = reentryTargetType === "contract" || Boolean(reentryTargetId.trim());

	async function addEvidence(criterion: OutcomeProofRecord["criteria"][number]) {
		if (!proof || pending) return;
		const draft = evidenceDrafts[criterion.criterionId] ?? defaultEvidence;
		if (!draft.summary.trim() || !draft.sourceRef.trim() || !draft.producerRef.trim()) return;
		try {
			await evidenceMutation.submit({
				expectedContractRevision: proof.contractRevision.number,
				contractRevisionId: proof.contractRevision.id,
				criterionId: criterion.criterionId,
				subjectType: "outcome",
				subjectId: proof.outcomeId,
				subjectRevision: proof.contractRevision.id,
				kind: draft.kind,
				sourceType: draft.sourceType,
				sourceRef: draft.sourceRef.trim(),
				producerType: draft.producerType,
				producerRef: draft.producerRef.trim(),
				summary: draft.summary.trim(),
				contentDigest: await sha256(`${draft.sourceType}\n${draft.sourceRef.trim()}\n${draft.summary.trim()}`),
				requestKey: crypto.randomUUID(),
			});
			setEvidenceDrafts((current) => ({ ...current, [criterion.criterionId]: { ...defaultEvidence } }));
		} catch {
			// The mutation exposes the daemon's typed refusal.
		}
	}

	async function addVerification(criterion: OutcomeProofRecord["criteria"][number]) {
		if (!proof || pending) return;
		const draft = verificationDrafts[criterion.criterionId] ?? defaultVerification;
		const latestEvidence = criterion.evidence[criterion.evidence.length - 1];
		if (!latestEvidence || !draft.method.trim() || !draft.verifierRef.trim()) return;
		try {
			await verificationMutation.submit({
				expectedContractRevision: proof.contractRevision.number,
				contractRevisionId: proof.contractRevision.id,
				criterionId: criterion.criterionId,
				subjectType: latestEvidence.subjectType,
				subjectId: latestEvidence.subjectId,
				subjectRevision: latestEvidence.subjectRevision,
				evidenceItemIds: [latestEvidence.id],
				method: draft.method.trim(),
				independenceClass: draft.independenceClass,
				result: draft.result,
				producerRef: draft.producerRef.trim() || undefined,
				verifierRef: draft.verifierRef.trim(),
				producerProvider: draft.producerProvider.trim() || undefined,
				verifierProvider: draft.verifierProvider.trim() || undefined,
				detail: draft.detail.trim() || undefined,
				requestKey: crypto.randomUUID(),
			});
			setVerificationDrafts((current) => ({ ...current, [criterion.criterionId]: { ...defaultVerification } }));
		} catch {
			// The mutation exposes the daemon's typed refusal.
		}
	}

	async function decide(kind: "accept" | "request_rework" | "reopen") {
		if (!proof || pending || !decisionSummary.trim()) return;
		const targetId = reentryTargetId.trim() || (reentryTargetType === "contract" ? proof.contractRevision.id : "");
		if (kind !== "accept" && !targetId) return;
		try {
			await decisionMutation.submit({
				expectedContractRevision: proof.contractRevision.number,
				contractRevisionId: proof.contractRevision.id,
				kind,
				summary: decisionSummary.trim(),
				resourceDisposition,
				reentryTargetType: kind === "accept" ? undefined : reentryTargetType,
				reentryTargetId: kind === "accept" ? undefined : targetId,
				requestKey: crypto.randomUUID(),
			});
			setDecisionSummary("");
		} catch {
			// The mutation exposes the daemon's typed refusal.
		}
	}

	if (proofQuery.isLoading) {
		return <p className="text-sm text-wd-secondary" role="status">{t("outcome.proof.loading")}</p>;
	}
	if (!proof) {
		return <ProofFailure message={failure?.message ?? t("outcome.proof.unavailable")} onRetry={proofQuery.refetch} />;
	}

	const accepted = proof.status === "accepted";
	const summaryMissing = !decisionSummary.trim();
	const acceptReason = summaryMissing ? t("outcome.proof.summaryRequired") : undefined;
	const reentryReason = summaryMissing ? t("outcome.proof.summaryRequired") : !reentryReady ? t("outcome.proof.reentryIdRequired") : undefined;
	const becameTrue = proof.criteria.filter((criterion) => criterion.ready);
	const didNotHappen = proof.criteria.filter((criterion) => !criterion.ready);
	const result = proof.result ?? { artifacts: [], checks: [], changes: [], uncertainty: [], nextSafeAction: proof.nextAction };

	return (
		<div className="flex max-w-3xl flex-col gap-5" data-testid="outcome-prove-close-surface">
			<header className="flex flex-col gap-1">
				<div className="flex flex-wrap items-center gap-2">
					<h2 className="text-base font-medium text-wd-primary">{t("outcome.proof.heading")}</h2>
					<StateBadge
						label={t(STATUS_LABEL_KEYS[proof.status] ?? "outcome.proof.statusActive")}
						tone={accepted ? "good" : proof.status === "ready_for_acceptance" ? "needsYou" : proof.status === "rework_required" ? "caution" : "neutral"}
					/>
				</div>
				<p className="text-xs tabular-nums text-wd-tertiary">{t("outcome.proof.binding", { revision: proof.contractRevision.number })}</p>
			</header>

			<ResultSummary proof={proof} />

			<div className="flex flex-col gap-4">
				{proof.criteria.map((criterion) => (
					<CriterionCard
						key={criterion.criterionId}
						criterion={criterion}
						evidenceDraft={evidenceDrafts[criterion.criterionId] ?? defaultEvidence}
						onEvidenceDraft={(draft) => setEvidenceDrafts((current) => ({ ...current, [criterion.criterionId]: draft }))}
						onSubmitEvidence={() => void addEvidence(criterion)}
						verificationDraft={verificationDrafts[criterion.criterionId] ?? defaultVerification}
						onVerificationDraft={(draft) => setVerificationDrafts((current) => ({ ...current, [criterion.criterionId]: draft }))}
						onSubmitVerification={() => void addVerification(criterion)}
						pending={pending}
					/>
				))}
			</div>

			{/* The close: what became true, what did not, what the close cannot
			    vouch for, and what is still open. Kennel can say the evidence is
			    enough; only the owner closes. */}
			<section aria-labelledby="proof-decision-title" className="flex flex-col gap-4 rounded-lg hairline border-wd-hairline bg-wd-surface p-5" data-testid="proof-decision-card">
				<div className="flex min-w-0 flex-col gap-1">
					<div className="flex items-center gap-2">
						<p className="text-xs text-wd-secondary">{accepted ? t("outcome.proof.reopenTitle") : t("outcome.proof.decisionTitle")}</p>
						{accepted ? <StateBadge label={t("outcome.proof.statusAccepted")} tone="good" /> : null}
					</div>
					<h3 className="text-lg font-medium leading-6 text-wd-primary" id="proof-decision-title">{proof.contractRevision.goal}</h3>
				</div>

				{becameTrue.length > 0 && (
					<CloseList label={t("outcome.proof.becameTrue")} testId="proof-close-became-true">
						{becameTrue.map((criterion) => (
							<li className="flex items-start gap-2 text-sm text-wd-primary" key={criterion.criterionId}>
								<StateMark className="mt-1 text-good" tone="good" />
								<span>{criterion.text}</span>
							</li>
						))}
					</CloseList>
				)}
				{didNotHappen.length > 0 && (
					<CloseList label={t("outcome.proof.didNotHappen")} testId="proof-close-did-not">
						{didNotHappen.map((criterion) => (
							<li className="text-sm text-wd-secondary" key={criterion.criterionId}>
								{criterion.text}
								{criterion.gap ? <> <span>{criterion.gap}</span></> : null}
							</li>
						))}
					</CloseList>
				)}
				{result.uncertainty.length > 0 && (
					<CloseList label={t("outcome.proof.limitations")} testId="outcome-result-uncertainty">
						{result.uncertainty.map((item) => <li className="text-sm text-wd-secondary" key={item}>{item}</li>)}
					</CloseList>
				)}
				<FactList facts={[{ label: t("outcome.proof.nextAction"), value: result.nextSafeAction }]} />

				<div className="flex flex-col gap-3 rounded-md bg-wd-inset p-3">
					<p className="text-xs text-wd-secondary">{t("outcome.proof.decisionBoundary")}</p>
					<div className="flex flex-col gap-1">
						<Label htmlFor="proof-decision-summary">{t("outcome.proof.decisionSummary")}</Label>
						<Input className="bg-wd-surface" data-testid="proof-decision-summary" id="proof-decision-summary" onChange={(event) => setDecisionSummary(event.target.value)} value={decisionSummary} />
					</div>
					<div className="grid gap-3 sm:grid-cols-2">
						<label className="flex flex-col gap-1 text-xs text-wd-secondary">
							{t("outcome.proof.disposition")}
							<select className={cn(FIELD_SELECT, "bg-wd-surface")} onChange={(event) => setResourceDisposition(event.target.value as typeof resourceDisposition)} value={resourceDisposition}>
								<option value="retain">{t("outcome.proof.dispositionRetain")}</option>
								<option value="cleanup_later">{t("outcome.proof.dispositionCleanup")}</option>
								<option value="not_applicable">{t("outcome.proof.dispositionNA")}</option>
							</select>
						</label>
						{proof.status !== "accepted" && proof.status !== "ready_for_acceptance" && <span />}
					</div>
					<ReentryFields targetId={reentryTargetId} targetType={reentryTargetType} targets={proof.reentryTargets ?? []} onTargetId={setReentryTargetId} onTargetType={(next) => { setReentryTargetType(next); setReentryTargetId(""); }} />
				</div>

				<div className="flex flex-wrap items-center justify-between gap-2">
					<div className="flex items-center gap-2">
						{!accepted && <Button data-testid="proof-request-rework" disabled={pending || summaryMissing || !reentryReady} onClick={() => void decide("request_rework")} size="sm" variant="ghost"><CircleAlert aria-hidden="true" className="size-3.5" />{t("outcome.proof.requestRework")}</Button>}
					</div>
					<div className="flex min-w-0 items-center gap-2">
						{proof.status === "ready_for_acceptance" && acceptReason ? <span className="text-xs text-wd-secondary" id="proof-accept-reason">{acceptReason}</span> : null}
						{accepted && reentryReason ? <span className="text-xs text-wd-secondary" id="proof-reopen-reason">{reentryReason}</span> : null}
						{proof.status === "ready_for_acceptance" && <Button aria-describedby={acceptReason ? "proof-accept-reason" : undefined} data-testid="proof-accept" disabled={pending || summaryMissing} onClick={() => void decide("accept")} size="sm"><CheckCircle2 aria-hidden="true" className="size-3.5" />{t("outcome.proof.accept")}</Button>}
						{accepted && <Button aria-describedby={reentryReason ? "proof-reopen-reason" : undefined} data-testid="proof-reopen" disabled={pending || summaryMissing || !reentryReady} onClick={() => void decide("reopen")} size="sm"><RotateCcw aria-hidden="true" className="size-3.5" />{t("outcome.proof.reopen")}</Button>}
					</div>
				</div>
				<p className="text-xs text-wd-tertiary">{t("outcome.proof.acceptanceOwnerOnly")}</p>
			</section>

			{failure && <ProofFailure message={failure.message} onRetry={proofQuery.refetch} />}
		</div>
	);
}

const FIELD_SELECT = "h-control-form w-full min-w-0 rounded-md bg-wd-inset px-3 text-sm text-wd-primary outline-none focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-(--border-focus) disabled:text-wd-disabled";

function CloseList({ label, testId, children }: { label: string; testId?: string; children: ReactNode }) {
	return (
		<div className="flex flex-col gap-1.5" data-testid={testId}>
			<p className="text-sm font-medium text-wd-primary">{label}</p>
			<ul className="flex flex-col gap-1">{children}</ul>
		</div>
	);
}

/** Verification results in the shared verdict vocabulary. */
function verificationVerdict(result: string): EvidenceVerdict {
	if (result === "passed") return "verified";
	if (result === "failed") return "failed";
	return "unconfirmed";
}

/** A deterministic check's verdict: uncertainty blocks a verdict, never a pass. */
function checkVerdict(check: { verdict: string; uncertain: boolean }): EvidenceVerdict {
	if (check.uncertain) return "unconfirmed";
	if (check.verdict === "passed" || check.verdict === "pass") return "verified";
	if (check.verdict === "failed" || check.verdict === "fail") return "failed";
	return "unconfirmed";
}

/** A criterion's verdict from what the proof read actually carries. */
function criterionVerdict(criterion: OutcomeProofRecord["criteria"][number]): EvidenceVerdict | undefined {
	if (criterion.ready) return "verified";
	const latest = criterion.verifications[criterion.verifications.length - 1];
	if (!latest) return "notRun";
	if (latest.result === "failed") return "failed";
	if (latest.result === "inconclusive" || latest.result === "exception") return "unconfirmed";
	return undefined;
}

const VERDICT_LABEL_KEYS: Record<EvidenceVerdict, MessageKey> = {
	verified: "outcome.verdict.verified",
	notRun: "outcome.verdict.notRun",
	stale: "outcome.verdict.stale",
	failed: "outcome.verdict.failed",
	unconfirmed: "outcome.verdict.unconfirmed",
	pending: "outcome.proof.gap",
};

function ResultSummary({ proof }: { proof: OutcomeProofRecord }) {
	const { t } = useTranslation();
	const evidenceCount = proof.criteria.reduce((total, criterion) => total + criterion.evidence.length, 0);
	const verificationCount = proof.criteria.reduce((total, criterion) => total + criterion.verifications.length, 0);
	const passedCount = proof.criteria.reduce((total, criterion) => total + criterion.verifications.filter((run) => run.result === "passed").length, 0);
	const result = proof.result ?? { artifacts: [], checks: [], changes: [], uncertainty: [], nextSafeAction: proof.nextAction };
	return (
		<section className="flex flex-col gap-4 rounded-lg hairline border-wd-hairline bg-wd-surface p-5" data-testid="outcome-result-summary">
			<h3 className="text-sm font-medium text-wd-primary">{t("outcome.proof.resultSummary")}</h3>
			<dl className="grid gap-3 sm:grid-cols-3">
				<SummaryFigure label={t("outcome.proof.criteriaSummary")} value={`${proof.criteria.filter((criterion) => criterion.ready).length}/${proof.criteria.length}`} unit={t("outcome.proof.criteriaReady")} />
				<SummaryFigure label={t("outcome.proof.evidenceSummaryLabel")} value={String(evidenceCount)} unit={t("outcome.proof.evidenceRecorded")} />
				<SummaryFigure label={t("outcome.proof.verificationSummaryLabel")} value={`${passedCount}/${verificationCount}`} unit={t("outcome.proof.verificationsPassed")} />
			</dl>
			{result.artifacts.length > 0 && <SummaryList label={t("outcome.proof.artifact")} testId="outcome-result-artifacts">{result.artifacts.map((artifact) => <li key={`${artifact.revision}-${artifact.digest}`}>{artifact.sourceRef} · <span className="tabular-nums">{artifact.revision}</span></li>)}</SummaryList>}
			{result.changes.length > 0 && (
				<SummaryList label={t("outcome.proof.whatChanged")} testId="outcome-result-changes" footer={result.changes.some((change) => change.truncated || change.files.length > 10) ? t("outcome.proof.changesMore") : undefined}>
					{result.changes.flatMap((change) => change.files.slice(0, 10).map((file) => <li key={`${change.attemptId}-${file.path}`}><span className="font-mono">{file.path}</span> · {file.changeKind}</li>))}
				</SummaryList>
			)}
			{result.checks.length > 0 && (
				<div className="flex flex-col" data-testid="outcome-result-checks">
					<p className="text-sm font-medium text-wd-primary">{t("outcome.proof.verification")}</p>
					<ul className="flex flex-col">
						{result.checks.map((check) => {
							const verdict = checkVerdict(check);
							return (
								<EvidenceRow
									assertion={<code className="text-xs">{check.command}</code>}
									key={`${check.criterionId}-${check.artifactRevision}-${check.command}`}
									limitation={check.uncertain ? "uncertainty blocks a verdict" : undefined}
									provenance={<span className="tabular-nums">{check.artifactRevision}{check.detail ? ` · ${check.detail}` : ""}</span>}
									verdict={verdict}
									verdictLabel={t(VERDICT_LABEL_KEYS[verdict])}
								/>
							);
						})}
					</ul>
				</div>
			)}
		</section>
	);
}

function SummaryFigure({ label, value, unit }: { label: string; value: string; unit: string }) {
	return (
		<div className="flex flex-col gap-0.5">
			<dt className="text-xs text-wd-secondary">{label}</dt>
			<dd className="text-sm text-wd-primary"><span className="tabular-nums">{value}</span> {unit}</dd>
		</div>
	);
}

function SummaryList({ label, testId, footer, children }: { label: string; testId?: string; footer?: string; children: ReactNode }) {
	return (
		<div className="flex flex-col gap-1" data-testid={testId}>
			<p className="text-xs text-wd-secondary">{label}</p>
			<ul className="flex list-disc flex-col gap-0.5 pl-4 text-xs text-wd-primary">{children}</ul>
			{footer ? <p className="text-xs text-wd-tertiary">{footer}</p> : null}
		</div>
	);
}

function CriterionCard({ criterion, evidenceDraft, onEvidenceDraft, onSubmitEvidence, verificationDraft, onVerificationDraft, onSubmitVerification, pending }: {
	criterion: OutcomeProofRecord["criteria"][number];
	evidenceDraft: EvidenceDraft;
	onEvidenceDraft: (draft: EvidenceDraft) => void;
	onSubmitEvidence: () => void;
	verificationDraft: VerificationDraft;
	onVerificationDraft: (draft: VerificationDraft) => void;
	onSubmitVerification: () => void;
	pending: boolean;
}) {
	const { t } = useTranslation();
	const needsProducerRef = verificationDraft.independenceClass === "producer_self_check" || verificationDraft.independenceClass === "separate_session";
	const needsProviders = verificationDraft.independenceClass === "cross_provider";
	const verificationReady = criterion.evidence.length > 0
		&& Boolean(verificationDraft.method.trim())
		&& Boolean(verificationDraft.verifierRef.trim())
		&& (!needsProducerRef || Boolean(verificationDraft.producerRef.trim()))
		&& (!needsProviders || Boolean(verificationDraft.producerProvider.trim()) && Boolean(verificationDraft.verifierProvider.trim()))
		&& (verificationDraft.result !== "exception" || Boolean(verificationDraft.detail.trim()));
	const verdict = criterionVerdict(criterion);

	return (
		<section className="flex flex-col gap-4 rounded-lg hairline border-wd-hairline bg-wd-surface p-5" data-testid={`proof-criterion-${criterion.criterionId}`}>
			<div className="flex items-start justify-between gap-3">
				<div className="flex min-w-0 flex-col gap-0.5">
					<p className="text-xs tabular-nums text-wd-secondary">{t("outcome.proof.criterion", { position: criterion.position })}</p>
					<h3 className="text-sm font-medium text-wd-primary">{criterion.text}</h3>
				</div>
				{verdict
					? <StateBadge label={criterion.ready ? t("outcome.proof.ready") : t(VERDICT_LABEL_KEYS[verdict])} tone={evidenceVerdictTone(verdict)} />
					: <StateBadge label={t("outcome.proof.gap")} tone="neutral" />}
			</div>
			{criterion.gap && <p className="text-xs text-wd-secondary">{criterion.gap}</p>}
			<div className="grid gap-5 lg:grid-cols-2">
				<div className="flex flex-col gap-2">
					<h4 className="text-sm font-medium text-wd-primary">{t("outcome.proof.evidence")}</h4>
					{criterion.evidence.length > 0 && (
						<ul className="flex flex-col">
							{criterion.evidence.map((item) => (
								<li className="flex items-start gap-3 hairline-b border-wd-hairline py-2.5 last:border-b-0" data-testid={`proof-evidence-${item.id}`} key={item.id}>
									<div className="flex min-w-0 flex-1 flex-col gap-1">
										<span className="break-words text-sm text-wd-primary">{item.summary}</span>
										<span className="break-all text-xs text-wd-tertiary">{item.sourceType} · {item.sourceRef} · {item.producerType}:{item.producerRef} · <span className="tabular-nums">{formatTimeCompact(item.createdAt)}</span></span>
									</div>
									<StateBadge label={item.kind === "contradicting" ? t("outcome.proof.contradicting") : t("outcome.proof.supporting")} tone={item.kind === "contradicting" ? "caution" : "neutral"} />
								</li>
							))}
						</ul>
					)}
					<div className="flex flex-col gap-2">
						<Input aria-label={t("outcome.proof.evidenceSummary")} data-testid={`proof-evidence-summary-${criterion.criterionId}`} onChange={(event) => onEvidenceDraft({ ...evidenceDraft, summary: event.target.value })} placeholder={t("outcome.proof.evidenceSummary")} value={evidenceDraft.summary} />
						<Input aria-label={t("outcome.proof.sourceRef")} data-testid={`proof-evidence-source-${criterion.criterionId}`} onChange={(event) => onEvidenceDraft({ ...evidenceDraft, sourceRef: event.target.value })} placeholder={t("outcome.proof.sourceRef")} value={evidenceDraft.sourceRef} />
						<div className="grid grid-cols-2 gap-2">
							<select aria-label={t("outcome.proof.evidenceKind")} className={FIELD_SELECT} onChange={(event) => onEvidenceDraft({ ...evidenceDraft, kind: event.target.value as EvidenceDraft["kind"] })} value={evidenceDraft.kind}><option value="supporting">{t("outcome.proof.supporting")}</option><option value="contradicting">{t("outcome.proof.contradicting")}</option></select>
							<select aria-label={t("outcome.proof.sourceType")} className={FIELD_SELECT} onChange={(event) => onEvidenceDraft({ ...evidenceDraft, sourceType: event.target.value as EvidenceDraft["sourceType"] })} value={evidenceDraft.sourceType}><option value="owner_walkthrough">{t("outcome.proof.ownerWalkthrough")}</option><option value="deterministic_check">{t("outcome.proof.deterministic")}</option><option value="artifact">{t("outcome.proof.artifact")}</option><option value="provider_output">{t("outcome.proof.providerOutput")}</option></select>
						</div>
						<div className="grid grid-cols-2 gap-2">
							<select aria-label={t("outcome.proof.producerType")} className={FIELD_SELECT} onChange={(event) => onEvidenceDraft({ ...evidenceDraft, producerType: event.target.value as EvidenceDraft["producerType"] })} value={evidenceDraft.producerType}><option value="user">{t("outcome.proof.producerUser")}</option><option value="tool">{t("outcome.proof.producerTool")}</option><option value="provider">{t("outcome.proof.producerProvider")}</option></select>
							<Input aria-label={t("outcome.proof.producerRef")} onChange={(event) => onEvidenceDraft({ ...evidenceDraft, producerRef: event.target.value })} placeholder={t("outcome.proof.producerRef")} value={evidenceDraft.producerRef} />
						</div>
						<Button className="self-start" data-testid={`proof-add-evidence-${criterion.criterionId}`} disabled={pending || !evidenceDraft.summary.trim() || !evidenceDraft.sourceRef.trim() || !evidenceDraft.producerRef.trim()} onClick={onSubmitEvidence} size="sm" variant="secondary">{t("outcome.proof.addEvidence")}</Button>
					</div>
				</div>
				<div className="flex flex-col gap-2">
					<h4 className="text-sm font-medium text-wd-primary">{t("outcome.proof.verification")}</h4>
					{criterion.verifications.length > 0 && (
						<ul className="flex flex-col">
							{criterion.verifications.map((run) => {
								const runVerdict = verificationVerdict(run.result);
								return (
									<EvidenceRow
										assertion={run.method}
										className="py-2.5"
										data-testid={`proof-verification-${run.id}`}
										key={run.id}
										limitation={run.detail}
										provenance={<>{t(INDEPENDENCE_LABEL_KEYS[run.independenceClass] ?? "outcome.proof.ownerWalkthrough")} · {run.verifierRef} · <span className="tabular-nums">{formatTimeCompact(run.createdAt)}</span></>}
										verdict={runVerdict}
										verdictLabel={t(VERDICT_LABEL_KEYS[runVerdict])}
									/>
								);
							})}
						</ul>
					)}
					<div className="flex flex-col gap-2">
						<Input aria-label={t("outcome.proof.verificationMethod")} data-testid={`proof-verification-method-${criterion.criterionId}`} onChange={(event) => onVerificationDraft({ ...verificationDraft, method: event.target.value })} placeholder={t("outcome.proof.verificationMethod")} value={verificationDraft.method} />
						<div className="grid grid-cols-2 gap-2">
							<select aria-label={t("outcome.proof.independence")} className={FIELD_SELECT} onChange={(event) => onVerificationDraft({ ...verificationDraft, independenceClass: event.target.value as VerificationDraft["independenceClass"] })} value={verificationDraft.independenceClass}><option value="owner_walkthrough">{t("outcome.proof.ownerWalkthrough")}</option><option value="deterministic">{t("outcome.proof.deterministic")}</option><option value="producer_self_check">{t("outcome.proof.producerSelfCheck")}</option><option value="separate_session">{t("outcome.proof.separateSession")}</option><option value="cross_provider">{t("outcome.proof.crossProvider")}</option></select>
							<select aria-label={t("outcome.proof.result")} className={FIELD_SELECT} onChange={(event) => onVerificationDraft({ ...verificationDraft, result: event.target.value as VerificationDraft["result"] })} value={verificationDraft.result}><option value="passed">{t("outcome.proof.passed")}</option><option value="failed">{t("outcome.proof.failed")}</option><option value="inconclusive">{t("outcome.proof.inconclusive")}</option><option value="exception">{t("outcome.proof.exception")}</option></select>
						</div>
						{needsProducerRef && <Input aria-label={t("outcome.proof.verificationProducerRef")} onChange={(event) => onVerificationDraft({ ...verificationDraft, producerRef: event.target.value })} placeholder={t("outcome.proof.verificationProducerRef")} value={verificationDraft.producerRef} />}
						{needsProviders && <div className="grid grid-cols-2 gap-2"><Input aria-label={t("outcome.proof.producerProviderRef")} onChange={(event) => onVerificationDraft({ ...verificationDraft, producerProvider: event.target.value })} placeholder={t("outcome.proof.producerProviderRef")} value={verificationDraft.producerProvider} /><Input aria-label={t("outcome.proof.verifierProviderRef")} onChange={(event) => onVerificationDraft({ ...verificationDraft, verifierProvider: event.target.value })} placeholder={t("outcome.proof.verifierProviderRef")} value={verificationDraft.verifierProvider} /></div>}
						<Input aria-label={t("outcome.proof.verifierRef")} onChange={(event) => onVerificationDraft({ ...verificationDraft, verifierRef: event.target.value })} placeholder={t("outcome.proof.verifierRef")} value={verificationDraft.verifierRef} />
						{verificationDraft.result === "exception" && <Input aria-label={t("outcome.proof.exceptionDetail")} onChange={(event) => onVerificationDraft({ ...verificationDraft, detail: event.target.value })} placeholder={t("outcome.proof.exceptionDetail")} value={verificationDraft.detail} />}
						<Button className="self-start" data-testid={`proof-add-verification-${criterion.criterionId}`} disabled={pending || !verificationReady} onClick={onSubmitVerification} size="sm" variant="secondary">{t("outcome.proof.addVerification")}</Button>
					</div>
				</div>
			</div>
		</section>
	);
}

type ReentryTarget = NonNullable<OutcomeProofRecord["reentryTargets"]>[number];

function ReentryFields({ targetId, targetType, targets, onTargetId, onTargetType }: { targetId: string; targetType: "attempt" | "work_unit" | "plan" | "contract"; targets: ReentryTarget[]; onTargetId: (value: string) => void; onTargetType: (value: "attempt" | "work_unit" | "plan" | "contract") => void }) {
	const { t } = useTranslation();
	const candidates = targets.filter((target) => target.targetType === targetType);
	return (
		<div className="grid gap-3 sm:grid-cols-2">
			<label className="flex flex-col gap-1 text-xs text-wd-secondary">
				{t("outcome.proof.reentryTarget")}
				<select className={cn(FIELD_SELECT, "bg-wd-surface")} onChange={(event) => onTargetType(event.target.value as typeof targetType)} value={targetType}><option value="contract">{t("outcome.proof.targetContract")}</option><option value="plan">{t("outcome.proof.targetPlan")}</option><option value="work_unit">{t("outcome.proof.targetWorkUnit")}</option><option value="attempt">{t("outcome.proof.targetAttempt")}</option></select>
			</label>
			{targetType === "contract" ? <p className="self-end pb-2.5 text-xs text-wd-secondary">{t("outcome.proof.targetContract")}</p> : <label className="flex flex-col gap-1 text-xs text-wd-secondary">
				{t("outcome.proof.reentryId")}
				<select aria-label={t("outcome.proof.reentryId")} className={cn(FIELD_SELECT, "bg-wd-surface")} onChange={(event) => onTargetId(event.target.value)} value={targetId}>
					<option value="">{t("outcome.proof.reentryIdRequired")}</option>
					{candidates.map((target) => <option key={target.targetId} value={target.targetId}>{target.label}</option>)}
				</select>
			</label>}
		</div>
	);
}

function ProofFailure({ message, onRetry }: { message: string; onRetry: () => void }) {
	const { t } = useTranslation();
	return (
		<div className="flex max-w-xl flex-col gap-1 rounded-lg hairline border-wd-hairline bg-wd-surface p-4" role="alert">
			<h3 className="flex items-center gap-1.5 text-sm font-medium text-wd-primary"><StateMark className="text-critical" tone="critical" />{t("outcome.proof.errorTitle")}</h3>
			<p className="text-sm text-wd-secondary">{message}</p>
			<Button className="mt-2 self-start" onClick={onRetry} size="sm" variant="outline">{t("outcome.understand.retry")}</Button>
		</div>
	);
}

async function sha256(value: string): Promise<string> {
	const bytes = new TextEncoder().encode(value);
	const digest = await crypto.subtle.digest("SHA-256", bytes);
	return Array.from(new Uint8Array(digest), (byte) => byte.toString(16).padStart(2, "0")).join("");
}
