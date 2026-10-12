import * as React from "react";
import { Slot } from "@radix-ui/react-slot";
import { cva, type VariantProps } from "class-variance-authority";
import { cn } from "../../lib/utils";

// Waldo buttons are pills (radius-full) with a medium label. `primary` is the
// ink plate (bg-inverse); `spot` is Waldo Spot with an ink label and appears
// at most once per screen, on the one action Waldo wants taken. `critical`
// names a destructive effect. Focus is the system ring: 2px border-focus,
// offset 2px. See DESIGN.md and the Waldo design system's Button contract.
const buttonVariants = cva(
	"group/button inline-flex shrink-0 items-center justify-center gap-1.5 whitespace-nowrap rounded-full hairline border-transparent bg-clip-padding text-sm font-medium transition-[background-color,border-color,color,box-shadow,transform,opacity] duration-fast ease-out outline-none select-none focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-(--border-focus) active:not-aria-[haspopup]:scale-[0.97] disabled:pointer-events-none disabled:bg-wd-inset disabled:text-wd-disabled disabled:border-transparent [&_svg]:pointer-events-none [&_svg]:shrink-0 [&_svg:not([class*='size-'])]:size-4",
	{
		variants: {
			variant: {
				primary: "bg-wd-inverse text-wd-on-inverse hover:opacity-90",
				spot: "bg-spot text-on-spot hover:bg-spot-pressed active:bg-spot-pressed",
				critical: "bg-critical-bg text-critical hover:bg-[color-mix(in_oklab,var(--critical-bg),var(--critical)_10%)]",
				outline:
					"border-wd-strong bg-transparent text-foreground hover:bg-wd-hover aria-expanded:bg-wd-hover aria-expanded:text-foreground",
				secondary:
					"bg-wd-inset text-foreground hover:bg-wd-pressed",
				ghost: "text-foreground hover:bg-wd-hover active:bg-wd-pressed",
				footer:
					"h-(--size-settings-action-height) rounded-(--radius-settings-action) border-[var(--color-border-settings-input)] bg-[var(--color-bg-settings-input)] px-3 text-[length:var(--font-size-md)] leading-5 text-[var(--color-text-settings-row)] hover:bg-[var(--color-bg-settings-input)] hover:opacity-90 active:not-aria-[haspopup]:scale-100 active:not-aria-[haspopup]:translate-y-0 focus-visible:border-[var(--color-border-settings-input)] focus-visible:ring-0 focus-visible:shadow-[0_0_0_2px_var(--bridge-accent-weak)]",
				"footer-primary":
					"h-(--size-settings-action-height) rounded-(--radius-settings-action) border-transparent bg-[var(--color-settings-accent)] px-(--size-settings-footer-button-padding-x) text-[length:var(--font-size-md)] leading-5 text-[var(--color-settings-footer-button-primary-fg)] hover:bg-[var(--color-settings-accent)] hover:opacity-90 active:not-aria-[haspopup]:scale-100 active:not-aria-[haspopup]:translate-y-0 focus-visible:border-transparent focus-visible:ring-0 focus-visible:shadow-[0_0_0_2px_var(--bridge-accent-weak)]",
			},
			size: {
				default: "h-control-form px-4",
				sm: "h-control-md px-3 text-xs",
				lg: "h-11 px-5 text-base",
				icon: "size-control-form",
				"icon-sm": "size-control-md",
				// Neutral size so footer variants own height/padding via tokens.
				none: "",
			},
		},
		defaultVariants: {
			variant: "primary",
			size: "default",
		},
	},
);

export interface ButtonProps
	extends React.ButtonHTMLAttributes<HTMLButtonElement>, VariantProps<typeof buttonVariants> {
	asChild?: boolean;
}

export const Button = React.forwardRef<HTMLButtonElement, ButtonProps>(
	({ asChild = false, className, size, variant, ...props }, ref) => {
		const Comp = asChild ? Slot : "button";
		// Footer chrome sets its own height/padding; don't let the default size
		// token fight those modal action measurements.
		const resolvedSize = variant === "footer" || variant === "footer-primary" ? "none" : size;
		return <Comp className={cn(buttonVariants({ variant, size: resolvedSize, className }))} ref={ref} {...props} />;
	},
);

Button.displayName = "Button";

export { buttonVariants };
