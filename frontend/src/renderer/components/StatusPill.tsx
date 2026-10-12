import { cn } from "@/lib/utils";

type StatusPillProps = {
	label: string;
	tone: string;
	breathe?: boolean;
	className?: string;
	leading?: "none" | "normal";
};

/**
 * Waldo state badge — shared by topbar and inspector. One word of state on a
 * faint tint of its tone (radius-xs), with a dot so colour is never the only
 * signal. Live work (`breathe`) is marked by a static halo around the dot, not
 * a pulse: Waldo's working state is calm.
 */
export function StatusPill({ label, tone, breathe, className, leading = "normal" }: StatusPillProps) {
	return (
		<span
			className={cn(
				"inline-flex shrink-0 items-center gap-1.5 whitespace-nowrap rounded-xs px-2 py-1 text-xs font-medium",
				leading === "none" && "leading-none",
				className,
			)}
			data-live={breathe ? "true" : undefined}
			style={{
				color: tone,
				background: `color-mix(in srgb, ${tone} 12%, transparent)`,
			}}
		>
			<span
				aria-hidden="true"
				className={cn("h-1.5 w-1.5 shrink-0 rounded-full", breathe && "status-dot--live")}
				style={{
					background: tone,
					boxShadow: breathe ? `0 0 0 2px color-mix(in srgb, ${tone} 28%, transparent)` : undefined,
				}}
			/>
			{label}
		</span>
	);
}
