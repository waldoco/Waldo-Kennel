import { cva, type VariantProps } from "class-variance-authority";
import { cn } from "@/lib/utils";

// Waldo topbar controls: pills and 28px round quiet icon buttons. `primary`
// is the ink plate, `accent`/`feature` the inset (secondary) fill, `icon` the
// quiet round button. Destructive variants name the effect in critical.
const FOCUS = "focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-(--border-focus)";
const topbarButtonVariants = cva(
	`topbar-control inline-flex items-center transition-[background-color,color,border-color,opacity] duration-fast outline-none ${FOCUS} disabled:cursor-not-allowed disabled:bg-wd-inset disabled:text-wd-disabled`,
	{
		variants: {
			variant: {
				primary:
					"topbar-control--primary h-control-lg gap-1.5 rounded-full bg-wd-inverse px-3.5 text-sm font-medium leading-none text-wd-on-inverse hover:opacity-90",
				accent:
					"topbar-control--accent h-control-lg gap-1.5 rounded-full border border-transparent bg-wd-inset px-3.5 text-sm font-medium leading-none text-wd-primary hover:bg-wd-pressed",
				feature:
					"topbar-control--feature h-control-lg gap-1.5 rounded-full border border-transparent bg-wd-inset px-3 text-control font-medium leading-none text-wd-primary hover:bg-wd-pressed",
				icon:
					"topbar-control--icon grid size-control-md place-items-center rounded-full text-wd-secondary hover:bg-wd-hover hover:text-wd-primary active:bg-wd-pressed disabled:bg-transparent",
				kill: "h-control-lg gap-1.5 rounded-full border border-transparent bg-transparent px-3.5 text-sm font-medium leading-none text-critical hover:bg-critical-bg",
				killIcon:
					"topbar-control--icon topbar-control--danger-icon grid size-control-md place-items-center rounded-full text-wd-secondary hover:bg-critical-bg hover:text-critical disabled:bg-transparent",
				killConfirm:
					"h-control-lg gap-1.5 rounded-full border border-transparent bg-critical-bg px-3 text-control font-medium leading-none text-critical hover:bg-[color-mix(in_oklab,var(--critical-bg),var(--critical)_10%)]",
				killCancel:
					"h-control-lg rounded-full px-2.5 text-control font-medium leading-none text-wd-secondary hover:bg-wd-hover hover:text-wd-primary",
			},
		},
		defaultVariants: { variant: "primary" },
	},
);

export function TopbarButton({
	className,
	variant,
	type = "button",
	...props
}: React.ButtonHTMLAttributes<HTMLButtonElement> & VariantProps<typeof topbarButtonVariants>) {
	return <button className={cn(topbarButtonVariants({ variant }), className)} type={type} {...props} />;
}

export function TopbarKillError({ className, ...props }: React.HTMLAttributes<HTMLSpanElement>) {
	return <span className={cn("text-caption text-critical", className)} role="alert" {...props} />;
}

export const topbarHeaderClass =
	"center-panel-titlebar flex h-toolbar shrink-0 items-center gap-3 border-b border-wd-hairline [border-bottom-width:var(--stroke-hairline)] bg-wd-canvas pr-4 z-chrome";

export const topbarProjectLabelClass =
	"text-brand font-medium leading-none text-wd-primary whitespace-nowrap";
