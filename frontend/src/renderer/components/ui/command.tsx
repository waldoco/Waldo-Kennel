"use client";

import * as React from "react";
import { Command as CommandPrimitive } from "cmdk";
import * as Dialog from "@radix-ui/react-dialog";
import { Search } from "lucide-react";

import { cn } from "@/lib/utils";

function Command({ className, ...props }: React.ComponentProps<typeof CommandPrimitive>) {
	return (
		<CommandPrimitive
			data-slot="command"
			className={cn(
				"flex w-full flex-col overflow-hidden rounded-xl bg-wd-raised text-wd-primary outline-none",
				className,
			)}
			{...props}
		/>
	);
}

function CommandDialog({
	title = "Command palette",
	description = "Search projects, sessions, PRs, and commands",
	children,
	className,
	commandProps,
	contentProps,
	...props
}: React.ComponentProps<typeof Dialog.Root> & {
	title?: string;
	description?: string;
	className?: string;
	commandProps?: React.ComponentProps<typeof CommandPrimitive>;
	contentProps?: React.ComponentProps<typeof Dialog.Content>;
}) {
	const { className: commandClassName, ...restCommandProps } = commandProps ?? {};
	return (
		<Dialog.Root data-slot="command-dialog" {...props}>
			<Dialog.Portal>
				<Dialog.Overlay
					data-slot="command-dialog-overlay"
					className="dialog-overlay data-[state=open]:animate-overlay-in data-[state=closed]:animate-overlay-out motion-reduce:animate-none"
				/>
				<Dialog.Content
					data-slot="command-dialog-content"
					aria-label={title}
					{...contentProps}
					// No entrance on the panel itself. The palette is summoned by ⌘K
					// dozens of times a session, and an animation on a keyboard action
					// reads as latency between the keypress and the caret being ready.
					// The scrim still fades so the change of context isn't a hard cut.
					// Waldo overlay: raised plane, radius-xl, shadow-overlay, no border.
					className={cn(
						"fixed left-1/2 top-command-palette z-overlay w-command-palette -translate-x-1/2 overflow-hidden rounded-xl bg-wd-raised text-wd-primary shadow-xl outline-none",
						className,
					)}
				>
					<Dialog.Title className="sr-only">{title}</Dialog.Title>
					<Dialog.Description className="sr-only">{description}</Dialog.Description>
					<Command
						className={cn(
							"**:[[cmdk-group-heading]]:px-[var(--size-command-pad-x)] **:[[cmdk-group-heading]]:pt-3 **:[[cmdk-group-heading]]:pb-1 **:[[cmdk-group-heading]]:text-xs **:[[cmdk-group-heading]]:font-medium **:[[cmdk-group-heading]]:text-wd-tertiary **:[[cmdk-group]]:px-0",
							commandClassName,
						)}
						{...restCommandProps}
					>
						{children}
					</Command>
				</Dialog.Content>
			</Dialog.Portal>
		</Dialog.Root>
	);
}

function CommandInput({ className, ...props }: React.ComponentProps<typeof CommandPrimitive.Input>) {
	return (
		<div
			data-slot="command-input-wrapper"
			className="flex items-center gap-3 border-b border-wd-hairline [border-bottom-width:var(--stroke-hairline)] px-[var(--size-command-pad-x)] py-4"
			cmdk-input-wrapper=""
		>
			<Search className="size-5 shrink-0 text-wd-tertiary" aria-hidden="true" />
			<CommandPrimitive.Input
				data-slot="command-input"
				className={cn(
					"flex h-7 w-full bg-transparent text-heading-sm leading-7 text-wd-primary caret-wd-primary outline-none placeholder:text-wd-tertiary disabled:cursor-not-allowed disabled:text-wd-disabled",
					className,
				)}
				{...props}
			/>
		</div>
	);
}

function CommandList({ className, ...props }: React.ComponentProps<typeof CommandPrimitive.List>) {
	return (
		<CommandPrimitive.List
			data-slot="command-list"
			className={cn(
				"max-h-command-palette-list scrollbar-none scroll-py-1 overflow-y-auto overflow-x-hidden overscroll-contain py-1 outline-none",
				className,
			)}
			{...props}
		/>
	);
}

function CommandEmpty({ ...props }: React.ComponentProps<typeof CommandPrimitive.Empty>) {
	return (
		<CommandPrimitive.Empty
			data-slot="command-empty"
			className="py-8 text-center text-sm text-wd-secondary"
			{...props}
		/>
	);
}

function CommandGroup({ className, ...props }: React.ComponentProps<typeof CommandPrimitive.Group>) {
	return (
		<CommandPrimitive.Group data-slot="command-group" className={cn("overflow-hidden pb-1", className)} {...props} />
	);
}

function CommandItem({ className, ...props }: React.ComponentProps<typeof CommandPrimitive.Item>) {
	return (
		<CommandPrimitive.Item
			data-slot="command-item"
			className={cn(
				"relative mx-[var(--size-command-item-inset)] flex cursor-default select-none items-center gap-2.5 rounded-sm py-1.5 pr-2.5 pl-[var(--size-command-item-pad-l)] text-control leading-[length:var(--leading-command-item)] text-wd-primary outline-none transition-colors duration-fast",
				"data-[selected=true]:bg-wd-selected data-[selected=true]:text-wd-primary",
				"data-[disabled=true]:pointer-events-none data-[disabled=true]:text-wd-disabled",
				"[&_svg]:size-3.5 [&_svg]:shrink-0 [&_svg]:text-wd-secondary data-[selected=true]:[&_svg]:text-wd-primary",
				className,
			)}
			{...props}
		/>
	);
}

function CommandFooter({ className, ...props }: React.ComponentProps<"div">) {
	return (
		<div
			data-slot="command-footer"
			className={cn(
				"flex items-center gap-4 border-t border-wd-hairline [border-top-width:var(--stroke-hairline)] px-[var(--size-command-pad-x)] pt-3 pb-3 text-xs text-wd-tertiary",
				className,
			)}
			{...props}
		/>
	);
}

export {
	Command,
	CommandDialog,
	CommandInput,
	CommandList,
	CommandEmpty,
	CommandGroup,
	CommandItem,
	CommandFooter,
};
