import { CheckCircle2, CircleAlert, Loader2, RotateCcw } from "lucide-react";
import { useState } from "react";
import { useTranslation } from "react-i18next";

import type { MessageKey } from "../../i18n/messages";
import {
	useDecideOutcomeAcceptance,
	useOutcomeProof,
	useRecordOutcomeEvidence,
	useRecordOutcomeVerification,
	type OutcomeProofRecord,
} from "../../hooks/useOutcome";
import { Badge } from "../ui/badge";
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
		return <div className="flex items-center gap-2 text-muted-foreground text-sm"><Loader2 aria-hidden="true" className="size-4 animate-spin" />{t("outcome.proof.loading")}</div>;
	}
	if (!proof) {
		return <ProofFailure message={failure?.message ?? t("outcome.proof.unavailable")} onRetry={proofQuery.refetch} />;
	}

	return (
		<div className="flex max-w-3xl flex-col gap-5" data-testid="outcome-prove-close-surface">
			<header>
				<div className="flex flex-wrap items-center gap-2">
					<h2 className="text-base font-medium">{t("outcome.proof.heading")}</h2>
					<Badge variant={proof.status === "accepted" ? "success" : proof.status === "ready_for_acceptance" ? "accent" : "outline"}>
						{t(STATUS_LABEL_KEYS[proof.status] ?? "outcome.proof.statusActive")}
					</Badge>
				</div>
				<p className="mt-1 text-muted-foreground text-xs">{t("outcome.proof.binding", { revision: proof.contractRevision.number })}</p>
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

			<section className="rounded-md border border-border p-4" data-testid="proof-decision-card">
				<h3 className="text-sm font-medium">{proof.status === "accepted" ? t("outcome.proof.reopenTitle") : t("outcome.proof.decisionTitle")}</h3>
				<p className="mt-1 text-muted-foreground text-sm">{t("outcome.proof.decisionBoundary")}</p>
				<Label className="mt-3" htmlFor="proof-decision-summary">{t("outcome.proof.decisionSummary")}</Label>
				<Input data-testid="proof-decision-summary" id="proof-decision-summary" onChange={(event) => setDecisionSummary(event.target.value)} value={decisionSummary} />
				<div className="mt-3 grid gap-3 sm:grid-cols-2">
					<label className="flex flex-col gap-1 text-xs text-muted-foreground">
						{t("outcome.proof.disposition")}
						<select className="h-9 rounded-md border border-input bg-background px-2 text-foreground" onChange={(event) => setResourceDisposition(event.target.value as typeof resourceDisposition)} value={resourceDisposition}>
							<option value="retain">{t("outcome.proof.dispositionRetain")}</option>
							<option value="cleanup_later">{t("outcome.proof.dispositionCleanup")}</option>
							<option value="not_applicable">{t("outcome.proof.dispositionNA")}</option>
						</select>
					</label>
					{proof.status !== "accepted" && proof.status !== "ready_for_acceptance" && <span />}
				</div>
				<ReentryFields targetId={reentryTargetId} targetType={reentryTargetType} onTargetId={setReentryTargetId} onTargetType={setReentryTargetType} />
				<div className="mt-4 flex flex-wrap gap-2">
					{proof.status === "ready_for_acceptance" && <Button data-testid="proof-accept" disabled={pending || !decisionSummary.trim()} onClick={() => void decide("accept")}><CheckCircle2 aria-hidden="true" className="size-3.5" />{t("outcome.proof.accept")}</Button>}
					{proof.status !== "accepted" && <Button data-testid="proof-request-rework" disabled={pending || !decisionSummary.trim() || !reentryReady} onClick={() => void decide("request_rework")} variant="outline"><CircleAlert aria-hidden="true" className="size-3.5" />{t("outcome.proof.requestRework")}</Button>}
					{proof.status === "accepted" && <Button data-testid="proof-reopen" disabled={pending || !decisionSummary.trim() || !reentryReady} onClick={() => void decide("reopen")} variant="outline"><RotateCcw aria-hidden="true" className="size-3.5" />{t("outcome.proof.reopen")}</Button>}
				</div>
			</section>

			{failure && <ProofFailure message={failure.message} onRetry={proofQuery.refetch} />}
		</div>
	);
}

