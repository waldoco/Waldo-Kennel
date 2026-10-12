"use client";

import * as React from "react";
import { Label as LabelPrimitive } from "radix-ui";

import { cn } from "@/lib/utils";

function Label({ className, ...props }: React.ComponentProps<typeof LabelPrimitive.Root>) {
	return (
		<LabelPrimitive.Root
			data-slot="label"
			className={cn(
				"wd-label flex items-center gap-2 text-wd-primary select-none group-data-[disabled=true]:pointer-events-none group-data-[disabled=true]:text-wd-disabled peer-disabled:cursor-not-allowed peer-disabled:text-wd-disabled",
				className,
			)}
			{...props}
		/>
	);
}

export { Label };
