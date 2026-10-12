import { Fragment, type ReactNode } from "react";
import type { ExternalLinkComponent } from "./external-link";
import { ArrowUpRightIcon } from "./icons";
import { GithubAvatar } from "./GithubAvatar";
import type {
	PRCardPresentation,
	PRCardStatus,
	PRDisplayTone,
	PRNoun,
	PRSummaryLink,
	PRSummaryMetadata,
	PRSummaryPart,
	PRSummaryPartKey,
} from "./pull-request-models";
import { StateMark, type StateTone } from "./StateBadge";
import { cn } from "./utils";

const toneClass: Record<PRDisplayTone, string> = {
	neutral: "text-wd-secondary",
	passive: "text-wd-tertiary",
	success: "text-good",
	review: "text-caution",
	warning: "text-caution",
	error: "text-critical",
};

const toneState: Record<PRDisplayTone, StateTone> = {
	neutral: "neutral",
	passive: "neutral",
	success: "good",
	review: "caution",
	warning: "caution",
	error: "critical",
};

/** The status's Waldo shape cue in a fixed slot so neutral rows (no mark)
 *  keep the same text edge. In-flight work reads as live (blue dot), never
 *  as a pulse. */
function PRStatusMark({ breathe, className, tone }: { breathe?: boolean; className?: string; tone: PRDisplayTone }) {
	return (
		<span aria-hidden="true" className={cn("inline-flex h-4 w-3 shrink-0 items-center justify-center", className)} data-pr-status-mark="">
			<StateMark tone={breathe ? "live" : toneState[tone]} />
		</span>
	);
}

export type CountNounLabel = (count: number, noun: PRNoun) => string;

const defaultCountNounLabel: CountNounLabel = (count, noun) => `${count} ${noun}${count === 1 ? "" : "s"}`;

export function PRSummaryMeta({
	className,
	countNounLabel = defaultCountNounLabel,
	externalLink: ExternalLink,
	leading,
	pr,
}: {
	className?: string;
	countNounLabel?: CountNounLabel;
	externalLink: ExternalLinkComponent;
	leading?: string;
	pr: PRSummaryMetadata;
}) {
	const branchRange = prBranchRange(pr);
	const hasDiff = hasDiffMetadata(pr);
	const authorHandle = pr.author?.replace(/^@/, "") ?? "";
	const primary: ReactNode[] = [leading, branchRange].filter(Boolean);
	let author: ReactNode = null;
	if (authorHandle) {
		author =
			pr.provider === "github" ? (
				<ExternalLink
					className="inline-flex min-w-0 items-center gap-1 text-wd-primary underline-offset-2 hover:underline focus-visible:rounded-xs"
					href={`https://github.com/${encodeURIComponent(authorHandle)}`}
				>
					<GithubAvatar className="size-3" login={authorHandle} />
					{authorHandle}
				</ExternalLink>
			) : (
				<span>{authorHandle}</span>
			);
	}
	if (primary.length === 0 && !hasDiff && !author) {
		return null;
	}
	return (
		<div className={cn("min-w-0 text-2xs leading-4", className)}>
			{primary.length > 0 ? (
				<div className="flex min-w-0 flex-wrap items-center gap-x-1.5 gap-y-0.5 text-wd-secondary">
					{primary.map((part, index) => (
						<Fragment key={index}>
							{index > 0 ? <span className="shrink-0 text-wd-tertiary">·</span> : null}
							<span className="min-w-0 break-words [overflow-wrap:anywhere]">{part}</span>
						</Fragment>
					))}
				</div>
			) : null}
			{author ? <div className="mt-0.5 min-w-0 break-words [overflow-wrap:anywhere] text-wd-secondary">{author}</div> : null}
			{hasDiff ? <PRDiffMeta countNounLabel={countNounLabel} pr={pr} /> : null}
		</div>
	);
}

function PRDiffMeta({
	countNounLabel,
	pr,
}: {
	countNounLabel: CountNounLabel;
	pr: PRSummaryMetadata;
}) {
	const parts: ReactNode[] = [];
	if (pr.changedFiles > 0) {
		parts.push(
			<span className="text-wd-secondary" key="files">
				{countNounLabel(pr.changedFiles, "file")}
			</span>,
		);
	}
	if (pr.additions > 0) {
		parts.push(
			<span className="text-wd-secondary" key="additions">
				+{pr.additions}
			</span>,
		);
	}
	if (pr.deletions > 0) {
		parts.push(
			<span className="text-wd-secondary" key="deletions">
				-{pr.deletions}
			</span>,
		);
	}
	return (
		<div className="flex min-w-0 flex-wrap items-center gap-x-1.5 tabular-nums text-wd-secondary">
			{parts.map((part, index) => (
				<Fragment key={index}>
					{index > 0 ? <span className="text-wd-tertiary">·</span> : null}
					{part}
				</Fragment>
			))}
		</div>
	);
}

