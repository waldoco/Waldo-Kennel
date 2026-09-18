import { useState } from "react";
import { Ban, Bot, Check, Circle, CircleAlert, CircleCheck, Loader2, User, Users, X } from "lucide-react";
import {
	ActivityChip,
	ApprovalCard,
	BoundedComposer,
	InspectorShell,
	MissionStatusChip,
	SelectionActionBar,
	SessionResponsibilityChip,
	TaskRow,
} from "@pin4sf/kennel-product-ui";

/**
 * Dev/preview-only gallery for the F1 portable primitives. Not reachable from
 * any production route or navigation item — driven entirely by typed
 * fixtures below, no data fetching, no host wiring.
 */

// The F2 chip takes pre-resolved presentation (classes + icon), so the gallery
// resolves each fixture state here — the same adapter shape WorkUnitListRow uses.
const STATUS_CHIPS: { label: string; className: string; indicatorClassName: string; icon: React.ReactNode }[] = [
	{ label: "Idle", className: "text-status-idle", indicatorClassName: "bg-status-idle", icon: <Circle className="size-icon-2xs" /> },
	{ label: "Executing", className: "text-status-working", indicatorClassName: "bg-status-working", icon: <Loader2 className="size-icon-2xs" /> },
	{ label: "Proven", className: "text-status-merged", indicatorClassName: "bg-status-merged", icon: <CircleCheck className="size-icon-2xs" /> },
	{ label: "Needs you", className: "text-status-needs-you", indicatorClassName: "bg-status-needs-you", icon: <CircleAlert className="size-icon-2xs" /> },
	{ label: "Blocked", className: "text-status-idle", indicatorClassName: "bg-status-idle", icon: <Ban className="size-icon-2xs" /> },
];

function GallerySection({ title, children }: { title: string; children: React.ReactNode }) {
	return (
		<section className="flex flex-col gap-3 border-b border-border/70 pb-6">
			<h2 className="text-xs font-semibold uppercase tracking-wide text-passive">{title}</h2>
			{children}
		</section>
	);
}

