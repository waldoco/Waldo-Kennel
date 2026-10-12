import { Dialog, DialogContent, DialogTitle, DialogDescription, DialogTrigger } from "../ui/dialog";
import { agentLabel, StateBadge, type StateTone } from "@pin4sf/kennel-product-ui";
import { Maximize2 } from "lucide-react";
import { ApprovedChecks } from "./ApprovedChecks";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import type { components } from "../../../api/schema";
import type { AttemptRecord } from "../../hooks/useOutcome";
import type { MessageKey } from "../../i18n/messages";
import { MissionWorkUnitGraph } from "./MissionWorkUnitGraph";
import { layerByDependency } from "../../lib/dependency-layers";

import { ENDED_ATTEMPT_PHASES } from "./attemptPhases";
import { Button } from "../ui/button";
import { newestAttempt } from "./OutcomeRunBoardAdapters";

type Unit = components["schemas"]["PlanWorkUnitResponse"];
type ChangeRecord = components["schemas"]["ControllersOutcomeResultChangesResponse"];
type Schedule = components["schemas"]["ScheduleResponse"];

/** Graph and Table inspect the same durable units; selecting never admits work. */
export function MissionPlanView({
	workUnits,
	schedule,
	criterionText,
	attempts,
	onOpenAttempt,
	onReviewProof,
	changes,
	graphOnly = false,
}: {
	workUnits: Unit[];
	schedule?: Schedule;
	criterionText?: (id: string) => string | undefined;
	attempts?: AttemptRecord[];
	onOpenAttempt?: (attempt: AttemptRecord) => void;
	/** Opens the daemon Result/proof surface for this Outcome, when the host offers it. */
	onReviewProof?: () => void;
	/** Daemon-projected measured changes (proof.result.changes); filtered per WorkUnit here. */
	changes?: ChangeRecord[];
	/** Execution embeds only the live topology. Plan detail remains in Plan. */
	graphOnly?: boolean;
}) {
	const { t } = useTranslation();
	const [view, setView] = useState<"graph" | "table">("graph");
	const [selectedId, setSelectedId] = useState<string>();
	const units = layerByDependency(
		workUnits.map((unit) => ({ ...unit, upstream: unit.dependsOn ?? [] })).sort((a, b) => a.id.localeCompare(b.id)),
	).flat();
	const selected = units.find((unit) => unit.id === selectedId);
	const entry = schedule?.workUnits.find((item) => item.workUnit.id === selectedId);
	const selectedAttempts = attempts?.filter((attempt) => attempt.workUnitId === selectedId) ?? [];
	const selectedAttempt = newestAttempt(selectedAttempts);
	const title = (id: string) => units.find((unit) => unit.id === id)?.title ?? id;
	return (
		<section className="flex min-w-0 flex-col gap-3" data-testid="mission-plan-view">
			<div className="flex flex-wrap items-center gap-2" role="group" aria-label={t("mission.plan")}>
				{/* Graph / Table as a segmented control: inset track, ink thumb
				    on the pressed reading. */}
				{!graphOnly && (
					<span className="inline-flex h-9 items-center gap-1 rounded-full bg-wd-inset p-1">
						{(["graph", "table"] as const).map((mode) => (
							<button
								aria-pressed={view === mode}
								className="inline-flex h-full items-center rounded-full px-3 text-sm font-medium text-wd-secondary transition-[background-color,color] duration-fast hover:text-wd-primary focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-(--border-focus) aria-pressed:bg-wd-inverse aria-pressed:text-wd-on-inverse motion-reduce:transition-none"
								key={mode}
								onClick={() => setView(mode)}
								type="button"
							>
								{t(`mission.${mode}`)}
							</button>
						))}
					</span>
				)}

                <Dialog>
                    <DialogTrigger asChild><Button size="sm" variant="ghost"><Maximize2 aria-hidden="true" className="size-3.5" />{t("mission.expandGraph")}</Button></DialogTrigger>
                    <DialogContent className="h-[90vh] w-[95vw] max-w-[95vw] overflow-hidden">
                        <DialogTitle>{t("outcome.missionGraph.heading")}</DialogTitle>
                        <DialogDescription>{t(schedule ? "outcome.missionGraph.serialNote" : "outcome.missionGraph.proposedNote")}</DialogDescription>
                        <div className="min-h-0 flex-1 overflow-auto p-4">
                            <MissionWorkUnitGraph workUnits={units} schedule={schedule} criterionText={criterionText} selectedWorkUnitId={selected?.id} onSelectWorkUnit={setSelectedId} />
							{!graphOnly && selected && <section className="mt-6 hairline-t border-wd-hairline pt-4"><h3 className="text-base font-medium text-wd-primary">{selected.title}</h3><p className="mt-2 text-sm text-wd-secondary">{selected.outputSummary}</p><ApprovedChecks unit={selected} criterionText={criterionText} /></section>}
                        </div>
                    </DialogContent>
                </Dialog>
			</div>
			{!graphOnly && <div className="space-y-3">{units.map(unit => <div key={unit.id}><p className="text-sm font-medium text-wd-primary">{unit.title}</p><ApprovedChecks unit={unit} criterionText={criterionText} /></div>)}</div>}
            {/* Keep both views mounted to preserve graph zoom, focus and scroll on refresh. */}
			<div hidden={!graphOnly && view !== "graph"} className="overflow-auto p-1">
				<MissionWorkUnitGraph
					workUnits={units}
					schedule={schedule}
					criterionText={criterionText}
					selectedWorkUnitId={selected?.id}
					onSelectWorkUnit={setSelectedId}
				/>
			</div>
			{/* #78: on the execution surface, selecting a WorkUnit opens its exact
			    daemon facts - schedule state, blocker, criterion proof readiness
			    and attempt lineage with session engagement. Nothing here is
			    renderer-derived; proposed topology (no schedule) shows nothing,
			    because there are no execution facts to show yet. Plan text stays
			    in Plan. */}
			{graphOnly && selected && schedule && (
				<ExecutionUnitDetail
					attempts={selectedAttempts}
					changes={changes}
					criterionText={criterionText}
					entry={entry}
					onOpenAttempt={onOpenAttempt}
					onReviewProof={onReviewProof}
					selectedAttempt={selectedAttempt}
					title={title}
					unit={selected}
				/>
			)}
			{!graphOnly && <div hidden={view !== "table"} className="overflow-auto">
				<table className="w-full border-collapse text-left text-xs text-wd-primary">
					<caption className="pb-2 text-left text-wd-secondary">
						{t(schedule ? "outcome.missionGraph.serialNote" : "outcome.missionGraph.proposedNote")}
					</caption>
					<thead>
						<tr>
							{(["workUnit", "state", "dependencies", "output", "agent", "evidence"] as const).map((label) => (
								<th key={label} className="hairline-b border-wd-hairline px-2 py-2 font-medium text-wd-secondary">
									{t(`mission.${label}`)}
								</th>
							))}
						</tr>
					</thead>
					<tbody>
						{units.map((unit) => {
							const item = schedule?.workUnits.find((item) => item.workUnit.id === unit.id);
							return (
								<tr
									key={unit.id}
									className="align-top hover:bg-wd-hover aria-selected:bg-wd-selected"
									aria-selected={selected?.id === unit.id}
								>
									<td className="hairline-b border-wd-hairline p-2">
										<button
											type="button"
											className="rounded-xs text-left font-medium hover:underline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-(--border-focus)"
											onClick={() => setSelectedId(unit.id)}
										>
											{unit.title}
										</button>
									</td>
									<td className="hairline-b border-wd-hairline p-2">
										{item ? t(`outcome.missionGraph.state.${item.state}`) : t("outcome.missionGraph.state.proposed")}
									</td>
									<td className="hairline-b border-wd-hairline p-2">
										{(unit.dependsOn ?? []).map(title).join(", ") || t("outcome.decide.dependsOnNone")}
										{item?.blockedReason && (
											<p className="mt-0.5 text-wd-secondary">
												{t(`outcome.missionGraph.blocked.${item.blockedReason}`, {
													units: (item.blockingDependencies ?? []).map(title).join(", "),
												})}
											</p>
										)}
									</td>
									<td className="hairline-b border-wd-hairline p-2">{unit.outputSummary || t("mission.unknown")}</td>
									<td className="hairline-b border-wd-hairline p-2">
										{unit.provider || t("outcome.missionGraph.bindingUnset")} ·{" "}
										{unit.modelSelection === "provider_default"
											? t("outcome.missionGraph.providerDefault")
											: unit.model || t("mission.unknown")}
									</td>
									<td className="hairline-b border-wd-hairline p-2">
										{(unit.evidenceChecks ?? []).join(" · ") || t("mission.none")}
									</td>
								</tr>
							);
						})}
					</tbody>
				</table>
			</div>}
			{!graphOnly && (selected ? (
					<section className="hairline-t border-wd-hairline pt-3" aria-label={selected.title} data-testid="mission-unit-detail">
						<h3 className="text-sm font-medium break-words text-wd-primary">{selected.title}</h3>
						<p className="mt-1 text-xs tabular-nums text-wd-tertiary">
							{t("mission.planBinding", {
								plan: schedule?.plan?.number ?? t("mission.notApproved"),
								contract: selected.contractRevisionNumber,
							})}
						</p>
						<dl className="mt-3 grid grid-cols-[auto_1fr] gap-x-4 gap-y-2 text-xs text-wd-primary [&>dt]:text-wd-secondary">
							<dt>{t("mission.objective")}</dt>
							<dd className="whitespace-pre-wrap break-words">{selected.title}</dd>
							<dt>{t("mission.dependencies")}</dt>
							<dd>{(selected.dependsOn ?? []).map(title).join(", ") || t("outcome.decide.dependsOnNone")}</dd>
							<dt>{t("mission.output")}</dt>
							<dd className="whitespace-pre-wrap break-words">{selected.outputSummary || t("mission.unknown")}</dd>
							<dt>{t("mission.agent")}</dt>
							<dd>
								{selected.provider || t("outcome.missionGraph.bindingUnset")} · {selected.modelSelection === "provider_default"
									? t("outcome.missionGraph.providerDefault")
									: selected.model || t("outcome.missionGraph.modelUnknown")}
							</dd>
							<dt>{t("mission.criteria")}</dt>
						<dd>
							{(selected.criterionIds ?? []).map((id) => criterionText?.(id) ?? id).join(" · ") || t("mission.none")}
						</dd>
							<dt>{t("mission.evidence")}</dt>
							<dd>
								{(selected.evidenceChecks ?? []).join(" · ")}
								<p>{selected.verificationRequirement}</p>
								<ApprovedChecks criterionText={criterionText} unit={selected} />
							</dd>
							<dt>{t("mission.capabilities")}</dt>
						<dd>{(selected.requiredCapabilities ?? []).join(", ") || t("mission.none")}</dd>
						<dt>{t("mission.stops")}</dt>
						<dd>{(selected.stopConditions ?? []).join(" · ") || t("mission.none")}</dd>
						</dl>
						<WorkUnitExecutionFacts
							attempts={selectedAttempts}
							changes={changes}
							criterionText={criterionText}
							entry={entry}
							onOpenAttempt={onOpenAttempt}
							selectedAttempt={selectedAttempt}
							title={title}
							unit={selected}
						/>
				</section>
			) : (
				<p className="text-sm text-wd-secondary">{t("mission.selectUnit")}</p>
			))}
		</section>
	);
}

