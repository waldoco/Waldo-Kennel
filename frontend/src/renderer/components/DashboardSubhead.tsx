import type { ReactNode } from "react";

// The board subhead: the screen title in Mottle (words only - the count sits
// outside the display run in tabular figures), a quiet one-line subtitle.
export function DashboardSubhead({
	title,
	subtitle,
	count,
	actions,
}: {
	title: string;
	subtitle: string;
	count?: number;
	actions?: ReactNode;
}) {
	return (
		<div className="flex items-center gap-3 px-4.5 pt-5.5">
			<div className="flex min-w-0 items-baseline gap-3">
				<h1 className="wd-display-4 text-wd-primary">{title}</h1>
				{typeof count === "number" && <span className="wd-data text-wd-tertiary">{count}</span>}
				<span className="text-sm text-wd-secondary">{subtitle}</span>
			</div>
			{actions ? <div className="ml-auto flex shrink-0 items-center gap-2">{actions}</div> : null}
		</div>
	);
}