function ResultSummary({ proof }: { proof: OutcomeProofRecord }) {
	const { t } = useTranslation();
	const evidenceCount = proof.criteria.reduce((total, criterion) => total + criterion.evidence.length, 0);
	const verificationCount = proof.criteria.reduce((total, criterion) => total + criterion.verifications.length, 0);
	const passedCount = proof.criteria.reduce((total, criterion) => total + criterion.verifications.filter((run) => run.result === "passed").length, 0);
	const gaps = proof.criteria.filter((criterion) => criterion.gap).map((criterion) => criterion.gap as string);
	const result = proof.result ?? { artifacts: [], checks: [], uncertainty: [], nextSafeAction: proof.nextAction };
	return (
		<section className="grid gap-3 rounded-md border border-border bg-card/50 p-4 sm:grid-cols-3" data-testid="outcome-result-summary">
			<div className="sm:col-span-3">
				<h3 className="text-sm font-medium">{t("outcome.proof.resultSummary")}</h3>
			</div>
			<div><p className="text-xs uppercase tracking-wide text-muted-foreground">{t("outcome.proof.criteriaSummary")}</p><p className="mt-1 text-sm">{proof.criteria.filter((criterion) => criterion.ready).length}/{proof.criteria.length} {t("outcome.proof.criteriaReady")}</p></div>
			<div><p className="text-xs uppercase tracking-wide text-muted-foreground">{t("outcome.proof.evidenceSummaryLabel")}</p><p className="mt-1 text-sm">{evidenceCount} {t("outcome.proof.evidenceRecorded")}</p></div>
			<div><p className="text-xs uppercase tracking-wide text-muted-foreground">{t("outcome.proof.verificationSummaryLabel")}</p><p className="mt-1 text-sm">{passedCount}/{verificationCount} {t("outcome.proof.verificationsPassed")}</p></div>
			{gaps.length > 0 && <div className="sm:col-span-3"><p className="text-xs uppercase tracking-wide text-warning">{t("outcome.proof.limitations")}</p><ul className="mt-1 list-disc pl-4 text-xs text-muted-foreground">{gaps.map((gap) => <li key={gap}>{gap}</li>)}</ul></div>}
			{result.artifacts.length > 0 && <div className="sm:col-span-3" data-testid="outcome-result-artifacts"><p className="text-xs uppercase tracking-wide text-muted-foreground">{t("outcome.proof.artifact")}</p><ul className="mt-1 list-disc pl-4 text-xs text-muted-foreground">{result.artifacts.map((artifact) => <li key={`${artifact.revision}-${artifact.digest}`}>{artifact.sourceRef} · {artifact.revision}</li>)}</ul></div>}
			{result.checks.length > 0 && <div className="sm:col-span-3" data-testid="outcome-result-checks"><p className="text-xs uppercase tracking-wide text-muted-foreground">{t("outcome.proof.verification")}</p><ul className="mt-1 list-disc pl-4 text-xs text-muted-foreground">{result.checks.map((check) => <li key={`${check.criterionId}-${check.artifactRevision}-${check.command}`}>{check.command} · {check.verdict || "unconfirmed"}{check.uncertain ? " · uncertainty blocks a verdict" : ""}</li>)}</ul></div>}
			{result.uncertainty.length > 0 && <div className="sm:col-span-3" data-testid="outcome-result-uncertainty"><p className="text-xs uppercase tracking-wide text-warning">{t("outcome.proof.limitations")}</p><ul className="mt-1 list-disc pl-4 text-xs text-muted-foreground">{result.uncertainty.map((item) => <li key={item}>{item}</li>)}</ul></div>}
			<div className="sm:col-span-3"><p className="text-xs uppercase tracking-wide text-muted-foreground">{t("outcome.proof.nextAction")}</p><p className="mt-1 text-sm">{result.nextSafeAction}</p></div>
			<p className="text-xs text-muted-foreground sm:col-span-3">{t("outcome.proof.acceptanceOwnerOnly")}</p>
		</section>
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

	return (
		<section className="rounded-md border border-border p-4" data-testid={`proof-criterion-${criterion.criterionId}`}>
			<div className="flex items-start justify-between gap-3">
				<div>
					<p className="text-xs text-muted-foreground">{t("outcome.proof.criterion", { position: criterion.position })}</p>
					<h3 className="text-sm font-medium">{criterion.text}</h3>
				</div>
				<Badge variant={criterion.ready ? "success" : "outline"}>{criterion.ready ? t("outcome.proof.ready") : t("outcome.proof.gap")}</Badge>
			</div>
			{criterion.gap && <p className="mt-2 text-warning text-xs">{criterion.gap}</p>}
			<div className="mt-4 grid gap-4 lg:grid-cols-2">
				<div>
					<h4 className="text-xs font-medium uppercase tracking-wide text-muted-foreground">{t("outcome.proof.evidence")}</h4>
					<div className="mt-2 flex flex-col gap-2">
						{criterion.evidence.map((item) => (
							<div className="rounded border border-border p-2 text-xs" data-testid={`proof-evidence-${item.id}`} key={item.id}>
								<div className="flex justify-between gap-2"><span>{item.summary}</span><Badge variant="outline">{item.kind}</Badge></div>
								<p className="mt-1 text-muted-foreground">{item.sourceType} · {item.sourceRef} · {item.producerType}:{item.producerRef}</p>
							</div>
						))}
					</div>
					<div className="mt-3 flex flex-col gap-2">
						<Input aria-label={t("outcome.proof.evidenceSummary")} data-testid={`proof-evidence-summary-${criterion.criterionId}`} onChange={(event) => onEvidenceDraft({ ...evidenceDraft, summary: event.target.value })} placeholder={t("outcome.proof.evidenceSummary")} value={evidenceDraft.summary} />
						<Input aria-label={t("outcome.proof.sourceRef")} data-testid={`proof-evidence-source-${criterion.criterionId}`} onChange={(event) => onEvidenceDraft({ ...evidenceDraft, sourceRef: event.target.value })} placeholder={t("outcome.proof.sourceRef")} value={evidenceDraft.sourceRef} />
						<div className="grid grid-cols-2 gap-2">
							<select aria-label={t("outcome.proof.evidenceKind")} className="h-9 rounded-md border border-input bg-background px-2 text-xs" onChange={(event) => onEvidenceDraft({ ...evidenceDraft, kind: event.target.value as EvidenceDraft["kind"] })} value={evidenceDraft.kind}><option value="supporting">{t("outcome.proof.supporting")}</option><option value="contradicting">{t("outcome.proof.contradicting")}</option></select>
							<select aria-label={t("outcome.proof.sourceType")} className="h-9 rounded-md border border-input bg-background px-2 text-xs" onChange={(event) => onEvidenceDraft({ ...evidenceDraft, sourceType: event.target.value as EvidenceDraft["sourceType"] })} value={evidenceDraft.sourceType}><option value="owner_walkthrough">{t("outcome.proof.ownerWalkthrough")}</option><option value="deterministic_check">{t("outcome.proof.deterministic")}</option><option value="artifact">{t("outcome.proof.artifact")}</option><option value="provider_output">{t("outcome.proof.providerOutput")}</option></select>
						</div>
						<div className="grid grid-cols-2 gap-2">
							<select aria-label={t("outcome.proof.producerType")} className="h-9 rounded-md border border-input bg-background px-2 text-xs" onChange={(event) => onEvidenceDraft({ ...evidenceDraft, producerType: event.target.value as EvidenceDraft["producerType"] })} value={evidenceDraft.producerType}><option value="user">{t("outcome.proof.producerUser")}</option><option value="tool">{t("outcome.proof.producerTool")}</option><option value="provider">{t("outcome.proof.producerProvider")}</option></select>
							<Input aria-label={t("outcome.proof.producerRef")} onChange={(event) => onEvidenceDraft({ ...evidenceDraft, producerRef: event.target.value })} placeholder={t("outcome.proof.producerRef")} value={evidenceDraft.producerRef} />
						</div>
						<Button data-testid={`proof-add-evidence-${criterion.criterionId}`} disabled={pending || !evidenceDraft.summary.trim() || !evidenceDraft.sourceRef.trim() || !evidenceDraft.producerRef.trim()} onClick={onSubmitEvidence} size="sm" variant="outline">{t("outcome.proof.addEvidence")}</Button>
					</div>
				</div>
				<div>
					<h4 className="text-xs font-medium uppercase tracking-wide text-muted-foreground">{t("outcome.proof.verification")}</h4>
					<div className="mt-2 flex flex-col gap-2">
						{criterion.verifications.map((run) => (
							<div className="rounded border border-border p-2 text-xs" data-testid={`proof-verification-${run.id}`} key={run.id}>
								<div className="flex justify-between gap-2"><span>{run.method}</span><Badge variant={run.result === "passed" ? "success" : "outline"}>{run.result}</Badge></div>
								<p className="mt-1 text-muted-foreground">{t(INDEPENDENCE_LABEL_KEYS[run.independenceClass] ?? "outcome.proof.ownerWalkthrough")} · {run.verifierRef}</p>
							</div>
						))}
					</div>
					<div className="mt-3 flex flex-col gap-2">
						<Input aria-label={t("outcome.proof.verificationMethod")} data-testid={`proof-verification-method-${criterion.criterionId}`} onChange={(event) => onVerificationDraft({ ...verificationDraft, method: event.target.value })} placeholder={t("outcome.proof.verificationMethod")} value={verificationDraft.method} />
						<div className="grid grid-cols-2 gap-2">
							<select aria-label={t("outcome.proof.independence")} className="h-9 rounded-md border border-input bg-background px-2 text-xs" onChange={(event) => onVerificationDraft({ ...verificationDraft, independenceClass: event.target.value as VerificationDraft["independenceClass"] })} value={verificationDraft.independenceClass}><option value="owner_walkthrough">{t("outcome.proof.ownerWalkthrough")}</option><option value="deterministic">{t("outcome.proof.deterministic")}</option><option value="producer_self_check">{t("outcome.proof.producerSelfCheck")}</option><option value="separate_session">{t("outcome.proof.separateSession")}</option><option value="cross_provider">{t("outcome.proof.crossProvider")}</option></select>
							<select aria-label={t("outcome.proof.result")} className="h-9 rounded-md border border-input bg-background px-2 text-xs" onChange={(event) => onVerificationDraft({ ...verificationDraft, result: event.target.value as VerificationDraft["result"] })} value={verificationDraft.result}><option value="passed">{t("outcome.proof.passed")}</option><option value="failed">{t("outcome.proof.failed")}</option><option value="inconclusive">{t("outcome.proof.inconclusive")}</option><option value="exception">{t("outcome.proof.exception")}</option></select>
						</div>
						{needsProducerRef && <Input aria-label={t("outcome.proof.verificationProducerRef")} onChange={(event) => onVerificationDraft({ ...verificationDraft, producerRef: event.target.value })} placeholder={t("outcome.proof.verificationProducerRef")} value={verificationDraft.producerRef} />}
						{needsProviders && <div className="grid grid-cols-2 gap-2"><Input aria-label={t("outcome.proof.producerProviderRef")} onChange={(event) => onVerificationDraft({ ...verificationDraft, producerProvider: event.target.value })} placeholder={t("outcome.proof.producerProviderRef")} value={verificationDraft.producerProvider} /><Input aria-label={t("outcome.proof.verifierProviderRef")} onChange={(event) => onVerificationDraft({ ...verificationDraft, verifierProvider: event.target.value })} placeholder={t("outcome.proof.verifierProviderRef")} value={verificationDraft.verifierProvider} /></div>}
						<Input aria-label={t("outcome.proof.verifierRef")} onChange={(event) => onVerificationDraft({ ...verificationDraft, verifierRef: event.target.value })} placeholder={t("outcome.proof.verifierRef")} value={verificationDraft.verifierRef} />
						{verificationDraft.result === "exception" && <Input aria-label={t("outcome.proof.exceptionDetail")} onChange={(event) => onVerificationDraft({ ...verificationDraft, detail: event.target.value })} placeholder={t("outcome.proof.exceptionDetail")} value={verificationDraft.detail} />}
						<Button data-testid={`proof-add-verification-${criterion.criterionId}`} disabled={pending || !verificationReady} onClick={onSubmitVerification} size="sm" variant="outline">{t("outcome.proof.addVerification")}</Button>
					</div>
				</div>
			</div>
		</section>
	);
}

function ReentryFields({ targetId, targetType, onTargetId, onTargetType }: { targetId: string; targetType: "attempt" | "work_unit" | "plan" | "contract"; onTargetId: (value: string) => void; onTargetType: (value: "attempt" | "work_unit" | "plan" | "contract") => void }) {
	const { t } = useTranslation();
	return (
		<div className="mt-3 grid gap-2 sm:grid-cols-2">
			<label className="flex flex-col gap-1 text-xs text-muted-foreground">
				{t("outcome.proof.reentryTarget")}
				<select className="h-9 rounded-md border border-input bg-background px-2 text-foreground" onChange={(event) => onTargetType(event.target.value as typeof targetType)} value={targetType}><option value="contract">{t("outcome.proof.targetContract")}</option><option value="plan">{t("outcome.proof.targetPlan")}</option><option value="work_unit">{t("outcome.proof.targetWorkUnit")}</option><option value="attempt">{t("outcome.proof.targetAttempt")}</option></select>
			</label>
			{targetType === "contract" ? <p className="self-end text-xs text-muted-foreground">{t("outcome.proof.targetContract")}</p> : <label className="flex flex-col gap-1 text-xs text-muted-foreground">
				{t("outcome.proof.reentryId")}
				<Input onChange={(event) => onTargetId(event.target.value)} placeholder={t("outcome.proof.reentryIdRequired")} value={targetId} />
			</label>}
		</div>
	);
}

function ProofFailure({ message, onRetry }: { message: string; onRetry: () => void }) {
	const { t } = useTranslation();
	return <div className="max-w-xl rounded-md border border-warning/40 bg-warning/5 p-4" role="alert"><h3 className="text-sm font-medium">{t("outcome.proof.errorTitle")}</h3><p className="mt-1 text-muted-foreground text-sm">{message}</p><Button className="mt-3" onClick={onRetry} size="sm" variant="outline">{t("outcome.understand.retry")}</Button></div>;
}

async function sha256(value: string): Promise<string> {
	const bytes = new TextEncoder().encode(value);
	const digest = await crypto.subtle.digest("SHA-256", bytes);
	return Array.from(new Uint8Array(digest), (byte) => byte.toString(16).padStart(2, "0")).join("");
}
