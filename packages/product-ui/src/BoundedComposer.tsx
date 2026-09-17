import type { KeyboardEvent, ReactNode } from "react";
import { cn } from "./utils";

export type BoundedComposerProps = {
	value: string;
	onChange: (value: string) => void;
	onSubmit: () => void;
	placeholder?: string;
	disabled?: boolean;
	/** A submit is in flight — distinct from `disabled`, which is a host policy, not activity. */
	pending?: boolean;
	pendingLabel?: string;
	submitLabel: string;
	/** e.g. "Scoped to WorkUnit wu-42" — host-owned disclosure, never invented here. */
	scopeDisclosure?: ReactNode;
	/** e.g. "Posting as codex" — host-owned, never a provider/model policy decision made in this file. */
	sourceDisclosure?: ReactNode;
	className?: string;
};

/**
 * A bounded text composer: value, submit, and two disclosure slots. This file
 * makes no provider, model, or slash-command decision — it only carries text
 * to `onSubmit` and renders whatever scope/source disclosure the host passes.
 */
export function BoundedComposer({
	value,
	onChange,
	onSubmit,
	placeholder,
	disabled = false,
	pending = false,
	pendingLabel,
	submitLabel,
	scopeDisclosure,
	sourceDisclosure,
	className,
}: BoundedComposerProps) {
	const blocked = disabled || pending;
	const canSubmit = !blocked && value.trim().length > 0;

	const submit = () => {
		if (canSubmit) onSubmit();
	};

	const handleKeyDown = (event: KeyboardEvent<HTMLTextAreaElement>) => {
		if (event.key === "Enter" && !event.shiftKey) {
			event.preventDefault();
			submit();
		}
	};

	return (
		<div className={cn("flex flex-col gap-1.5 rounded-md hairline border-border bg-card p-2.5", className)} data-testid="bounded-composer">
			{scopeDisclosure ? (
				<div className="text-2xs text-passive" data-testid="bounded-composer-scope">
					{scopeDisclosure}
				</div>
			) : null}
			<textarea
				className={cn(
					"min-h-16 w-full resize-none bg-transparent text-xs text-foreground placeholder:text-passive",
					"focus-visible:outline-none disabled:opacity-60",
				)}
				disabled={blocked}
				onChange={(event) => onChange(event.target.value)}
				onKeyDown={handleKeyDown}
				placeholder={placeholder}
				value={value}
			/>
			<div className="flex items-center justify-between gap-2">
				{sourceDisclosure ? (
					<div className="min-w-0 truncate text-2xs text-passive" data-testid="bounded-composer-source">
						{sourceDisclosure}
					</div>
				) : (
					<span />
				)}
				<button
					className={cn(
						"shrink-0 rounded-md bg-primary px-2.5 py-1.5 text-2xs font-medium text-primary-foreground transition-colors motion-reduce:transition-none",
						"hover:bg-primary/80 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/70",
						"disabled:pointer-events-none disabled:opacity-50",
					)}
					data-testid="bounded-composer-submit"
					disabled={!canSubmit}
					onClick={submit}
					type="button"
				>
					{pending ? (pendingLabel ?? "Sending…") : submitLabel}
				</button>
			</div>
		</div>
	);
}