export function MissionPrimitivesGallery() {
	const [composerValue, setComposerValue] = useState("");
	const [selectedOptionId, setSelectedOptionId] = useState<string | undefined>();
	const [activityOpen, setActivityOpen] = useState(false);

	return (
		<div className="flex flex-col gap-6 bg-background p-6 text-foreground">
			<GallerySection title="MissionStatusChip — icon + text, never color alone">
				<div className="flex flex-wrap gap-2">
					{STATUS_CHIPS.map((chip) => (
						<MissionStatusChip
							key={chip.label}
							className={chip.className}
							icon={chip.icon}
							indicatorClassName={chip.indicatorClassName}
							label={chip.label}
						/>
					))}
					<MissionStatusChip
						className="text-status-merged"
						icon={<Check className="size-icon-2xs" />}
						indicatorClassName="bg-status-merged"
						label="Verified"
					/>
				</div>
			</GallerySection>

			<GallerySection title="SessionResponsibilityChip — who owns the next move">
				<div className="flex flex-wrap gap-2">
					<SessionResponsibilityChip icon={<Bot className="size-icon-2xs" />} label="Agent working" responsibility="agent" />
					<SessionResponsibilityChip icon={<User className="size-icon-2xs" />} label="Owner must decide" responsibility="owner" />
					<SessionResponsibilityChip icon={<Users className="size-icon-2xs" />} label="Shared" responsibility="shared" />
				</div>
			</GallerySection>

			<GallerySection title="ActivityChip — secondary, informational only">
				<div className="flex flex-wrap gap-3">
					<ActivityChip icon={<Check className="size-icon-2xs" />} label="3 checks passed" />
					<ActivityChip label="2 unresolved comments" />
				</div>
			</GallerySection>

			<GallerySection title="TaskRow — identity, state, freshness, one next-action slot">
				<div className="flex flex-col gap-2">
					<TaskRow
						freshnessLabel="2h ago"
						nextAction={<button className="rounded-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/70 text-2xs text-primary" type="button">Resume</button>}
						statusChip={
							<MissionStatusChip
								className="text-status-needs-you"
								icon={<CircleAlert className="size-icon-2xs" />}
								indicatorClassName="bg-status-needs-you"
								label="Needs you"
							/>
						}
						title="Reconcile fixture (normal, with next action)"
					/>
					<TaskRow
						freshnessLabel="3 weeks ago (stale)"
						statusChip={
							<MissionStatusChip
								className="text-status-idle"
								icon={<Ban className="size-icon-2xs" />}
								indicatorClassName="bg-status-idle"
								label="Blocked"
							/>
						}
						title="Persist schema (stale, no next action)"
					/>
					<TaskRow
						statusChip={
							<MissionStatusChip
								className="text-status-unknown"
								icon={<CircleAlert className="size-icon-2xs" />}
								indicatorClassName="bg-status-unknown"
								label="Unknown"
							/>
						}
						title="Undated WorkUnit (unknown freshness)"
					/>
				</div>
			</GallerySection>

			<GallerySection title="ApprovalCard — bounded options, explicit primary + deny">
				<div className="grid gap-3 md:grid-cols-2">
					<ApprovalCard
						denyLabel="Deny"
						onDeny={() => undefined}
						onPrimary={() => undefined}
						onSelectOption={setSelectedOptionId}
						options={[
							{ id: "merge", label: "Merge", description: "Accept the Result as-is" },
							{ id: "rework", label: "Request rework", description: "Send back for another pass" },
						]}
						primaryLabel="Approve"
						reason="All checks passed; the WorkUnit is ready to close."
						requestedAction="Accept the verified Result"
						scopeLabel="Scope: WorkUnit wu-42 only"
						selectedOptionId={selectedOptionId}
					/>
					<ApprovalCard
						denyLabel="Deny"
						onDeny={() => undefined}
						onPrimary={() => undefined}
						primaryLabel="Approve"
						reason="No bounded choices — a direct yes/no approval."
						requestedAction="Add @xyflow/react to frontend/package.json"
						scopeLabel="Scope: frontend/package.json only"
					/>
				</div>
			</GallerySection>

			<GallerySection title="BoundedComposer — value/submit, disabled vs. pending, disclosure slots">
				<div className="grid gap-3 md:grid-cols-3">
					<BoundedComposer
						inputLabel="Ask the Supervisor"
						onChange={setComposerValue}
						onSubmit={() => undefined}
						placeholder="Ask the Supervisor…"
						scopeDisclosure="Scoped to WorkUnit wu-42"
						sourceDisclosure="Posting as codex"
						submitLabel="Send"
						value={composerValue}
					/>
					<BoundedComposer disabled inputLabel="Composer input (disabled)" onChange={() => undefined} onSubmit={() => undefined} submitLabel="Send" value="" />
					<BoundedComposer inputLabel="Composer input (pending)" onChange={() => undefined} onSubmit={() => undefined} pending submitLabel="Send" value="Sent already" />
				</div>
			</GallerySection>

			<GallerySection title="SelectionActionBar — contextual actions, destructive distinct">
				<div className="flex flex-col gap-2">
					<SelectionActionBar
						actions={[
							{ id: "assign", label: "Assign", onClick: () => undefined },
							{ id: "remove", label: "Remove from Plan", onClick: () => undefined, destructive: true },
						]}
						selectionLabel="2 WorkUnits selected"
					/>
					<SelectionActionBar
						actions={[{ id: "delete", label: "Delete Contract draft", onClick: () => undefined, destructive: true }]}
						selectionLabel="1 draft selected (destructive only)"
					/>
				</div>
			</GallerySection>

			<GallerySection title="InspectorShell — summary, host-controlled disclosure, explicit footer">
				<InspectorShell
					activity={<p className="text-2xs text-muted-foreground">Session started 2h ago; last check ran 10m ago.</p>}
					activityLabel="Activity"
					activityOpen={activityOpen}
					footer={
						<>
							<button className="rounded-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/70 text-2xs text-error" type="button"><X className="mr-1 inline size-icon-2xs" />Deny</button>
							<button className="rounded-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/70 text-2xs text-primary" type="button">Accept</button>
						</>
					}
					onActivityOpenChange={setActivityOpen}
					summary={<p className="text-2xs text-muted-foreground">WorkUnit wu-42 is proven; awaiting Accept.</p>}
					title="WorkUnit wu-42"
				/>
			</GallerySection>
		</div>
	);
}
