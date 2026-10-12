import { cn } from "@/lib/utils";

// Waldo Skeleton: static blocks on bg-inset. No pulse, no shimmer — pair it
// with a specific present-tense line saying what is being checked.
function Skeleton({ className, ...props }: React.ComponentProps<"div">) {
	return <div data-slot="skeleton" className={cn("rounded-xs bg-wd-inset", className)} {...props} />;
}

export { Skeleton };
