import { ChevronRight, Pencil, type LucideIcon } from "lucide-react";
import { useEffect, useRef, useState, type KeyboardEvent, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { cn } from "../../lib/utils";

function SettingsRowLabel({
	icon: Icon,
	label,
	detail,
	destructive,
	disabled,
}: {
	icon?: LucideIcon;
	label: string;
	detail?: ReactNode;
	destructive?: boolean;
	disabled?: boolean;
}) {
	return (
		<div className="flex min-w-0 shrink items-center gap-3">
			{Icon && !destructive ? (
				<span
					className={cn(
						"settings-row-icon grid size-7 shrink-0 place-items-center rounded-sm bg-wd-inset text-wd-secondary",
						disabled && "opacity-(--opacity-inactive)",
					)}
					aria-hidden="true"
				>
					<Icon className="size-4" />
				</span>
			) : null}
			<span className="flex min-w-0 flex-col gap-0.5">
				<span className={cn("wd-body truncate", destructive ? "text-critical" : "text-wd-primary")}>{label}</span>
				{detail ? <span className="wd-body-s truncate text-wd-secondary">{detail}</span> : null}
			</span>
		</div>
	);
}

/**
 * Waldo ListRow: optional icon tile, title, one-line detail, trailing control.
 * A disabled row shows its reason in place of the detail.
 */
export function SettingsRow({
	icon,
	label,
	detail,
	disabledReason,
	children,
	className,
}: {
	icon?: LucideIcon;
	label: string;
	detail?: ReactNode;
	disabledReason?: ReactNode;
	children: ReactNode;
	className?: string;
}) {
	return (
		<div className={cn("settings-row-bar", className)}>
			<SettingsRowLabel icon={icon} label={label} detail={disabledReason ?? detail} disabled={Boolean(disabledReason)} />
			<div className="flex min-w-0 flex-1 items-center justify-end gap-2">{children}</div>
		</div>
	);
}

/** Idle value + pencil; the input bar appears only after click. */
export function SettingsInlineInput({
	id,
	label,
	value,
	onChange,
	placeholder,
	className,
}: {
	id: string;
	label: string;
	value: string;
	onChange: (value: string) => void;
	placeholder?: string;
	className?: string;
}) {
	const { t } = useTranslation();
	const [editing, setEditing] = useState(false);
	const inputRef = useRef<HTMLInputElement>(null);

	useEffect(() => {
		if (!editing) return;
		const input = inputRef.current;
		if (!input) return;
		input.focus();
		input.select();
	}, [editing]);

	const finishEditing = () => setEditing(false);

	const onKeyDown = (event: KeyboardEvent<HTMLInputElement>) => {
		if (event.key === "Enter") {
			event.preventDefault();
			finishEditing();
			return;
		}
		if (event.key === "Escape") {
			event.preventDefault();
			event.stopPropagation();
			finishEditing();
		}
	};

	if (!editing) {
		return (
			<button
				type="button"
				className={cn("settings-inline-edit-trigger", className)}
				aria-label={t("settings.field.edit", { label })}
				onClick={() => setEditing(true)}
			>
				<span className="settings-row-value" title={value || placeholder}>
					{value || placeholder}
				</span>
				<Pencil className="settings-inline-edit-icon" aria-hidden="true" />
			</button>
		);
	}

	return (
		<input
			ref={inputRef}
			id={id}
			aria-label={label}
			className={cn("settings-inline-edit-input", className)}
			value={value}
			onChange={(event) => onChange(event.target.value)}
			onBlur={finishEditing}
			onKeyDown={onKeyDown}
			placeholder={placeholder}
		/>
	);
}

export function SettingsInputRow({
	icon,
	label,
	id,
	value,
	onChange,
	placeholder,
}: {
	icon?: LucideIcon;
	label: string;
	id: string;
	value: string;
	onChange: (value: string) => void;
	placeholder?: string;
}) {
	return (
		<SettingsRow icon={icon} label={label}>
			<SettingsInlineInput id={id} label={label} value={value} onChange={onChange} placeholder={placeholder} />
		</SettingsRow>
	);
}

export function SettingsLinkRow({
	icon,
	label,
	detail,
	value,
	onClick,
	disabled = false,
	disabledReason,
	destructive = false,
}: {
	icon?: LucideIcon;
	label: string;
	detail?: ReactNode;
	/** Current value, shown before the chevron. */
	value?: ReactNode;
	onClick: () => void;
	disabled?: boolean;
	/** Replaces the detail when the row is disabled. */
	disabledReason?: ReactNode;
	/** Sign out, revoke, delete: no icon, critical title, no chevron. */
	destructive?: boolean;
}) {
	const isDisabled = disabled || Boolean(disabledReason);
	return (
		<button
			type="button"
			onClick={onClick}
			disabled={isDisabled}
			className="settings-row-bar settings-link-row w-full text-left transition-colors duration-fast disabled:cursor-not-allowed"
		>
			<SettingsRowLabel
				icon={icon}
				label={label}
				detail={(isDisabled ? disabledReason : undefined) ?? detail}
				destructive={destructive}
				disabled={isDisabled}
			/>
			{destructive ? null : (
				<span className="flex min-w-0 shrink-0 items-center gap-1.5">
					{value ? <span className="settings-row-value">{value}</span> : null}
					<ChevronRight className="size-4 shrink-0 text-wd-tertiary" aria-hidden="true" />
				</span>
			)}
		</button>
	);
}

/** Trailing text input inside a ListRow: compact TextField on bg-inset. */
export const settingsRowInputClass =
	"h-control-form w-56 max-w-[55%] shrink rounded-sm border border-transparent bg-wd-inset px-3 text-right text-sm text-wd-primary placeholder:text-wd-tertiary focus-visible:border-(--border-focus) focus-visible:ring-0 focus-visible:shadow-[inset_0_0_0_var(--stroke-1)_var(--border-focus)] disabled:opacity-100 disabled:text-wd-secondary aria-invalid:border-critical aria-invalid:ring-0";
