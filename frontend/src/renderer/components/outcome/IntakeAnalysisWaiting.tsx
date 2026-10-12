import { useSettings } from "../../hooks/useSettings";
import { ReasoningSettingsSection } from "../settings/ReasoningSettingsSection";
import { StateMark } from "@pin4sf/kennel-product-ui";
import { useTranslation } from "react-i18next";
import waldoMark from "../../../../assets/waldo-mark.svg";

import { useAgentsQuery } from "../../hooks/useAgentsQuery";
import type { IntakeAnalysisRequest } from "../../hooks/useIntakeAnalysisRequest";
import { intakeFailureMessage, nativeReasoningUnavailable, reasoningStatusMessage } from "../../lib/reasoning-status";
import { AgentAvatar } from "../AgentAvatar";
import { Button } from "../ui/button";

/**
 * The catalog's display label for a harness, falling back to the stored id.
 *
 * The id is what the daemon records ("codex"); the label is what every other
 * agent control in the app shows ("Codex"). Reading it here keeps this surface
 * from being the one place that spells an agent differently.
 */
function useHarnessLabel(harness: string | undefined): string | undefined {
	const catalog = useAgentsQuery().data;
	if (!harness) return undefined;
	return catalog?.supported?.find((agent) => agent.id === harness)?.label ?? harness;
}

/**
 * What an owner sees while an agent reads the project to propose a Contract.
 *
 * Two things make this different from the indefinite "Understanding the
 * Outcome" message it replaces. It names who is working, because an anonymous
 * spinner gives a person nothing to judge — a spawned agent can take minutes,
 * and knowing which harness is reading is what makes waiting a decision rather
 * than a hope. The owner can release the intake while waiting; no alternate
 * reasoning path is substituted.
 */
export function IntakeAnalysisWaiting({
	request,
	pending,
	onRelease,
}: {
	/**
	 * Absent until the ask loads, and absent entirely for the brief moment an
	 * model analysis occupies this state. The surface still renders: the
	 * daemon says analysis is happening, and waiting silently on a blank page
	 * would be worse than waiting on an unnamed one.
	 */
	request?: IntakeAnalysisRequest;
	pending: boolean;
	onRelease: () => void;
}) {
	const { t } = useTranslation();
	// The avatar is keyed by the stored id; the text shows the catalog label.
	const harnessId = request?.harness?.trim();
	const harness = useHarnessLabel(harnessId);
	return (
		<div
			aria-busy="true"
			aria-live="polite"
			className="mx-auto flex h-full w-full max-w-xl flex-col items-center justify-center gap-4 px-4 text-center sm:px-8"
			data-testid="intake-analysis-waiting"
		>
			{/* Who is working, said once and held still: no spinner, no pulse.
			    The sentence below is the working state. */}
			<div className="flex items-center gap-2.5">
				{harnessId ? <AgentAvatar provider={harnessId} className="size-icon-lg" decorative /> : null}
				<img alt="" aria-hidden="true" className="size-icon-sm" data-brand="waldo" src={waldoMark} />
			</div>
			<div className="flex flex-col gap-2">
				<h2 className="wd-display-4 text-wd-primary">
					{harness ? t("outcome.intake.waiting.titleNamed", { harness }) : t("outcome.intake.waiting.title")}
				</h2>
				<p className="text-sm leading-body text-wd-secondary">{t("outcome.intake.waiting.body")}</p>
			</div>

			{/* Always present, never behind a disclosure: the whole point is that
			    waiting is optional. */}
			<div className="flex flex-wrap items-center justify-center gap-2">
				<Button disabled={pending} onClick={onRelease} size="sm" variant="ghost" type="button">
					{t("outcome.intake.waiting.release")}
				</Button>
			</div>
		</div>
	);
}

/**
 * A draft the daemon refused, kept beside the reason it was refused.
 *
 * The draft is retained rather than discarded so the refusal is inspectable:
 * an owner deciding whether to ask again deserves to see what the agent
 * actually proposed and which rule it broke, instead of only being told that
 * something went wrong.
 */