// Waldo's state vocabulary: running is the live accent, ready/proved/
// succeeded are good, a retryable unit is the owner's next move, failed and
// lost are critical, everything waiting is a neutral word.
const SCHEDULE_STATE_TONE: Record<string, StateTone> = {
	proven: "good",
	executing: "live",
	runnable: "good",
	paused: "neutral",
	retryable: "needsYou",
	blocked: "neutral",
};

const ATTEMPT_STATUS_BADGE: Record<AttemptRecord["status"], { key: MessageKey; tone: StateTone }> = {
	queued: { key: "outcome.run.badgeQueued", tone: "neutral" },
	running: { key: "outcome.run.badgeRunning", tone: "live" },
	paused: { key: "outcome.run.badgePaused", tone: "neutral" },
	succeeded: { key: "outcome.run.badgeSucceeded", tone: "good" },
	failed: { key: "outcome.run.badgeFailed", tone: "critical" },
	cancelled: { key: "outcome.run.badgeCancelled", tone: "neutral" },
	lost: { key: "outcome.run.badgeLost", tone: "critical" },
	reconciled: { key: "outcome.run.badgeReconciled", tone: "neutral" },
};

/**
 * One renderer for a WorkUnit's execution facts, shared by the Plan detail
 * panel and the execution graph card so the two never drift. Every value is a
 * daemon fact (schedule entry, attempt lineage, projected change manifest);
 * nothing is recomputed or inferred here.
 */
