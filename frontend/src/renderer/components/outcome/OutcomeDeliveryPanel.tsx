import { StateBadge, StateMark, type StateTone } from "@pin4sf/kennel-product-ui";
import { CheckCircle2, FolderOpen, RefreshCw } from "lucide-react";
import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";

import {
	useOutcomeDeliveries,
	useRequestOutcomeDelivery,
	type OutcomeDeliveryRecord,
	type RequestOutcomeDeliveryInput,
} from "../../hooks/useOutcomeArtifacts";
import { useOutcomeAttempts, useOutcomeProof } from "../../hooks/useOutcome";
import { useOutcomeRunState } from "../../hooks/useOutcomeRunState";
import { aoBridge } from "../../lib/bridge";
import { Button } from "../ui/button";
import { Input } from "../ui/input";
import { Label } from "../ui/label";

function latestAcceptedDecision(proof: ReturnType<typeof useOutcomeProof>["proof"]) {
	return proof?.decisions
		.filter((decision) => decision.kind === "accept")
		.sort((left, right) => left.createdAt.localeCompare(right.createdAt))
		.at(-1);
}

const DELIVERY_STATE: Record<OutcomeDeliveryRecord["state"], { tone: StateTone; key: "mission.delivery.state.pending" | "mission.delivery.state.succeeded" | "mission.delivery.state.failed" | "mission.delivery.state.cancelled" }> = {
	pending: { tone: "neutral", key: "mission.delivery.state.pending" },
	succeeded: { tone: "good", key: "mission.delivery.state.succeeded" },
	failed: { tone: "critical", key: "mission.delivery.state.failed" },
	cancelled: { tone: "neutral", key: "mission.delivery.state.cancelled" },
};

/**
 * Delivery is the final transfer boundary, not a synonym for acceptance. The
 * owner supplies the exact retained Attempt/artifact identity and destination;
 * the daemon owns validation, copy, recovery and the durable delivery ledger.
 */
