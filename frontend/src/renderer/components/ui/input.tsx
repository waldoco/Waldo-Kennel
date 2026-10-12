import * as React from "react";
import { cn } from "../../lib/utils";

export const Input = React.forwardRef<HTMLInputElement, React.InputHTMLAttributes<HTMLInputElement>>(
	({ className, type = "text", ...props }, ref) => (
		<input
			data-slot="input"
			className={cn(
				"h-control-form w-full min-w-0 rounded-md border border-transparent bg-wd-inset px-3 py-1 text-sm text-wd-primary transition-[color,background-color,border-color] duration-fast outline-none placeholder:text-wd-tertiary focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-(--border-focus) disabled:pointer-events-none disabled:cursor-not-allowed disabled:text-wd-disabled aria-invalid:border-critical",
				className,
			)}
			ref={ref}
			type={type}
			{...props}
		/>
	),
);

Input.displayName = "Input";
