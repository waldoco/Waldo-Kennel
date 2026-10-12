// State badges: a quiet word on a faint tint (radius-xs). The raw hue appears
// only in the word; colour never carries the state alone (Waldo StateBadge).
import * as React from "react";
import { cn } from "../../lib/utils";

type BadgeVariant = "neutral" | "outline" | "accent" | "spot" | "success" | "warning" | "error";

export function Badge({
	className,
	variant = "neutral",
	...props
}: React.HTMLAttributes<HTMLSpanElement> & { variant?: BadgeVariant }) {
	return (
		<span
			data-slot="badge"
			className={cn(
				"inline-flex h-5 shrink-0 items-center justify-center gap-1 overflow-hidden whitespace-nowrap rounded-xs border border-transparent px-1.5 py-0.5 text-xs font-medium transition-[background-color,border-color,color,box-shadow] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-(--border-focus) [&>svg]:pointer-events-none [&>svg]:size-3",
				variant === "neutral" && "bg-wd-inset text-wd-secondary",
				variant === "outline" && "border-wd-strong text-wd-secondary hover:bg-wd-hover",
				variant === "accent" && "bg-wd-inverse text-wd-on-inverse",
				variant === "spot" && "bg-spot-subtle text-spot-text",
				variant === "success" && "bg-good-bg text-good",
				variant === "warning" && "bg-caution-bg text-caution",
				variant === "error" && "bg-critical-bg text-critical",
				className,
			)}
			{...props}
		/>
	);
}
