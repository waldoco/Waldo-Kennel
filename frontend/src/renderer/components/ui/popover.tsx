import { Popover as PopoverPrimitive } from "radix-ui";
import { cn } from "../../lib/utils";

export const Popover = PopoverPrimitive.Root;
export const PopoverTrigger = PopoverPrimitive.Trigger;

export function PopoverContent({
	className,
	sideOffset = 6,
	...props
}: React.ComponentProps<typeof PopoverPrimitive.Content>) {
	return (
		<PopoverPrimitive.Portal>
			<PopoverPrimitive.Content
				className={cn(
					"z-overlay rounded-md bg-wd-raised text-wd-primary shadow-md outline-none",
					"origin-(--radix-popover-content-transform-origin)",
					"data-[state=open]:animate-popover-in data-[state=closed]:animate-popover-out motion-reduce:animate-none",
					className,
				)}
				sideOffset={sideOffset}
				{...props}
			/>
		</PopoverPrimitive.Portal>
	);
}
