import { useId, type KeyboardEvent } from "react";
import { FactList, StateBadge, type Fact } from "./StateBadge";
import { cn } from "./utils";

export type ApprovalOption = {
	id: string;
	label: string;
	description?: string;
};

export type ApprovalCardProps = {
	/** What is being requested — a fact, never a summary of free text. Worded
	 *  as the action; it is the card's subject and heading. */
	requestedAction: string;
	reason: string;
	/** The exact scope of the change, shown in the card's inset preview. */
	scopeLabel: string;
	/** A bounded set of choices. When present, the primary action requires one to be selected first. */
	options?: ApprovalOption[];
	optionsLabel?: string;
	selectedOptionId?: string;
	onSelectOption?: (id: string) => void;
	/** The exact effect ("Merge the Result", "Add the dependency"), never "Approve". */
	primaryLabel: string;
	/** Never called from anything but the primary button — no prose is parsed into an approval. */
	onPrimary: (optionId?: string) => void;
	denyLabel: string;
	onDeny: () => void;
	/** Host-translated "Needs you" word for the eyebrow badge. Omitted, no badge renders. */
	needsYouLabel?: string;
	/** Trailing eyebrow caption: the expiry and/or request version ("until 08:45 · v3"). */
	caption?: string;
	/** Host-translated lead-in for `reason` ("Why now"). Without it the reason reads as a plain sentence. */
	reasonLabel?: string;
	/** Further facts after the reason — If you approve / If you don't / Based on — only those the data has. */
	facts?: Fact[];
	/** Shown in place of a silent disabled button while no bounded option is chosen. */
	disabledReason?: string;
	/** `ink` by default. `spot` only when this is the single decision on the screen. */
	primaryTone?: "ink" | "spot";
	className?: string;
};

/**
 * A bounded approval surface on Waldo's Needs You anatomy: eyebrow, the exact
 * action as the subject, the exact scope in an inset preview, the facts in
 * words, then the one way out (quiet, leading) and the effect (ink, trailing).
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
	needsYouLabel,
	caption,
	reasonLabel,
	facts,
	disabledReason,
	primaryTone = "ink",
	className,
}: ApprovalCardProps) {
	const subjectId = useId();
	const hasOptions = Boolean(options && options.length > 0);
	const canApprove = !hasOptions || Boolean(selectedOptionId);
	const reasonFacts: Fact[] = reasonLabel ? [{ label: reasonLabel, value: reason, key: "reason" }] : [];
	const allFacts = [...reasonFacts, ...(facts ?? [])];

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
		<section
			aria-labelledby={subjectId}
			className={cn("flex flex-col gap-3 rounded-lg hairline border-wd-hairline bg-wd-surface p-4", className)}
			data-testid="approval-card"
		>
			{needsYouLabel || caption ? (
				<div className="flex min-w-0 items-center justify-between gap-2">
					{needsYouLabel ? <StateBadge label={needsYouLabel} tone="needsYou" /> : <span />}
					{caption ? <span className="truncate text-xs tabular-nums text-wd-tertiary">{caption}</span> : null}
				</div>
			) : null}

			<h3 className="text-base font-medium leading-6 text-wd-primary" id={subjectId}>
				{requestedAction}
			</h3>

			<div className="rounded-md bg-wd-inset p-3 text-sm leading-5 text-wd-primary">{scopeLabel}</div>

			{allFacts.length > 0 ? (
				<FactList facts={allFacts} />
			) : (
				<p className="text-sm leading-5 text-wd-secondary">{reason}</p>
			)}

			{hasOptions ? (
				<div
					aria-label={optionsLabel ?? scopeLabel}
					className="flex flex-col gap-1"
					data-testid="approval-card-options"
					role="radiogroup"
				>
					{options!.map((option, index) => {
						const selected = option.id === selectedOptionId;
						return (
							<button
								aria-checked={selected}
								className={cn(
									"flex items-start gap-2.5 rounded-sm hairline px-3 py-2 text-left text-sm transition-colors duration-fast motion-reduce:transition-none",
									"focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-(--border-focus)",
									selected
										? "border-wd-strong bg-wd-selected text-wd-primary"
										: "border-wd-hairline text-wd-primary hover:bg-wd-hover",
								)}
								key={option.id}
								onClick={() => onSelectOption?.(option.id)}
								onKeyDown={(event) => selectAdjacent(event, index)}
								role="radio"
								tabIndex={selected || (!selectedOptionId && index === 0) ? 0 : -1}
								type="button"
							>
								<span
									aria-hidden="true"
									className={cn(
										"mt-0.5 flex size-4 shrink-0 items-center justify-center rounded-full border-[1.5px]",
										selected ? "border-wd-primary" : "border-wd-strong",
									)}
								>
									{selected ? <span className="size-2 rounded-full bg-wd-inverse" /> : null}
								</span>
								<span className="flex min-w-0 flex-col gap-0.5">
									<span className="font-medium">{option.label}</span>
									{option.description ? <span className="text-xs text-wd-secondary">{option.description}</span> : null}
								</span>
							</button>
						);
					})}
				</div>
			) : null}

			<div className="flex items-center justify-between gap-2">
				<button
					className={cn(
						"inline-flex h-8 items-center rounded-full px-3 text-sm font-medium text-wd-primary transition-colors duration-fast motion-reduce:transition-none",
						"hover:bg-wd-hover focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-(--border-focus)",
					)}
					data-testid="approval-card-deny"
					onClick={onDeny}
					type="button"
				>
					{denyLabel}
				</button>
				<div className="flex min-w-0 items-center gap-2">
					{!canApprove && disabledReason ? (
						<span className="truncate text-xs text-wd-secondary" data-testid="approval-card-disabled-reason">
							{disabledReason}
						</span>
					) : null}
					<button
						className={cn(
							"inline-flex h-8 shrink-0 items-center rounded-full px-3 text-sm font-medium transition-[background-color,opacity] duration-fast motion-reduce:transition-none",
							"focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-(--border-focus)",
							primaryTone === "spot"
								? "bg-spot text-on-spot hover:bg-spot-pressed"
								: "bg-wd-inverse text-wd-on-inverse hover:opacity-90",
							"disabled:pointer-events-none disabled:bg-wd-inset disabled:text-wd-disabled",
						)}
						data-testid="approval-card-primary"
						disabled={!canApprove}
						onClick={() => onPrimary(selectedOptionId)}
						type="button"
					>
						{primaryLabel}
					</button>
				</div>
			</div>
		</section>
	);
}
