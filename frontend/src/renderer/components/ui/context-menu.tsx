import { ContextMenu as ContextMenuPrimitive } from "radix-ui";
import { cn } from "../../lib/utils";

export const ContextMenu = ContextMenuPrimitive.Root;
export const ContextMenuTrigger = ContextMenuPrimitive.Trigger;
export const ContextMenuGroup = ContextMenuPrimitive.Group;
export const ContextMenuPortal = ContextMenuPrimitive.Portal;

export function ContextMenuContent({
	className,
	...props
}: React.ComponentProps<typeof ContextMenuPrimitive.Content>) {
	return (
		<ContextMenuPrimitive.Portal>
			<ContextMenuPrimitive.Content
				className={cn(
					"z-overlay min-w-[10rem] overflow-hidden rounded-md bg-wd-raised p-1 text-wd-primary shadow-md",
					// A context menu is a popover, not a scrim: it fades in at the point
					// that summoned it and fades back out on close.
					"origin-(--radix-context-menu-content-transform-origin)",
					"data-[state=open]:animate-popover-in data-[state=closed]:animate-popover-out motion-reduce:animate-none",
					className,
				)}
				{...props}
			/>
		</ContextMenuPrimitive.Portal>
	);
}

export function ContextMenuItem({
	className,
	inset,
	...props
}: React.ComponentProps<typeof ContextMenuPrimitive.Item> & { inset?: boolean }) {
	return (
		<ContextMenuPrimitive.Item
			className={cn(
				"relative flex cursor-default select-none items-center gap-2.5 rounded-sm px-2.5 py-1.5 text-control outline-none transition-colors duration-fast",
				"text-wd-primary focus:bg-wd-hover data-[state=checked]:bg-wd-selected data-[state=open]:bg-wd-hover data-[disabled]:pointer-events-none data-[disabled]:text-wd-disabled",
				"[&_svg]:size-icon-lg [&_svg]:shrink-0 [&_svg]:text-wd-secondary",
				inset && "pl-8",
				className,
			)}
			{...props}
		/>
	);
}

export function ContextMenuLabel({
	className,
	inset,
	...props
}: React.ComponentProps<typeof ContextMenuPrimitive.Label> & { inset?: boolean }) {
	return (
		<ContextMenuPrimitive.Label
			className={cn(
				"px-2.5 pt-2 pb-1 text-xs font-medium text-wd-tertiary",
				inset && "pl-8",
				className,
			)}
			{...props}
		/>
	);
}

export function ContextMenuSeparator({
	className,
	...props
}: React.ComponentProps<typeof ContextMenuPrimitive.Separator>) {
	return <ContextMenuPrimitive.Separator className={cn("-mx-1 my-1 h-[var(--stroke-hairline)] bg-wd-hairline", className)} {...props} />;
}
