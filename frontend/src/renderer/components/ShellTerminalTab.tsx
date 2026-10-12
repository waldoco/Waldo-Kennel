import { SquareTerminal, X } from "lucide-react";
import { type MouseEvent, useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { useTruncatedText } from "../hooks/useTruncatedText";
import type { ShellTerminal } from "../hooks/useShellTerminals";
import { isWindowsPlatform } from "../lib/platform";
import { cn } from "../lib/utils";

type ShellTerminalTabProps = {
	shell: ShellTerminal;
	isActive: boolean;
	onSelect: () => void;
	onClose: () => void;
	/** Visually connects the active tab to a terminal pane directly below it. */
	appearance?: "pill" | "connected";
	/** Commit a new tab title. Omitted where rename is not wired. */
	onRename?: (title: string) => void;
};

// One standalone-shell tab, shared by the session pane's tab strip (CenterPane)
// and the standalone /terminals screen (ShellTerminalsView) so the two never
// drift. Both are Waldo pills: the selected tab sits on bg-selected (never a
// coloured underline); the close control is a quiet round button.
// The full title only becomes the hover tooltip when the strip truncates it.
//
// Renaming happens inline with the platform's native gesture: double-click on
// macOS/Linux, right-click on Windows. Enter or blur commits, Escape cancels,
// and an empty or unchanged name is discarded. The close control is a sibling
// button, not nested inside the tab button - nesting interactive elements is
// invalid HTML and breaks keyboard traversal.
export function ShellTerminalTab({
	shell,
	isActive,
	onSelect,
	onClose,
	appearance = "pill",
	onRename,
}: ShellTerminalTabProps) {
	const { t } = useTranslation();
	const { ref, isTruncated } = useTruncatedText<HTMLButtonElement>(shell.title);
	const [isEditing, setIsEditing] = useState(false);
	const [draft, setDraft] = useState(shell.title);
	const inputRef = useRef<HTMLInputElement | null>(null);
	const lastClickAtRef = useRef(0);
	// Rename gesture per platform: Windows uses right-click (its convention for
	// tab/file renames); macOS and Linux use double-click.
	const renameViaRightClick = isWindowsPlatform();

	useEffect(() => {
		if (isEditing) {
			inputRef.current?.focus();
			inputRef.current?.select();
		}
	}, [isEditing]);

	const beginEdit = () => {
		if (!onRename || isEditing) return;
		setDraft(shell.title);
		setIsEditing(true);
	};

	// Detect a double-click ourselves from two quick clicks rather than relying on
	// the native dblclick event: some trackpad configurations deliver two taps as
	// two separate clicks that never synthesize a dblclick, so onDoubleClick would
	// never fire. Two clicks within 500ms anywhere on the tab start the rename.
	const handleClick = (event: MouseEvent) => {
		onSelect();
		// Roving tab navigation activates the focused tab with HTMLElement.click(),
		// whose detail is 0. It should select, but must not participate in the
		// pointer-only double-click rename gesture.
		if (renameViaRightClick || event.detail === 0) return;
		const now = Date.now();
		const isDoubleClick = now - lastClickAtRef.current < 500;
		lastClickAtRef.current = isDoubleClick ? 0 : now;
		if (isDoubleClick) beginEdit();
	};

	// The rename gesture lives on the whole tab so it works anywhere on the tab,
	// not just precisely on the label. Windows uses right-click; macOS/Linux use
	// the click-timing double-click detector above.
	const containerRenameHandlers = isEditing
		? {}
		: renameViaRightClick
			? {
					onClick: handleClick,
					onContextMenu: (event: MouseEvent) => {
						event.preventDefault();
						beginEdit();
					},
				}
			: { onClick: handleClick, onDoubleClick: beginEdit };

	const commit = () => {
		if (!isEditing) return;
		setIsEditing(false);
		const next = draft.trim();
		if (next && next !== shell.title) onRename?.(next);
	};

	const cancel = () => {
		setIsEditing(false);
		setDraft(shell.title);
	};

	return (
		<span
			className={cn(
				"group relative min-w-shell-tab-min shrink-0 items-center rounded-full transition-colors duration-fast",
				appearance === "connected"
					? "grid h-control-md w-shell-tab-connected grid-cols-[auto_minmax(0,1fr)_auto] self-center pl-2.5 pr-0"
					: "inline-flex gap-1 px-2.5 py-1",
				isActive
					? "bg-wd-selected text-wd-primary"
					: "text-wd-secondary hover:bg-wd-hover hover:text-wd-primary",
			)}
			{...containerRenameHandlers}
		>
			{appearance === "connected" ? (
				<SquareTerminal aria-hidden="true" className="mr-1 size-icon-sm shrink-0 translate-y-px" />
			) : null}
			{isEditing ? (
				<input
					aria-label={t("terminal.rename", { title: shell.title })}
					className={cn(
						"rounded-xs bg-wd-inset px-1 text-control font-medium text-wd-primary outline-2 outline-offset-0 outline-(--border-focus)",
						appearance === "connected" ? "min-w-0 w-full text-left" : "min-w-flex-min max-w-shell-tab-max",
					)}
					onBlur={commit}
					onChange={(event) => setDraft(event.target.value)}
					onKeyDown={(event) => {
						if (event.key === "Enter") {
							event.preventDefault();
							commit();
						} else if (event.key === "Escape") {
							event.preventDefault();
							cancel();
						}
					}}
					ref={inputRef}
					value={draft}
				/>
			) : (
				<button
					ref={ref}
					aria-current={isActive}
					aria-selected={isActive}
					className={cn(
						"select-none truncate text-control transition-colors",
						appearance === "connected" ? "min-w-0 w-full text-left" : "min-w-flex-min max-w-shell-tab-max",
						isActive ? "font-medium text-wd-primary" : "font-normal text-wd-secondary group-hover:text-wd-primary",
					)}
					role="tab"
					tabIndex={isActive ? 0 : -1}
					title={
						isTruncated
							? shell.title
							: t(renameViaRightClick ? "terminal.renameHintRightClick" : "terminal.renameHintDoubleClick", {
									workingDir: shell.workingDir,
								})
					}
					type="button"
				>
					{shell.title}
				</button>
			)}
			<button
				aria-label={t("terminal.closeNamed", { title: shell.title })}
				data-terminal-tab-action
				className={cn(
					"inline-flex h-control-sm shrink-0 items-center justify-center overflow-hidden rounded-full text-wd-tertiary transition-[width,margin,background,color,opacity] duration-fast hover:bg-wd-hover hover:text-wd-primary",
					appearance === "connected"
						? isActive
							? "ml-1 mr-1 w-control-sm opacity-100"
							: "ml-0 mr-1 w-0 opacity-0 group-hover:ml-1 group-hover:w-control-sm group-hover:opacity-100 group-focus-within:ml-1 group-focus-within:w-control-sm group-focus-within:opacity-100"
						: "w-control-sm opacity-0 group-hover:opacity-100 group-focus-within:opacity-100",
				)}
				onClick={(event) => {
					event.stopPropagation();
					onClose();
				}}
				onDoubleClick={(event) => event.stopPropagation()}
				onContextMenu={(event) => event.stopPropagation()}
				title={t("terminal.close")}
				type="button"
			>
				<X aria-hidden="true" className="size-icon-sm" />
			</button>
		</span>
	);
}