export function IntakeAnalysisRefused({
	request,
	failureCode,
	pending,
	onRetry,
}: {
	/** Absent when the analysis failed without an agent ever answering. */
	request?: IntakeAnalysisRequest;
	failureCode?: string;
	pending: boolean;
	onRetry: () => void;
}) {
	const { t } = useTranslation();
	const harness = useHarnessLabel(request?.harness?.trim());
	const { settings } = useSettings();
	const savedReasoning = settings?.reasoning;
	const safeFailure = request?.rawProposal ? request.refusalReason : intakeFailureMessage(failureCode, t);
	const nativeReason = savedReasoning && nativeReasoningUnavailable(savedReasoning)
		? reasoningStatusMessage(savedReasoning, savedReasoning.provider, t)
		: undefined;
	return (
		<div
			className="mx-auto flex w-full max-w-2xl flex-col gap-3 px-4 py-8 sm:px-8"
			data-testid="intake-analysis-refused"
		>
			<div className="flex flex-col gap-2">
				<h2 className="wd-display-4 text-wd-primary">
					{harness ? t("outcome.intake.refused.titleNamed", { harness }) : t("outcome.intake.reasoningFailedTitle")}
				</h2>
				<p className="text-sm leading-body text-wd-secondary">
					{t(request?.rawProposal ? "outcome.intake.refused.body" : "outcome.intake.reasoningFailedBody")}
				</p>
			</div>
			{safeFailure ? (
				<p className="flex items-start gap-2 rounded-md bg-wd-inset p-3 text-sm leading-body text-wd-primary" role="alert">
					<StateMark className="mt-1 text-critical" tone="critical" />
					<span className="min-w-0">{safeFailure}</span>
				</p>
			) : null}
			{nativeReason ? (
				<p className="flex items-start gap-2 rounded-md bg-wd-inset p-3 text-sm leading-body text-wd-primary" role="status">
					<StateMark className="mt-1 text-caution" tone="caution" />
					<span className="min-w-0">{nativeReason}</span>
				</p>
			) : null}
			{request?.rawProposal ? (
				<details className="rounded-lg hairline border-wd-hairline bg-wd-surface p-4">
					<summary className="cursor-pointer rounded-sm text-sm font-medium text-wd-primary focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-(--border-focus)">
						{t("outcome.intake.refused.showDraft")}
					</summary>
					<pre className="mt-3 max-h-64 overflow-auto whitespace-pre-wrap break-words rounded-md bg-wd-inset p-3 font-mono text-xs leading-body text-wd-secondary">
						{request.rawProposal}
					</pre>
				</details>
			) : null}
			<details className="rounded-lg hairline border-wd-hairline bg-wd-surface p-4">
				<summary className="cursor-pointer rounded-sm text-sm font-medium text-wd-primary focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-(--border-focus)">{t("outcome.intake.configureReasoning")}</summary>
				<div className="mt-3">
					<ReasoningSettingsSection />
				</div>
			</details>
			<div className="flex flex-wrap gap-2">
				<Button
					disabled={pending || !settings?.reasoning.ready}
					onClick={onRetry}
					size="sm"
					variant="primary"
					type="button"
				>
					{t("outcome.intake.refused.retry")}
				</Button>
			</div>
		</div>
	);
}

/**
 * One line above the Contract saying where it came from.
 *
 * A person about to hand-write four criteria deserves to know whether anything
 * analyzed the first draft. Without this, an offline proposal and an
 * agent-authored one look identical on screen while being worth very different
 * amounts of trust.
 */
export function ProposalProvenanceNote({ kind, harness }: { kind: "agent" | "unattributed"; harness?: string }) {
	const { t } = useTranslation();
	const label = useHarnessLabel(harness);
	if (kind === "agent") {
		return (
			<p className="flex items-center gap-1.5 text-xs text-wd-tertiary" data-testid="proposal-provenance">
				{harness ? <AgentAvatar provider={harness} className="size-icon-sm" decorative /> : null}
				{label ? t("outcome.intake.provenance.agentNamed", { harness: label }) : t("outcome.intake.provenance.agent")}
			</p>
		);
	}
	return (
		<p className="text-xs text-wd-tertiary" data-testid="proposal-provenance">
			{t("outcome.intake.provenance.unattributed")}
		</p>
	);
}