function WorkUnitExecutionFacts({
	unit,
	entry,
	attempts,
	selectedAttempt,
	criterionText,
	changes,
	onOpenAttempt,
	title,
}: {
	unit: Unit;
	entry?: Schedule["workUnits"][number];
	attempts: AttemptRecord[];
	selectedAttempt?: AttemptRecord;
	criterionText?: (id: string) => string | undefined;
	changes?: ChangeRecord[];
	onOpenAttempt?: (attempt: AttemptRecord) => void;
	title: (id: string) => string;
}) {
	const { t } = useTranslation();
	const blockers = (entry?.blockingDependencies ?? []).map(title).join(", ");
	const unitChanges = (changes ?? []).filter((change) => change.workUnitId === unit.id);
	const changeFiles = unitChanges.flatMap((change) => change.files.slice(0, 10));
	const changesTruncated = unitChanges.some((change) => change.truncated || change.files.length > 10);
	return (
		<div className="mt-3 flex flex-col gap-3" data-testid="work-unit-execution-facts">
			<div className="flex flex-col gap-1">
				<StateBadge
					className="self-start"
					label={entry ? t(`outcome.missionGraph.state.${entry.state}`) : t("outcome.missionGraph.state.proposed")}
					tone={entry ? (SCHEDULE_STATE_TONE[entry.state] ?? "neutral") : "neutral"}
				/>
				{entry?.blockedReason ? (
					<p className="text-xs leading-body text-wd-primary">
						{t(`outcome.missionGraph.blocked.${entry.blockedReason}`, { units: blockers })}
					</p>
				) : null}
			</div>
			{entry?.blockedDetail ? (
				<p className="whitespace-pre-wrap break-words text-xs text-wd-secondary">{entry.blockedDetail}</p>
			) : null}
			{(unit.criterionIds ?? []).length > 0 ? (
				<div>
					<h4 className="text-xs font-medium text-wd-secondary">{t("mission.evidence")}</h4>
					<ul className="mt-1.5 space-y-1.5 text-xs text-wd-primary" data-testid="mission-unit-criterion-readiness">
						{(unit.criterionIds ?? []).map((criterionId) => {
							const ready = entry?.criterionReady == null ? undefined : entry.criterionReady[criterionId];
							return (
								<li className="flex flex-wrap items-center gap-1.5" key={criterionId}>
									<span>{criterionText?.(criterionId) ?? criterionId}</span>
									{ready === undefined ? (
										<StateBadge label={t("mission.proofUnknown")} tone="neutral" />
									) : ready ? (
										<StateBadge label={t("mission.proofReady")} tone="good" />
									) : (
										<StateBadge label={t("mission.proofMissing")} tone="caution" />
									)}
								</li>
							);
						})}
					</ul>
				</div>
			) : null}
			{changeFiles.length > 0 ? (
				<div data-testid="mission-unit-changes">
					<h4 className="text-xs font-medium text-wd-secondary">{t("outcome.proof.whatChanged")}</h4>
					<ul className="mt-1.5 list-disc pl-4 text-xs text-wd-primary">
						{changeFiles.map((file) => (
							<li key={file.path}>{file.path} · {file.changeKind}</li>
						))}
					</ul>
					{changesTruncated ? <p className="mt-1 text-xs text-wd-secondary">{t("outcome.proof.changesMore")}</p> : null}
				</div>
			) : null}
			<div data-testid="mission-unit-attempts">
				<h4 className="text-xs font-medium text-wd-secondary">{t("mission.attempts")}</h4>
				{attempts.length > 0 ? (
					<ul className="mt-1.5 flex flex-col">
						{attempts.map((attempt) => (
							<li className="flex items-center justify-between gap-2 hairline-b border-wd-hairline py-1.5 text-xs text-wd-secondary last:border-b-0" key={attempt.id}>
								<span className="flex min-w-0 flex-wrap items-center gap-1.5">
									{(() => {
										const statusBadge = ATTEMPT_STATUS_BADGE[attempt.status];
										return statusBadge ? (
											<StateBadge label={t(statusBadge.key, { number: attempt.number })} tone={statusBadge.tone} />
										) : (
											<span>{attempt.status}</span>
										);
									})()}
									<time className="tabular-nums text-wd-tertiary" dateTime={attempt.updatedAt}>{new Date(attempt.updatedAt).toLocaleString()}</time>
									{attempt.sessions.length > 0
										? `· ${attempt.sessions.map((session) => `${agentLabel(session.harness)} · ${session.mode ?? t("mission.modeUnknown")}`).join(" · ")}`
										: ""}
								</span>
								{attempt.id === selectedAttempt?.id && attempt.sessions.length > 0 && onOpenAttempt ? (
									<Button onClick={() => onOpenAttempt(attempt)} size="sm" variant="secondary">
										{attempt.presentation && ENDED_ATTEMPT_PHASES.has(attempt.presentation.phase)
											? t("outcome.run.inspectCta")
											: t("outcome.run.engageCta")}
									</Button>
								) : null}
							</li>
						))}
					</ul>
				) : (
					<p className="mt-1 text-xs text-wd-tertiary">{t("mission.noAttempts")}</p>
				)}
			</div>
		</div>
	);
}

