import type { ReactNode } from "react";
import { cn } from "./utils";

/**
 * Generic status tones, not product-specific states — the host maps its own
 * vocabulary (e.g. "blocked", "mergeable", "needs_input") onto one of these
 * five before rendering. Keeps this primitive portable across Outcome
 * WorkUnit state, PR state, session attention, or anything else typed this way.
 */
export type MissionStatusTone = "neutral" | "info" | "positive" | "warning" | "danger";

const TONE_CLASS: Record<MissionStatusTone, string> = {
	neutral: "border-border text-muted-foreground",
	info: "border-status-working/40 text-status-working",
	positive: "border-success/40 text-success",
	warning: "border-warning/40 text-warning",
	danger: "border-error/40 text-error",
};

function ToneDot({ className }: { className?: string }) {
	return (
		<svg aria-hidden="true" className={cn("size-1.5 shrink-0", className)} viewBox="0 0 8 8">
			<circle cx="4" cy="4" fill="currentColor" r="4" />
		</svg>
	);
}

export type MissionStatusChipProps = {
	tone: MissionStatusTone;
	/** Always rendered as visible text — the tone color is never the only signal. */
	label: string;
	/** Defaults to a plain tone dot. Pass an icon for a more specific glyph (e.g. a check for "positive"). */
	icon?: ReactNode;
	className?: string;
};

/** A typed status chip: tone, icon/glyph, and text together — never color alone. */
export function MissionStatusChip({ tone, label, icon, className }: MissionStatusChipProps) {
	return (
		<span
			className={cn(
				"inline-flex max-w-full items-center gap-1.5 rounded-full border px-2 py-0.5 text-2xs font-medium leading-none",
				TONE_CLASS[tone],
				className,
			)}
			data-tone={tone}
			data-testid="mission-status-chip"
		>
			{icon ?? <ToneDot />}
			<span className="truncate">{label}</span>
		</span>
	);
}
