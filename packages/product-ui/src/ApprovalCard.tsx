import type { KeyboardEvent } from "react";
import { cn } from "./utils";

export type ApprovalOption = {
	id: string;
	label: string;
	description?: string;
};

export type ApprovalCardProps = {
	/** What is being requested — a fact, never a summary of free text. */
	requestedAction: string;
	reason: string;
	scopeLabel: string;
	/** A bounded set of choices. When present, the primary action requires one to be selected first. */
	options?: ApprovalOption[];
	optionsLabel?: string;
	selectedOptionId?: string;
	onSelectOption?: (id: string) => void;
	primaryLabel: string;
	/** Never called from anything but the primary button — no prose is parsed into an approval. */
	onPrimary: (optionId?: string) => void;
	denyLabel: string;
	onDeny: () => void;
	className?: string;
};

/**
 * A bounded approval surface: requested action, reason, scope, an explicit
 * (optional) set of choices, and two explicit slots — primary and deny.
 * There is no free-text path to approval; the only way `onPrimary` fires is
 * the primary button, and only after any required option is selected.
 */
export function ApprovalCard({
	requestedAction,
	reason,
	scopeLabel,
	options,
	optionsLabel,
	selectedOptionId,
	onSelectOption,
	primaryLabel,
	onPrimary,
	denyLabel,
	onDeny,
	className,
}: ApprovalCardProps) {
	const hasOptions = Boolean(options && options.length > 0);
	const canApprove = !hasOptions || Boolean(selectedOptionId);

	const selectAdjacent = (event: KeyboardEvent<HTMLButtonElement>, index: number) => {
		if (!options) return;
		let next: number;
		if (event.key === "ArrowRight" || event.key === "ArrowDown") next = (index + 1) % options.length;
		else if (event.key === "ArrowLeft" || event.key === "ArrowUp") next = (index - 1 + options.length) % options.length;
		else return;
		event.preventDefault();
		onSelectOption?.(options[next].id);
		event.currentTarget.parentElement?.querySelectorAll<HTMLButtonElement>('[role="radio"]').item(next)?.focus();
	};

	return (
		<section className={cn("flex flex-col gap-3 rounded-md hairline border-border bg-card p-3", className)} data-testid="approval-card">
			<div className="flex flex-col gap-1">
				<p className="text-xs font-medium text-foreground">{requestedAction}</p>
				<p className="text-2xs leading-body text-muted-foreground">{reason}</p>
				<p className="text-2xs text-passive">{scopeLabel}</p>
			</div>

			{hasOptions ? (
				<div
					aria-label={optionsLabel ?? scopeLabel}
					className="flex flex-col gap-1.5"
					data-testid="approval-card-options"
					role="radiogroup"
				>
					{options!.map((option, index) => {
						const selected = option.id === selectedOptionId;
						return (
							<button
								aria-checked={selected}
								className={cn(
									"flex flex-col gap-0.5 rounded-md border px-2.5 py-1.5 text-left text-2xs transition-colors motion-reduce:transition-none",
									"focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/70",
									selected ? "border-primary bg-primary/10 text-foreground" : "border-border text-muted-foreground hover:bg-interactive-hover",
								)}
								key={option.id}
								onClick={() => onSelectOption?.(option.id)}
								onKeyDown={(event) => selectAdjacent(event, index)}
								role="radio"
								tabIndex={selected || (!selectedOptionId && index === 0) ? 0 : -1}
								type="button"
							>
								<span className="font-medium">{option.label}</span>
								{option.description ? <span className="text-passive">{option.description}</span> : null}
							</button>
						);
					})}
				</div>
			) : null}

			<div className="flex items-center justify-end gap-2">
				<button
					className={cn(
						"rounded-md border border-error/40 px-2.5 py-1.5 text-2xs font-medium text-error transition-colors motion-reduce:transition-none",
						"hover:bg-error/10 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/70",
					)}
					data-testid="approval-card-deny"
					onClick={onDeny}
					type="button"
				>
					{denyLabel}
				</button>
				<button
					className={cn(
						"rounded-md bg-primary px-2.5 py-1.5 text-2xs font-medium text-primary-foreground transition-colors motion-reduce:transition-none",
						"hover:bg-primary/80 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/70",
						"disabled:pointer-events-none disabled:opacity-50",
					)}
					data-testid="approval-card-primary"
					disabled={!canApprove}
					onClick={() => onPrimary(selectedOptionId)}
					type="button"
				>
					{primaryLabel}
				</button>
			</div>
		</section>
	);
}
