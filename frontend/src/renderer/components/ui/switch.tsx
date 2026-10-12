"use client";

import * as React from "react";
import { Switch as SwitchPrimitive } from "radix-ui";

import { cn } from "@/lib/utils";

type SwitchProps = React.ComponentProps<typeof SwitchPrimitive.Root> & {
	size?: "default" | "sm";
};

// Waldo Switch: the on track is `accent` (system blue, never green, never
// Spot); the off track is bg-pressed; the thumb is Waldo White in both themes.
// The user's finger and the thumb should arrive together. It runs on the fast token with the system's
// ease-out rather than Tailwind's default 150ms/ease-in-out, and takes the
// same press scale as every other pressable. See DESIGN.md -> Motion.
function Switch({ className, size = "default", ...props }: SwitchProps) {
	return (
		<SwitchPrimitive.Root
			data-slot="switch"
			className={cn(
				"peer relative inline-flex shrink-0 cursor-pointer items-center rounded-full border-transparent transition-[background-color,border-color,box-shadow,transform] duration-fast ease-out outline-none active:scale-press after:absolute after:-inset-x-3 after:-inset-y-2 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-(--border-focus) disabled:cursor-not-allowed disabled:opacity-40 data-[state=checked]:bg-wd-accent data-[state=unchecked]:bg-wd-pressed",
				size === "sm" ? "h-4 w-8 border" : "h-6 w-10 border-2",
				className,
			)}
			{...props}
		>
			<SwitchPrimitive.Thumb
				data-slot="switch-thumb"
				className={cn(
					"pointer-events-none block rounded-full bg-[var(--coat-0)] ring-0 transition-transform duration-[var(--duration-instant)] ease-[var(--ease-standard)] data-[state=unchecked]:translate-x-0",
					size === "sm"
						? "size-3 data-[state=checked]:translate-x-4"
						: "size-5 data-[state=checked]:translate-x-4",
				)}
			/>
		</SwitchPrimitive.Root>
	);
}

export { Switch };
