import { ContractCoverage, StateLabel, StateMark } from "@pin4sf/kennel-product-ui";
import { ExternalLink, X } from "lucide-react";
import { useTranslation } from "react-i18next";

import type { MessageKey } from "../../i18n/messages";
import { Button } from "../ui/button";
import type { MissionNodeRecord } from "../../hooks/useOutcome";
import type { MissionNodeView } from "./mission-presentation";

/**
 * The one inspector Canvas and List both open on selection (F2 scope item
 * 7/invariant I). Inspect-only: no live-session or takeover action is
 * rendered here — those remain gated on a separate canonical control-state
 * bridge this slice does not have. `session.status = unknown` always renders
 * Unknown, never Running.
 */
export type OutcomeInspectorProps = {
	node: MissionNodeRecord;
	view: MissionNodeView;
	onClose: () => void;
	/** The owning overlay supplies dialog semantics; omit nested region semantics. */
	withinDialog?: boolean;
	onOpenSession?: (sessionId: string) => void;
};

export function OutcomeInspector({ node, view, onClose, onOpenSession, withinDialog = false }: OutcomeInspectorProps) {
	const { t } = useTranslation();
	return (
		<div
			aria-label={withinDialog ? undefined : t("mission.inspector.heading" satisfies MessageKey)}
			className="flex h-full min-h-0 flex-col gap-5 overflow-y-auto rounded-lg hairline border-wd-hairline bg-wd-surface p-5"
			data-testid="outcome-inspector"
			role={withinDialog ? undefined : "region"}
		>
			<div className="flex items-start justify-between gap-2">
				<h3 className="wd-headline break-words text-wd-primary">{view.title}</h3>
				<Button
					aria-label={t("mission.inspector.close" satisfies MessageKey)}
					data-testid="outcome-inspector-close"
					onClick={onClose}
					size="icon-sm"
					variant="ghost"
				>
					<X aria-hidden="true" className="size-4" />
				</Button>
			</div>

			<section>
				<h4 className="wd-label text-wd-secondary">
					{t("mission.inspector.objective" satisfies MessageKey)}
				</h4>
				<p className="mt-1 text-sm text-wd-primary">{view.title}</p>
			</section>

			<section>
				<h4 className="wd-label text-wd-secondary">
					{t("mission.inspector.currentState" satisfies MessageKey)}
				</h4>
				<p className="mt-1 text-sm font-medium" data-testid="outcome-inspector-state">
					<StateLabel label={view.status.label} tone={view.status.tone} />
				</p>
			</section>

			{view.attention && (
				<section className="rounded-md bg-wd-inset p-3" data-testid="outcome-inspector-needs-you">
					<h4 className="flex items-center gap-2 text-sm font-medium text-wd-primary">
						<StateMark tone="needsYou" />
						{view.attention.label}
					</h4>
					<p className="mt-1 text-sm text-wd-secondary">{view.attention.summary}</p>
				</section>
			)}

			<section>
				<h4 className="wd-label text-wd-secondary">
					{t("mission.inspector.contractCoverage" satisfies MessageKey)}
				</h4>
				<div className="mt-1">
					{node.criterionReady ? (
						<ContractCoverage
							criterionLabel={(position) => t("mission.criteria.position" satisfies MessageKey, { position })}
							items={node.criterionIds.map((id, index) => ({
								position: index + 1,
								ready: node.criterionReady?.[id] === true,
							}))}
							pendingLabel={t("mission.criteria.pending" satisfies MessageKey)}
							readyLabel={t("mission.criteria.ready" satisfies MessageKey)}
						/>
					) : (
						<ContractCoverage unavailable unavailableLabel={t("mission.criteria.unavailable" satisfies MessageKey)} />
					)}
				</div>
			</section>

			<section>
				<h4 className="wd-label text-wd-secondary">
					{t("mission.inspector.latestAttempt" satisfies MessageKey)}
				</h4>
				{view.attemptLabel ? (
					<p className="mt-1 text-sm text-wd-primary" data-testid="outcome-inspector-attempt">
						{view.attemptLabel}
						{view.sessionStatusLabel ? ` — ${view.sessionStatusLabel}` : ""}
					</p>
				) : (
					<p className="mt-1 text-sm text-wd-secondary">{t("mission.inspector.noAttempt" satisfies MessageKey)}</p>
				)}
			</section>

			{node.currentAttempt?.status === "running" && node.currentAttempt.session?.sessionId && onOpenSession ? (
				<Button data-testid="outcome-inspector-open-session" onClick={() => onOpenSession(node.currentAttempt!.session!.sessionId)} variant="outline">
					<ExternalLink aria-hidden="true" className="size-icon-sm" />
					{t("mission.sessionHub.open")}
				</Button>
			) : null}

			<details className="mt-auto">
				<summary className="wd-label cursor-pointer text-wd-secondary">
					{t("mission.inspector.technicalDetails" satisfies MessageKey)}
				</summary>
				<dl className="mt-2 flex flex-col gap-1 text-xs text-wd-secondary" data-testid="outcome-inspector-technical">
					<div className="flex justify-between gap-2">
						<dt>{t("mission.inspector.workUnitId" satisfies MessageKey)}</dt>
						<dd className="wd-data truncate text-wd-tertiary">{node.workUnitId}</dd>
					</div>
					{node.currentAttempt && (
						<div className="flex justify-between gap-2">
							<dt>{t("mission.inspector.attemptId" satisfies MessageKey)}</dt>
							<dd className="wd-data truncate text-wd-tertiary">{node.currentAttempt.attemptId}</dd>
						</div>
					)}
				</dl>
			</details>
		</div>
	);
}