/** The execution graph's selected-WorkUnit card: shared facts plus Result navigation. */
function ExecutionUnitDetail({
	unit,
	entry,
	attempts,
	selectedAttempt,
	criterionText,
	changes,
	onOpenAttempt,
	onReviewProof,
	title,
}: {
	unit: Unit;
	entry?: Schedule["workUnits"][number];
	attempts: AttemptRecord[];
	selectedAttempt?: AttemptRecord;
	criterionText?: (id: string) => string | undefined;
	changes?: ChangeRecord[];
	onOpenAttempt?: (attempt: AttemptRecord) => void;
	onReviewProof?: () => void;
	title: (id: string) => string;
}) {
	const { t } = useTranslation();
	return (
		<section aria-label={unit.title} className="rounded-lg hairline border-wd-hairline bg-wd-surface p-4" data-testid="mission-unit-execution-detail">
			<h3 className="text-sm font-medium break-words text-wd-primary">{unit.title}</h3>
			<WorkUnitExecutionFacts
				attempts={attempts}
				changes={changes}
				criterionText={criterionText}
				entry={entry}
				onOpenAttempt={onOpenAttempt}
				selectedAttempt={selectedAttempt}
				title={title}
				unit={unit}
			/>
			{onReviewProof ? (
				<Button className="mt-4" onClick={onReviewProof} size="sm" variant="secondary">
					{t("mission.viewResult")}
				</Button>
			) : null}
		</section>
	);
}
