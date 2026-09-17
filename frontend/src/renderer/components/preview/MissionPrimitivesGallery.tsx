import { useState } from "react";
import { Bot, Check, User, Users, X } from "lucide-react";
import {
	ActivityChip,
	ApprovalCard,
	BoundedComposer,
	InspectorShell,
	MissionStatusChip,
	SelectionActionBar,
	SessionResponsibilityChip,
	TaskRow,
	type MissionStatusTone,
} from "@pin4sf/kennel-product-ui";

/**
 * Dev/preview-only gallery for the F1 portable primitives. Not reachable from
 * any production route or navigation item — driven entirely by typed
 * fixtures below, no data fetching, no host wiring.
 */

const STATUS_TONES: { tone: MissionStatusTone; label: string }[] = [
	{ tone: "neutral", label: "Idle" },
	{ tone: "info", label: "Executing" },
	{ tone: "positive", label: "Proven" },
	{ tone: "warning", label: "Needs you" },
	{ tone: "danger", label: "Blocked" },
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
			<GallerySection title="MissionStatusChip — tone + icon/text, never color alone">
				<div className="flex flex-wrap gap-2">
					{STATUS_TONES.map(({ tone, label }) => (
						<MissionStatusChip key={tone} label={label} tone={tone} />
					))}
					<MissionStatusChip icon={<Check className="size-icon-2xs" />} label="Verified" tone="positive" />
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
						nextAction={<button className="text-2xs text-primary" type="button">Resume</button>}
						statusChip={<MissionStatusChip label="Needs you" tone="warning" />}
						title="Reconcile fixture (normal, with next action)"
					/>
					<TaskRow
						freshnessLabel="3 weeks ago (stale)"
						statusChip={<MissionStatusChip label="Blocked" tone="danger" />}
						title="Persist schema (stale, no next action)"
					/>
					<TaskRow
						statusChip={<MissionStatusChip label="Unknown" tone="neutral" />}
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
						onChange={setComposerValue}
						onSubmit={() => undefined}
						placeholder="Ask the Supervisor…"
						scopeDisclosure="Scoped to WorkUnit wu-42"
						sourceDisclosure="Posting as codex"
						submitLabel="Send"
						value={composerValue}
					/>
					<BoundedComposer disabled onChange={() => undefined} onSubmit={() => undefined} submitLabel="Send" value="" />
					<BoundedComposer onChange={() => undefined} onSubmit={() => undefined} pending submitLabel="Send" value="Sent already" />
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
							<button className="text-2xs text-error" type="button"><X className="mr-1 inline size-icon-2xs" />Deny</button>
							<button className="text-2xs text-primary" type="button">Accept</button>
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