export function PRCardStatusSummary({
	action,
	className,
	externalLink,
	omit,
	presentation,
	reviewDetailsAction,
}: {
	action?: ReactNode;
	className?: string;
	externalLink: ExternalLinkComponent;
	omit?: PRSummaryPartKey[];
	presentation: PRCardPresentation;
	reviewDetailsAction?: ReactNode;
}) {
	const omitted = omit ? new Set(omit) : undefined;
	const supporting = presentation.supporting.filter(
		(status) => status.key === "lifecycle" || !omitted?.has(status.key),
	);
	const rows = presentation.statusRows?.filter((status) => status.key === "lifecycle" || !omitted?.has(status.key));
	if (rows && presentation.readiness) {
		return (
			<div className={cn("hairline-t border-wd-hairline pt-2", className)}>
				<div className="grid min-w-0 grid-cols-1 gap-y-1.5">
					{rows.map((status) => (
						<div className="min-w-0" key={status.key}>
							<div className={cn("flex min-w-0 items-center gap-2 text-xs font-medium leading-4", toneClass[status.tone])}>
								<PRStatusMark breathe={status.breathe} tone={status.tone} />
								<PRCardStatusLink externalLink={externalLink} status={status} />
							</div>
							{status.detail || (status.key === "review" && reviewDetailsAction) ? (
								<div className="mt-0.5 flex min-w-0 items-baseline gap-2 pl-5 text-2xs leading-4">
									{status.detail ? <span className="text-wd-secondary">{status.detail}</span> : null}
									{status.key === "review" && reviewDetailsAction ? reviewDetailsAction : null}
								</div>
							) : null}
						</div>
					))}
				</div>
				<div className="mt-2 hairline-t border-wd-hairline pt-2">
					<div className="flex min-w-0 items-center justify-between gap-3">
						<div className="min-w-0">
							<div className={cn("flex items-center gap-2 text-xs font-medium leading-4", toneClass[presentation.readiness.tone])}>
								<PRStatusMark tone={presentation.readiness.tone} />
								{presentation.readiness.label}
							</div>
							<div className="mt-0.5 min-w-0 break-words pl-5 text-2xs leading-4 text-wd-secondary">{presentation.readiness.detail}</div>
						</div>
						{action ? <div className="shrink-0 self-center">{action}</div> : null}
					</div>
				</div>
			</div>
		);
	}
	return (
		<div className={cn("hairline-t border-wd-hairline pt-2", className)}>
			<div className="flex min-w-0 items-center gap-3">
				<div className="flex min-w-0 flex-1 flex-col gap-1.5">
					<div className="flex min-w-0 items-start gap-2">
						<PRStatusMark
							breathe={presentation.primary.breathe}
							className={toneClass[presentation.primary.tone]}
							tone={presentation.primary.tone}
						/>
						<div className="min-w-0 flex-1">
							<div
								className={cn(
									"text-xs font-medium leading-4",
									toneClass[presentation.primary.tone],
								)}
							>
								<PRCardStatusLink externalLink={externalLink} status={presentation.primary} />
							</div>
							{presentation.primary.detail ? (
								<div className="mt-0.5 min-w-0 break-words text-2xs leading-4 text-wd-secondary">
									{presentation.primary.detail}
								</div>
							) : null}
							{presentation.primary.links.length > 0 ? (
								<div className="mt-1 flex min-w-0 flex-wrap gap-x-1.5 gap-y-1 text-2xs">
									{presentation.primary.links.slice(0, 3).map((link, index) => (
										<SummaryLink
											className={toneClass[presentation.primary.tone]}
											externalLink={externalLink}
											interactive
											key={`${presentation.primary.key}-${index}-${link.label}`}
											link={link}
										/>
									))}
								</div>
							) : null}
						</div>
					</div>
					{supporting.length > 0 ? (
						<div className="min-w-0 pl-5">
							<div className="flex min-w-0 flex-wrap gap-x-3 gap-y-1 text-2xs">
								{supporting.map((status) => (
									<span
										className={cn("inline-flex items-center gap-1", toneClass[status.tone])}
										key={status.key}
									>
										<PRStatusMark breathe={status.breathe} tone={status.tone} />
										<PRCardStatusLink externalLink={externalLink} status={status} />
									</span>
								))}
							</div>
						</div>
					) : null}
				</div>
				{action ? <div className="shrink-0 self-center">{action}</div> : null}
			</div>
		</div>
	);
}

