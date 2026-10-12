import * as TooltipPrimitive from "@radix-ui/react-tooltip";
import { cn } from "../../lib/utils";

export const TooltipProvider = TooltipPrimitive.Provider;
export const Tooltip = TooltipPrimitive.Root;
export const TooltipTrigger = TooltipPrimitive.Trigger;

export function TooltipContent({
	className,
	sideOffset = 6,
	...props
}: React.ComponentPropsWithoutRef<typeof TooltipPrimitive.Content>) {
	return (
		<TooltipPrimitive.Portal>
			<TooltipPrimitive.Content
				className={cn(
					"z-overlay rounded-sm bg-wd-inverse px-2 py-1 text-xs text-wd-on-inverse",
					// Tooltips are the one flipped surface (inverse ink). Fade from the trigger.
					"origin-(--radix-tooltip-content-transform-origin)",
					"data-[state=delayed-open]:animate-tooltip-in data-[state=closed]:animate-tooltip-out",
					// instant-open means the user already has a tooltip open and moved
					// to a neighbour. The delay has been paid once; replaying the
					// entrance on every neighbour makes a toolbar feel sticky.
					"data-[state=instant-open]:animate-none",
					"motion-reduce:animate-none",
					className,
				)}
				sideOffset={sideOffset}
				{...props}
			/>
		</TooltipPrimitive.Portal>
	);
}