export function OutcomeDeliveryPanel({ outcomeId }: { outcomeId: string }) {
	const { t } = useTranslation();
	const deliveriesQuery = useOutcomeDeliveries(outcomeId);
	const attemptsQuery = useOutcomeAttempts(outcomeId);
	const proofQuery = useOutcomeProof(outcomeId);
	const runQuery = useOutcomeRunState(outcomeId);
	const requestMutation = useRequestOutcomeDelivery(outcomeId);
	const [attemptId, setAttemptId] = useState("");
	const [artifactVersion, setArtifactVersion] = useState("");
	const [destination, setDestination] = useState("");
	const [disposition, setDisposition] = useState<"accepted" | "draft">("draft");

	const deliveries = deliveriesQuery.deliveries;
	const acceptanceDecision = latestAcceptedDecision(proofQuery.proof);
	const exportEligibility = runQuery.data?.eligibleActions.find((item) => item.action === "export");
	const latestAttempt = attemptsQuery.attempts?.find((attempt) => attempt.status === "succeeded") ?? attemptsQuery.attempts?.at(-1);

	useEffect(() => {
		if (attemptId) return;
		setAttemptId(deliveries[0]?.attemptId ?? latestAttempt?.id ?? "");
	}, [attemptId, deliveries, latestAttempt?.id]);
	useEffect(() => {
		if (artifactVersion) return;
		setArtifactVersion(deliveries[0]?.artifactVersion ?? "");
	}, [artifactVersion, deliveries]);
	useEffect(() => {
		if (!acceptanceDecision && disposition === "accepted") setDisposition("draft");
	}, [acceptanceDecision, disposition]);

	const pending = requestMutation.pending;
	const failure = deliveriesQuery.failure ?? attemptsQuery.failure ?? requestMutation.failure;
	const featureUnavailable = exportEligibility && !exportEligibility.available;
	const exactFieldsReady = Boolean(attemptId.trim() && artifactVersion.trim() && destination.trim());
	const canRequest = Boolean(exportEligibility?.available) && exactFieldsReady;
	const unavailableReason = exportEligibility?.reason
		? t(`mission.delivery.reason.${exportEligibility.reason}`, { defaultValue: exportEligibility.reason })
		: "unavailable";

	async function chooseDestination() {
		const path = await aoBridge.app.chooseDirectory(t("mission.delivery.chooseDestination"));
		if (path) setDestination(path);
	}

	async function request() {
		if (!canRequest || pending) return;
		const input: RequestOutcomeDeliveryInput = {
			attemptId: attemptId.trim(),
			artifactVersion: artifactVersion.trim(),
			destination: destination.trim(),
			disposition,
			...(disposition === "accepted" && acceptanceDecision
				? { acceptanceDecisionId: acceptanceDecision.id }
				: {}),
		};
		try {
			await requestMutation.request(input);
		} catch {
			// The typed daemon refusal is rendered below and remains retryable.
		}
	}

	function reuseDelivery(delivery: OutcomeDeliveryRecord) {
		setAttemptId(delivery.attemptId);
		setArtifactVersion(delivery.artifactVersion);
		setDestination(delivery.destination);
		setDisposition(acceptanceDecision && delivery.disposition === "accepted" ? "accepted" : "draft");
	}

	return (
		<section className="mt-5 flex flex-col gap-3 rounded-lg hairline border-wd-hairline bg-wd-surface p-5" data-testid="outcome-delivery-panel">
			<div className="flex flex-wrap items-start justify-between gap-3">
				<div className="flex min-w-0 flex-col gap-1">
					<div className="flex items-center gap-2">
						<CheckCircle2 aria-hidden="true" className="size-4 text-wd-secondary" />
						<h3 className="text-sm font-medium text-wd-primary">{t("mission.delivery.heading")}</h3>
					</div>
					<p className="text-xs text-wd-secondary">{t("mission.delivery.intro")}</p>
				</div>
				{deliveriesQuery.failure && (
					<Button onClick={deliveriesQuery.refetch} size="sm" variant="ghost">
						<RefreshCw aria-hidden="true" className="size-3.5" />
						{t("mission.refresh")}
					</Button>
				)}
			</div>

			<div className="flex flex-col gap-3 rounded-md bg-wd-inset p-3">
				<p className="text-xs text-wd-secondary">{t("mission.delivery.exactArtifact")}</p>
				<div className="grid gap-3 sm:grid-cols-2">
					<div className="flex flex-col gap-1">
						<Label htmlFor="delivery-attempt-id">{t("mission.delivery.attemptId")}</Label>
						<Input className="bg-wd-surface tabular-nums" id="delivery-attempt-id" onChange={(event) => setAttemptId(event.target.value)} value={attemptId} />
					</div>
					<div className="flex flex-col gap-1">
						<Label htmlFor="delivery-artifact-version">{t("mission.delivery.artifactVersion")}</Label>
						<Input className="bg-wd-surface tabular-nums" id="delivery-artifact-version" onChange={(event) => setArtifactVersion(event.target.value)} value={artifactVersion} />
					</div>
				</div>
				<div className="flex flex-col gap-1">
					<Label htmlFor="delivery-destination">{t("mission.delivery.destination")}</Label>
					<div className="flex gap-2">
						<Input className="bg-wd-surface" id="delivery-destination" onChange={(event) => setDestination(event.target.value)} value={destination} />
						<Button aria-label={t("mission.delivery.chooseDestination")} onClick={() => void chooseDestination()} size="icon" type="button" variant="ghost">
							<FolderOpen aria-hidden="true" className="size-4" />
						</Button>
					</div>
				</div>
				<fieldset disabled={pending}>
					<legend className="text-xs text-wd-secondary">{t("mission.delivery.disposition")}</legend>
					<div className="mt-2 flex flex-wrap gap-4 text-sm text-wd-primary">
						<label className="flex items-center gap-2">
							<input checked={disposition === "draft"} className="accent-(--text-primary)" name="delivery-disposition" onChange={() => setDisposition("draft")} type="radio" />
							{t("mission.delivery.draft")}
						</label>
						<label className="flex items-center gap-2">
							<input checked={disposition === "accepted"} className="accent-(--text-primary)" disabled={!acceptanceDecision} name="delivery-disposition" onChange={() => setDisposition("accepted")} type="radio" />
							{t("mission.delivery.accepted")}
						</label>
					</div>
					<p className="mt-1 text-xs text-wd-secondary">
						{acceptanceDecision ? t("mission.delivery.acceptedAvailable") : t("mission.delivery.acceptedUnavailable")}
					</p>
				</fieldset>
				<div className="flex flex-wrap items-center gap-x-3 gap-y-1">
					<Button aria-busy={pending || undefined} data-testid="outcome-delivery-request" disabled={!canRequest || pending} onClick={() => void request()} size="sm">
						{pending ? t("mission.delivery.requesting") : t("mission.delivery.request")}
					</Button>
					{featureUnavailable && <span className="text-xs text-wd-secondary">{t("mission.delivery.unavailableReason", { reason: unavailableReason })}</span>}
					{!exportEligibility && <span className="text-xs text-wd-secondary">{t("mission.delivery.waitingForEligibility")}</span>}
				</div>
			</div>

			<p className="text-xs text-wd-tertiary">{t("mission.delivery.boundary")}</p>
			{deliveriesQuery.isLoading && <p className="text-xs text-wd-secondary" role="status">{t("mission.delivery.loading")}</p>}
			{failure && <p className="flex items-start gap-1.5 text-xs text-wd-primary" role="alert"><StateMark className="mt-0.5 text-critical" tone="critical" />{failure.message}</p>}

			<div className="flex flex-col gap-1">
				<h4 className="text-sm font-medium text-wd-primary">{t("mission.delivery.history")}</h4>
				{deliveries.length === 0 ? (
					<p className="text-xs text-wd-secondary">{t("mission.delivery.empty")}</p>
				) : (
					<ul className="flex flex-col">
						{deliveries.map((delivery) => <DeliveryHistoryRow delivery={delivery} key={delivery.id} onReuse={reuseDelivery} />)}
					</ul>
				)}
			</div>
		</section>
	);
}

