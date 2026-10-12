import type { ReactNode } from "react";
import { StateBadge, type StateTone } from "./StateBadge";
import { cn } from "./utils";

/**
 * Evidence verdicts use the shared state vocabulary. "Not run" and "Skipped"
 * stay visible; they never disappear to make a list look green.
 */
export type EvidenceVerdict = "verified" | "notRun" | "stale" | "failed" | "unconfirmed" | "pending";

const VERDICT_TONE: Record<EvidenceVerdict, StateTone> = {
	verified: "good",
	notRun: "neutral",
	pending: "neutral",
	stale: "caution",
	failed: "critical",
	unconfirmed: "critical",
};

export function evidenceVerdictTone(verdict: EvidenceVerdict): StateTone {
	return VERDICT_TONE[verdict];
}

export type EvidenceRowProps = {
	/** A plain statement of fact, not a task. */
	assertion: ReactNode;
	verdict: EvidenceVerdict;
	/** The verdict word, host-translated ("Verified", "Not run"…). */
	verdictLabel: string;
	/** Who collected it, the source and when: "Checked by Kennel · npm test · 16:40". */
	provenance?: ReactNode;
	/** What the check doesn't prove. */
	limitation?: ReactNode;
	/** Extra content under the provenance line (detail disclosure, actions). */
	children?: ReactNode;
	className?: string;
	as?: "li" | "div";
	"data-testid"?: string;
};

/** One assertion, its verdict and where the verdict came from. */
export function EvidenceRow({
	assertion,
	verdict,
	verdictLabel,
	provenance,
	limitation,
	children,
	className,
	as: Tag = "li",
	"data-testid": testId,
}: EvidenceRowProps) {
	return (
		<Tag
			className={cn(
				"flex items-start gap-3 hairline-b border-wd-hairline py-3 last:border-b-0",
				className,
			)}
			data-evidence-verdict={verdict}
			data-testid={testId}
		>
			<div className="flex min-w-0 flex-1 flex-col gap-1">
				<div className="break-words text-sm leading-5 text-wd-primary">{assertion}</div>
				{provenance ? <div className="text-xs leading-[18px] text-wd-tertiary">{provenance}</div> : null}
				{limitation ? <div className="text-xs leading-[18px] text-wd-secondary">{limitation}</div> : null}
				{children}
			</div>
			<StateBadge label={verdictLabel} tone={VERDICT_TONE[verdict]} />
		</Tag>
	);
}
