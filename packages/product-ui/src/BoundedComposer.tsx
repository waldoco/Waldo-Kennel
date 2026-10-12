import type { KeyboardEvent, ReactNode } from "react";
import { cn } from "./utils";

export type BoundedComposerProps = {
	value: string;
	onChange: (value: string) => void;
	onSubmit: () => void;
	/** Accessible name of the text field. Required: placeholder text is not an
	 * accessible name, and an unnamed composer is invisible to the AX tree. */
	inputLabel: string;
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
	inputLabel,
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
		<div className={cn("flex flex-col gap-2", className)} data-testid="bounded-composer">
			{scopeDisclosure ? (
				<div className="text-2xs text-wd-tertiary" data-testid="bounded-composer-scope">
					{scopeDisclosure}
				</div>
			) : null}
			{/* Composer contract: one inset field with a hairline; the 2px focus ring
			    draws around the field, so the textarea itself stays unringed. */}
			<div
				className={cn(
					"rounded-md hairline border-wd-hairline bg-wd-inset px-3 py-2",
					"outline-2 outline-offset-2 outline-transparent transition-[outline-color] duration-fast motion-reduce:transition-none",
					"focus-within:outline-(--border-focus)",
				)}
			>
				<textarea
					aria-label={inputLabel}
					className={cn(
						"block min-h-16 w-full resize-none bg-transparent text-xs text-wd-primary placeholder:text-wd-tertiary",
						"focus-visible:outline-none disabled:cursor-not-allowed disabled:text-wd-secondary",
					)}
					disabled={blocked}
					onChange={(event) => onChange(event.target.value)}
					onKeyDown={handleKeyDown}
					placeholder={placeholder}
					value={value}
				/>
			</div>
			<div className="flex items-center justify-between gap-2">
				{sourceDisclosure ? (
					<div className="min-w-0 truncate text-2xs text-wd-tertiary" data-testid="bounded-composer-source">
						{sourceDisclosure}
					</div>
				) : (
					<span />
				)}
				<button
					className={cn(
						"inline-flex h-7 shrink-0 items-center rounded-full bg-wd-inverse px-3 text-2xs font-medium text-wd-on-inverse transition-opacity duration-fast motion-reduce:transition-none",
						"hover:opacity-90 disabled:pointer-events-none disabled:opacity-40",
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