function DeliveryHistoryRow({ delivery, onReuse }: { delivery: OutcomeDeliveryRecord; onReuse: (delivery: OutcomeDeliveryRecord) => void }) {
	const { t } = useTranslation();
	const sameDestinationRetry = delivery.state === "failed" && delivery.failureCode === "DELIVERY_INTERRUPTED";
	const recoveryNote = delivery.completionSource === "recovered"
		? t("mission.delivery.recovered")
		: delivery.completionSource === "observed"
			? t("mission.delivery.observed")
			: undefined;

	return (
		<li className="flex flex-col gap-1 hairline-b border-wd-hairline py-3 text-xs last:border-b-0" data-testid={`delivery-${delivery.id}`}>
			<div className="flex flex-wrap items-start justify-between gap-2">
				<p className="min-w-0 break-all text-sm text-wd-primary">{delivery.destination}</p>
				<StateBadge label={t(DELIVERY_STATE[delivery.state]?.key ?? "mission.delivery.state.pending")} tone={DELIVERY_STATE[delivery.state]?.tone ?? "neutral"} />
			</div>
			<p className="text-wd-tertiary">
				{delivery.disposition === "accepted" ? t("mission.delivery.accepted") : t("mission.delivery.draft")}
				{" · "}
				<span className="tabular-nums">{t("mission.delivery.artifactSummary", { attempt: delivery.attemptId, version: delivery.artifactVersion })}</span>
				{" · "}
				<time className="tabular-nums" dateTime={delivery.requestedAt}>{new Date(delivery.requestedAt).toLocaleString()}</time>
			</p>
			{recoveryNote && <p className="text-wd-secondary">{recoveryNote}</p>}
			{delivery.failureCode && <p className="text-wd-primary"><span className="text-wd-secondary">{delivery.failureCode}: </span>{delivery.failureDetail ?? t("mission.delivery.noFailureDetail")}</p>}
			{delivery.failureCode === "DELIVERY_RECOVERY_MISMATCH" && <p className="text-wd-secondary">{t("mission.delivery.chooseDifferentDestination")}</p>}
			{delivery.failureCode === "DELIVERY_RECOVERY_UNREADABLE" && <p className="text-wd-secondary">{t("mission.delivery.unreadable")}</p>}
			{delivery.failureCode === "DELIVERY_RECOVERY_UNVERIFIABLE" && <p className="text-wd-secondary">{t("mission.delivery.reproduce")}</p>}
			{delivery.state === "pending" && <p className="text-wd-secondary">{t("mission.delivery.pendingNoRetry")}</p>}
			{sameDestinationRetry && <Button className="mt-1 self-start" onClick={() => onReuse(delivery)} size="sm" variant="secondary">{t("mission.delivery.retrySame")}</Button>}
		</li>
	);
}