function PRCardStatusLink({
	externalLink: ExternalLink,
	status,
}: {
	externalLink: ExternalLinkComponent;
	status: PRCardStatus;
}) {
	if (!status.href) {
		return status.label;
	}
	return (
		<ExternalLink
			className="inline-flex items-center gap-0.5 underline-offset-2 hover:underline focus-visible:rounded-xs"
			href={status.href}
		>
			{status.label}
			<ArrowUpRightIcon className="size-icon-2xs shrink-0" />
		</ExternalLink>
	);
}

export function PRSummaryParts({
	className,
	countNounLabel = defaultCountNounLabel,
	externalLink,
	interactiveLinks = true,
	maxLinks = 3,
	omit,
	parts,
	variant = "compact",
}: {
	className?: string;
	countNounLabel?: CountNounLabel;
	externalLink: ExternalLinkComponent;
	interactiveLinks?: boolean;
	maxLinks?: number;
	omit?: PRSummaryPartKey[];
	parts: PRSummaryPart[];
	variant?: "compact" | "stacked";
}) {
	const omitted = omit ? new Set(omit) : undefined;
	const visibleParts = parts.filter((part) => !omitted?.has(part.key));
	const stacked = variant === "stacked";
	return (
		<div
			className={cn(
				stacked
					? "flex flex-col gap-1.5 text-2xs leading-4"
					: "flex flex-wrap gap-x-3 gap-y-1 text-2xs",
				className,
			)}
		>
			{visibleParts.map((part) => {
				const links = part.links.slice(0, maxLinks);
				const extra = (part.linkTotal ?? part.links.length) - links.length;
				const overflowLabel =
					extra > 0
						? part.overflowNoun
							? `+${countNounLabel(extra, part.overflowNoun)}`
							: `+${extra}`
						: undefined;
				return (
					<div
						className={cn(
							"min-w-0",
							stacked ? "flex flex-col" : "inline-flex flex-wrap gap-x-1",
						)}
						key={part.key}
					>
						<div className="min-w-0 truncate">
							<span className="text-wd-tertiary">{part.label}</span>{" "}
							<span className={cn("font-medium", toneClass[part.tone])}>{part.status}</span>
							{part.summary ? <span className="text-wd-tertiary"> · {part.summary}</span> : null}
						</div>
						{links.length > 0 || overflowLabel ? (
							<div
								className={cn(
									"flex min-w-0 flex-wrap gap-x-1.5 gap-y-1",
									stacked ? "mt-0.5" : "",
								)}
							>
								{links.map((link, index) => (
									<SummaryLink
										externalLink={externalLink}
										interactive={interactiveLinks}
										key={`${part.key}-${index}-${link.label}`}
										link={link}
									/>
								))}
								{overflowLabel ? <span className="tabular-nums text-wd-tertiary">{overflowLabel}</span> : null}
							</div>
						) : null}
					</div>
				);
			})}
		</div>
	);
}

function SummaryLink({
	className,
	externalLink: ExternalLink,
	interactive,
	link,
}: {
	className?: string;
	externalLink: ExternalLinkComponent;
	interactive: boolean;
	link: PRSummaryLink;
}) {
	if (interactive && link.href) {
		return (
			<ExternalLink
				className={cn(
					"inline-flex max-w-full min-w-0 items-center gap-0.5 text-wd-primary underline-offset-2 hover:underline focus-visible:rounded-xs",
					className,
				)}
				href={link.href}
				stopPropagation
				title={link.title}
			>
				<span className="truncate">{link.label}</span>
				<ArrowUpRightIcon className="h-2.5 w-2.5 shrink-0" />
			</ExternalLink>
		);
	}
	return (
		<span className="max-w-full truncate text-wd-secondary" title={link.title}>
			{link.label}
		</span>
	);
}

export function prBranchRange(
	pr: Pick<PRSummaryMetadata, "sourceBranch" | "targetBranch">,
): string | undefined {
	if (pr.sourceBranch && pr.targetBranch) {
		return `${pr.sourceBranch} → ${pr.targetBranch}`;
	}
	if (pr.sourceBranch) {
		return pr.sourceBranch;
	}
	if (pr.targetBranch) {
		return `→ ${pr.targetBranch}`;
	}
	return undefined;
}

export function hasDiffMetadata(
	pr: Pick<PRSummaryMetadata, "changedFiles" | "additions" | "deletions">,
): boolean {
	return pr.changedFiles > 0 || pr.additions > 0 || pr.deletions > 0;
}
