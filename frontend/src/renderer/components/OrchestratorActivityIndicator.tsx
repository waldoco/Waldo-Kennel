import type { WorkspaceSession } from "../types/workspace";
import { getAgentActivityView } from "../lib/session-presentation";
import { cn } from "../lib/utils";

// A static state dot (Waldo: no pulsing). Live execution reads as the accent
// dot; the word travels with it as the tooltip so colour is never alone.
export function OrchestratorActivityIndicator({ session }: { session: WorkspaceSession }) {
	const activity = getAgentActivityView(session.activity);

	return (
		<span
			aria-hidden="true"
			className={cn("size-dot-sm shrink-0 rounded-full", activity.dotClassName)}
			data-activity={activity.state}
			title={activity.label}
		/>
	);
}
