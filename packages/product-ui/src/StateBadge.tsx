import type { ReactNode } from "react";
import { CheckIcon, ClockIcon, XIcon } from "./icons";
import { cn } from "./utils";

/**
 * Waldo's state vocabulary for Kennel. Each tone carries a shape cue so colour
 * is never the only signal:
 *
 * - `needsYou` — the owner has the next move (Spot dot).
 * - `live` — execution is running right now (system-blue dot; Kennel only).
 * - `neutral` — Waiting, Proposed, Scheduled, Paused, Not run, Unknown (no mark).
 * - `good` — Confirmed, Verified, Accepted, Ready (check).
 * - `caution` — Stale, Partial, Expires soon, In review (clock).
 * - `critical` — Failed, Revoked, Unconfirmed, Exited (cross).
 */
export type StateTone = "needsYou" | "live" | "neutral" | "good" | "caution" | "critical";

const BADGE_TONE: Record<StateTone, string> = {
	needsYou: "bg-spot-subtle text-spot-text",
	live: "bg-wd-inset text-wd-primary",
	neutral: "bg-wd-inset text-wd-secondary",
	good: "bg-good-bg text-good",
	caution: "bg-caution-bg text-caution",
	critical: "bg-critical-bg text-critical",
};

const TEXT_TONE: Record<StateTone, string> = {
	needsYou: "text-wd-primary",
	live: "text-wd-primary",
	neutral: "text-wd-secondary",
	good: "text-good",
	caution: "text-caution",
	critical: "text-critical",
};

/** The tone's shape cue on its own: a dot for Needs you / Running, a check,
 *  clock or cross for the settled tones, nothing for neutral. */
export function StateMark({ tone, className }: { tone: StateTone; className?: string }) {
	if (tone === "needsYou" || tone === "live") {
		return (
			<span
				aria-hidden="true"
				className={cn("size-1.5 shrink-0 rounded-full", tone === "needsYou" ? "bg-spot" : "bg-wd-accent", className)}
				data-state-mark={tone}
			/>
		);
	}
	const glyphClass = cn("size-3 shrink-0", className);
	if (tone === "good") return <CheckIcon className={glyphClass} data-state-mark={tone} />;
	if (tone === "caution") return <ClockIcon className={glyphClass} data-state-mark={tone} />;
	if (tone === "critical") return <XIcon className={glyphClass} data-state-mark={tone} />;
	return null;
}

/** One word of state on its quiet tint (radius-xs). Text, not a button. */
export function StateBadge({
	tone,
	label,
	className,
	icon,
}: {
	tone: StateTone;
	label: string;
	className?: string;
	/** Replaces the tone's default mark when the host has a more specific glyph. */
	icon?: ReactNode;
}) {
	return (
		<span
			className={cn(
				"inline-flex h-5 max-w-full shrink-0 items-center gap-1 whitespace-nowrap rounded-xs px-1.5 text-xs font-medium",
				BADGE_TONE[tone],
				className,
			)}
			data-state-tone={tone}
			data-testid="state-badge"
		>
			{icon ?? <StateMark tone={tone} />}
			<span className="truncate">{label}</span>
		</span>
	);
}

/** A state written inline as text with its mark — for lane headers, rows and
 *  node faces where a tinted badge would be too loud. */
export function StateLabel({
	tone,
	label,
	className,
}: {
	tone: StateTone;
	label: string;
	className?: string;
}) {
	return (
		<span
			className={cn("inline-flex min-w-0 items-center gap-1.5", TEXT_TONE[tone], className)}
			data-state-tone={tone}
		>
			<StateMark tone={tone} />
			<span className="min-w-0 truncate">{label}</span>
		</span>
	);
}

/**
 * Inline facts on one axis: a quiet lead-in word ("Next", "Why now") followed
 * by the sentence in primary text. Used by Outcome cards and Needs You cards.
 */
export type Fact = { label: string; value: ReactNode; key?: string };

export function FactList({ facts, className }: { facts: Fact[]; className?: string }) {
	const visible = facts.filter((fact) => fact.value !== undefined && fact.value !== null && fact.value !== "");
	if (visible.length === 0) return null;
	return (
		<dl className={cn("flex flex-col gap-1 text-sm leading-5", className)}>
			{visible.map((fact) => (
				<div className="min-w-0" key={fact.key ?? fact.label}>
					<dt className="inline text-wd-secondary">{fact.label}: </dt>
					<dd className="inline text-wd-primary">{fact.value}</dd>
				</div>
			))}
		</dl>
	);
}
