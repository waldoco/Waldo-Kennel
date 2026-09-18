import { Plus, Trash2 } from "lucide-react";
import { useId } from "react";
import { useTranslation } from "react-i18next";

import type { components } from "../../../api/schema";
import { cn } from "../../lib/utils";
import { Button } from "../ui/button";
import { Switch } from "../ui/switch";

type ProposalInput = components["schemas"]["IntakeProposalInput"];
type IntakeAuthority = components["schemas"]["ControllersIntakeAuthority"];
type IntakeCriterion = components["schemas"]["ControllersIntakeCriterionInput"];
type IntakeFacet = components["schemas"]["ControllersIntakeFacet"];
type FacetKind = NonNullable<IntakeFacet["kind"]>;

/**
 * Every facet kind the daemon admits (`domain.ContractFacetKind.Valid`). Listed
 * rather than derived because the wire enum is the contract: a kind missing
 * here is refused at `Validate()`, so silently offering fewer is better than
 * offering one that fails on save.
 */
export const FACET_KINDS: FacetKind[] = [
	"software",
	"research",
	"design",
	"documentation",
	"investigation",
	"evaluation",
	"operations",
];

/**
 * The authority ceiling in escalating order of consequence, which is also the
 * order the daemon's own brief renders it in. Local reads first, effects the
 * owner cannot take back last — the ordering is the explanation.
 */
const AUTHORITY_KEYS: (keyof IntakeAuthority)[] = [
	"readWorkspace",
	"writeWorkspace",
	"executeLocal",
	"useNetwork",
	"commitLocal",
	"createPr",
	"deploy",
	"externalEffect",
];

const AUTHORITY_LABEL_KEYS: Record<keyof IntakeAuthority, string> = {
	readWorkspace: "outcome.intake.authority.readWorkspace",
	writeWorkspace: "outcome.intake.authority.writeWorkspace",
	executeLocal: "outcome.intake.authority.executeLocal",
	useNetwork: "outcome.intake.authority.useNetwork",
	commitLocal: "outcome.intake.authority.commitLocal",
	createPr: "outcome.intake.authority.createPr",
	deploy: "outcome.intake.authority.deploy",
	externalEffect: "outcome.intake.authority.externalEffect",
};

/**
 * Problems that would make the daemon refuse this proposal, named in the
 * owner's terms.
 *
 * This mirrors `domain.OutcomeContractProposal.Validate` deliberately, and is
 * NOT a second authority: the daemon still validates and still wins. Checking
 * here only means a missing stop condition is said before a round trip rather
 * than after one, at the field that caused it.
 */
export function proposalProblems(draft: ProposalInput): string[] {
	const problems: string[] = [];
	if (!draft.title.trim()) problems.push("outcome.intake.problem.title");
	if (!draft.desiredState.trim()) problems.push("outcome.intake.problem.desiredState");
	if (!draft.reviewMethod.trim()) problems.push("outcome.intake.problem.reviewMethod");
	if (draft.criteria.length === 0) problems.push("outcome.intake.problem.criteriaEmpty");
	if (draft.criteria.some((criterion) => !criterion.text.trim())) {
		problems.push("outcome.intake.problem.criterionBlank");
	}
	if (draft.criteria.some((criterion) => criterion.evidenceExpected.filter((line) => line.trim()).length === 0)) {
		problems.push("outcome.intake.problem.evidenceMissing");
	}
	if (draft.stopConditions.filter((line) => line.trim()).length === 0) {
		problems.push("outcome.intake.problem.stopConditions");
	}
	if (draft.facets.some((facet) => !facet.summary.trim())) problems.push("outcome.intake.problem.facetSummary");
	return problems;
}

/**
 * Drop what the daemon would reject as blank, and collapse an empty temporal
 * condition back to null rather than sending "". The domain treats a present
 * but blank temporal condition as invalid, so an emptied field has to become
 * absent, not empty.
 */
export function normalizeProposal(draft: ProposalInput): ProposalInput {
	const temporal = draft.temporalCondition?.trim();
	return {
		...draft,
		title: draft.title.trim(),
		desiredState: draft.desiredState.trim(),
		reviewMethod: draft.reviewMethod.trim(),
		criteria: draft.criteria.map((criterion) => ({
			...criterion,
			text: criterion.text.trim(),
			evidenceExpected: criterion.evidenceExpected.map((line) => line.trim()).filter(Boolean),
		})),
		constraints: draft.constraints?.map((line) => line.trim()).filter(Boolean),
		nonGoals: draft.nonGoals?.map((line) => line.trim()).filter(Boolean),
		stopConditions: draft.stopConditions.map((line) => line.trim()).filter(Boolean),
		facets: draft.facets.map((facet) => ({
			...facet,
			summary: facet.summary.trim(),
			requirements: facet.requirements?.map((line) => line.trim()).filter(Boolean),
		})),
		temporalCondition: temporal ? temporal : null,
	};
}

/**
 * The Contract proposal, in full, before anything durable exists.
 *
 * Every field the daemon models is shown and editable here. The screen this
 * replaces edited four of them (title, desired state, criterion text, review
 * method) and round-tripped the rest untouched, so a person confirming an
 * Outcome could not see the constraints, non-goals, stop conditions, or
 * per-criterion evidence they were agreeing to — and the authority ceiling was
 * static prose that happened to match what the rule-based analyzer always
 * emitted, rather than the proposal's real ceiling.
 */
export function IntakeContractReview({
	draft,
	onChange,
}: {
	draft: ProposalInput;
	onChange: (next: ProposalInput) => void;
}) {
	const { t } = useTranslation();

	function patch(next: Partial<ProposalInput>) {
		onChange({ ...draft, ...next });
	}

	function patchCriterion(index: number, next: Partial<IntakeCriterion>) {
		patch({ criteria: draft.criteria.map((criterion, i) => (i === index ? { ...criterion, ...next } : criterion)) });
	}

	function patchFacet(index: number, next: Partial<IntakeFacet>) {
		patch({ facets: draft.facets.map((facet, i) => (i === index ? { ...facet, ...next } : facet)) });
	}

	return (
		<div className="flex min-w-0 flex-col gap-2" data-testid="intake-contract-review">
			<Block title={t("outcome.intake.section.identity")}>
				<EditorField
					label={t("outcome.intake.titleField")}
					onChange={(title) => patch({ title })}
					value={draft.title}
				/>
				<EditorField
					label={t("outcome.intake.desiredStateField")}
					multiline
					onChange={(desiredState) => patch({ desiredState })}
					value={draft.desiredState}
				/>
			</Block>

			{/* Criteria carry their own expected evidence, so they cannot be one
			    textarea of lines: reordering or editing text there would silently
			    re-pair evidence with the wrong criterion. */}
			<Block hint={t("outcome.intake.section.criteriaHint")} title={t("outcome.intake.section.criteria")}>
				<div className="flex flex-col gap-2">
					{draft.criteria.map((criterion, index) => (
						<div className="rounded-md hairline border-border bg-background/40 p-3" key={criterion.id ?? index}>
							<div className="flex items-start justify-between gap-2">
								<span className="text-2xs text-passive">
									{t("outcome.intake.criterionIndex", { index: index + 1 })}
								</span>
								<Button
									aria-label={t("outcome.intake.removeCriterion", { index: index + 1 })}
									disabled={draft.criteria.length === 1}
									onClick={() => patch({ criteria: draft.criteria.filter((_, i) => i !== index) })}
									size="sm"
									type="button"
									variant="ghost"
								>
									<Trash2 aria-hidden="true" className="size-3.5" />
								</Button>
							</div>
							<EditorField
								label={t("outcome.intake.criterionText")}
								multiline
								onChange={(text) => patchCriterion(index, { text })}
								value={criterion.text}
							/>
							<StringList
								addLabel={t("outcome.intake.addEvidence")}
								label={t("outcome.intake.evidenceExpected")}
								onChange={(evidenceExpected) => patchCriterion(index, { evidenceExpected })}
								removeLabel={t("outcome.intake.removeEvidence")}
								values={criterion.evidenceExpected}
							/>
						</div>
					))}
				</div>
				<Button
					className="mt-2 self-start"
					onClick={() => patch({ criteria: [...draft.criteria, { text: "", evidenceExpected: [""] }] })}
					size="sm"
					type="button"
					variant="outline"
				>
					<Plus aria-hidden="true" className="size-3.5" />
					{t("outcome.intake.addCriterion")}
				</Button>
			</Block>

			<Block title={t("outcome.intake.section.review")}>
				<EditorField
					label={t("outcome.intake.reviewField")}
					multiline
					onChange={(reviewMethod) => patch({ reviewMethod })}
					value={draft.reviewMethod}
				/>
			</Block>

			<Block hint={t("outcome.intake.section.boundsHint")} title={t("outcome.intake.section.bounds")}>
				<StringList
					addLabel={t("outcome.intake.addConstraint")}
					label={t("outcome.intake.constraints")}
					onChange={(constraints) => patch({ constraints })}
					removeLabel={t("outcome.intake.removeConstraint")}
					values={draft.constraints ?? []}
				/>
				<StringList
					addLabel={t("outcome.intake.addNonGoal")}
					label={t("outcome.intake.nonGoals")}
					onChange={(nonGoals) => patch({ nonGoals })}
					removeLabel={t("outcome.intake.removeNonGoal")}
					values={draft.nonGoals ?? []}
				/>
				<StringList
					addLabel={t("outcome.intake.addStopCondition")}
					label={t("outcome.intake.stopConditions")}
					onChange={(stopConditions) => patch({ stopConditions })}
					removeLabel={t("outcome.intake.removeStopCondition")}
					values={draft.stopConditions}
				/>
			</Block>

			<Block hint={t("outcome.intake.section.facetsHint")} title={t("outcome.intake.section.facets")}>
				<div className="flex flex-col gap-2">
					{draft.facets.map((facet, index) => (
						<div className="rounded-md hairline border-border bg-background/40 p-3" key={index}>
							<div className="flex items-start justify-between gap-2">
								<FacetKindField onChange={(kind) => patchFacet(index, { kind })} value={facet.kind ?? "software"} />
								<Button
									aria-label={t("outcome.intake.removeFacet", { index: index + 1 })}
									onClick={() => patch({ facets: draft.facets.filter((_, i) => i !== index) })}
									size="sm"
									type="button"
									variant="ghost"
								>
									<Trash2 aria-hidden="true" className="size-3.5" />
								</Button>
							</div>
							<EditorField
								label={t("outcome.intake.facetSummary")}
								onChange={(summary) => patchFacet(index, { summary })}
								value={facet.summary}
							/>
							<StringList
								addLabel={t("outcome.intake.addRequirement")}
								label={t("outcome.intake.facetRequirements")}
								onChange={(requirements) => patchFacet(index, { requirements })}
								removeLabel={t("outcome.intake.removeRequirement")}
								values={facet.requirements ?? []}
							/>
						</div>
					))}
				</div>
				<Button
					className="mt-2 self-start"
					onClick={() => patch({ facets: [...draft.facets, { kind: "software", summary: "" }] })}
					size="sm"
					type="button"
					variant="outline"
				>
					<Plus aria-hidden="true" className="size-3.5" />
					{t("outcome.intake.addFacet")}
				</Button>
			</Block>

			<Block hint={t("outcome.intake.section.temporalHint")} title={t("outcome.intake.section.temporal")}>
				<EditorField
					label={t("outcome.intake.temporalCondition")}
					onChange={(value) => patch({ temporalCondition: value })}
					value={draft.temporalCondition ?? ""}
				/>
			</Block>

			{/* Provenance, not an editable field: these record what was clarified
			    on the way to this proposal. Rewriting them would rewrite history. */}
			{draft.clarificationNotes && draft.clarificationNotes.length > 0 ? (
				<Block title={t("outcome.intake.section.clarifications")}>
					<ul className="flex list-disc flex-col gap-1 pl-4 text-xs leading-body text-muted-foreground">
						{draft.clarificationNotes.map((note, index) => (
							<li key={index}>{note}</li>
						))}
					</ul>
				</Block>
			) : null}
		</div>
	);
}

/** Read the same structured draft that confirmation submits; editing is optional. */
export function IntakeProposalSummary({ draft }: { draft: ProposalInput }) {
	const { t } = useTranslation();
	const lines = (values: string[]) =>
		values.length ? (
			<ul className="list-disc space-y-1 pl-4 text-sm">
				{values.map((value, index) => (
					<li className="whitespace-pre-wrap break-words" key={index}>
						{value}
					</li>
				))}
			</ul>
		) : (
			<p className="text-sm text-muted-foreground">{t("mission.none")}</p>
		);
	return (
		<div className="flex flex-col gap-2" data-testid="intake-proposal-summary">
			<Block title={t("outcome.intake.section.identity")}>
				<h2 className="text-base font-medium">{draft.title}</h2>
				<p className="mt-2 whitespace-pre-wrap text-sm">{draft.desiredState}</p>
			</Block>
			<Block title={t("outcome.intake.section.criteria")}>
				{draft.criteria.map((criterion, index) => (
					<div key={criterion.id ?? index} className="mb-3">
						<p className="text-sm font-medium">{criterion.text}</p>
						<p className="mt-1 text-xs text-muted-foreground">{t("outcome.intake.evidenceExpected")}</p>
						{lines(criterion.evidenceExpected)}
					</div>
				))}
			</Block>
			<Block title={t("outcome.intake.section.review")}>
				<p className="whitespace-pre-wrap text-sm">{draft.reviewMethod}</p>
			</Block>
			{(
				[
					["outcome.intake.constraints", draft.constraints ?? []],
					["outcome.intake.nonGoals", draft.nonGoals ?? []],
					["outcome.intake.stopConditions", draft.stopConditions],
					["outcome.intake.section.clarifications", draft.clarificationNotes ?? []],
				] as const
			).map(([label, values]) => (
				<Block key={label} title={t(label)}>
					{lines(values)}
				</Block>
			))}
			<Block title={t("outcome.intake.section.facets")}>
				{draft.facets.map((facet, index) => (
					<div key={index} className="mb-2 text-sm">
						<p>
							{facet.kind} · {facet.summary}
						</p>
						{lines(facet.requirements ?? [])}
					</div>
				))}
			</Block>
			<Block title={t("outcome.intake.section.temporal")}>
				<p className="text-sm">{draft.temporalCondition || t("mission.none")}</p>
			</Block>
		</div>
	);
}

/**
 * The authority ceiling as the eight real flags it is, bound to the draft.
 * Lives beside Confirm because it is the part of the proposal a person is most
 * likely to want narrowed before agreeing to it.
 */
export function IntakeAuthorityEditor({
	value,
	onChange,
	readOnly = false,
}: {
	readOnly?: boolean;
	value: IntakeAuthority;
	onChange: (next: IntakeAuthority) => void;
}) {
	const { t } = useTranslation();
	return (
		<div className="flex flex-col gap-2" data-testid="intake-authority-editor">
			{AUTHORITY_KEYS.map((key) => (
				<label className="flex items-center justify-between gap-3 text-xs" key={key}>
					<span className={cn(value[key] ? "text-foreground" : "text-muted-foreground")}>
						{t(AUTHORITY_LABEL_KEYS[key] as never)}
					</span>
					{readOnly ? (
						<span className="text-xs">{t(value[key] ? "mission.allowed" : "mission.denied")}</span>
					) : (
						<span className="flex items-center gap-2">
						<span>{t(value[key] ? "mission.allowed" : "mission.denied")}</span>
						<Switch
							aria-label={t(AUTHORITY_LABEL_KEYS[key] as never)}
							checked={value[key]}
							onCheckedChange={(checked) => onChange({ ...value, [key]: checked })}
							size="sm"
						/>
						</span>
					)}
				</label>
			))}
		</div>
	);
}

function Block({ title, hint, children }: { title: string; hint?: string; children: React.ReactNode }) {
	return (
		<section className="flex flex-col rounded-lg hairline border-border bg-card px-4 py-3">
			<h3 className="text-xs font-medium text-foreground">{title}</h3>
			{hint ? <p className="mt-0.5 text-2xs leading-body text-passive">{hint}</p> : null}
			<div className="mt-2 flex flex-col">{children}</div>
		</section>
	);
}

function EditorField({
	label,
	value,
	onChange,
	multiline = false,
}: {
	label: string;
	value: string;
	onChange: (value: string) => void;
	multiline?: boolean;
}) {
	const id = useId();
	return (
		<label className="mt-2 flex flex-col gap-1 text-xs font-medium text-muted-foreground" htmlFor={id}>
			{label}
			{multiline ? (
				<textarea
					aria-label={label}
					className="min-h-16 rounded-md hairline border-border bg-background px-2.5 py-2 font-normal text-foreground outline-none focus-visible:ring-2 focus-visible:ring-ring/60"
					id={id}
					onChange={(event) => onChange(event.target.value)}
					value={value}
				/>
			) : (
				<input
					aria-label={label}
					className="rounded-md hairline border-border bg-background px-2.5 py-1.5 font-normal text-foreground outline-none focus-visible:ring-2 focus-visible:ring-ring/60"
					id={id}
					onChange={(event) => onChange(event.target.value)}
					value={value}
				/>
			)}
		</label>
	);
}

/**
 * One editable line per entry rather than a newline-delimited textarea: these
 * lists are sent as arrays, and a textarea would make "one item containing a
 * newline" and "two items" indistinguishable.
 */
function StringList({
	label,
	values,
	onChange,
	addLabel,
	removeLabel,
}: {
	label: string;
	values: string[];
	onChange: (next: string[]) => void;
	addLabel: string;
	removeLabel: string;
}) {
	const { t } = useTranslation();
	return (
		<fieldset className="mt-2 flex flex-col gap-1">
			<legend className="text-xs font-medium text-muted-foreground">{label}</legend>
			{values.length === 0 ? <p className="text-2xs text-passive">{t("outcome.intake.listEmpty")}</p> : null}
			{values.map((entry, index) => (
				<div className="flex items-center gap-1" key={index}>
					<input
						aria-label={`${label} ${index + 1}`}
						className="min-w-0 flex-1 rounded-md hairline border-border bg-background px-2.5 py-1.5 text-xs text-foreground outline-none focus-visible:ring-2 focus-visible:ring-ring/60"
						onChange={(event) => onChange(values.map((line, i) => (i === index ? event.target.value : line)))}
						value={entry}
					/>
					<Button
						aria-label={`${removeLabel} ${index + 1}`}
						onClick={() => onChange(values.filter((_, i) => i !== index))}
						size="sm"
						type="button"
						variant="ghost"
					>
						<Trash2 aria-hidden="true" className="size-3.5" />
					</Button>
				</div>
			))}
			<Button className="self-start" onClick={() => onChange([...values, ""])} size="sm" type="button" variant="ghost">
				<Plus aria-hidden="true" className="size-3.5" />
				{addLabel}
			</Button>
		</fieldset>
	);
}

function FacetKindField({ value, onChange }: { value: FacetKind; onChange: (kind: FacetKind) => void }) {
	const { t } = useTranslation();
	const id = useId();
	return (
		<label className="flex items-center gap-2 text-xs font-medium text-muted-foreground" htmlFor={id}>
			{t("outcome.intake.facetKind")}
			<select
				aria-label={t("outcome.intake.facetKind")}
				className="rounded-md hairline border-border bg-background px-2 py-1 font-normal text-foreground outline-none focus-visible:ring-2 focus-visible:ring-ring/60"
				id={id}
				onChange={(event) => onChange(event.target.value as FacetKind)}
				value={value}
			>
				{FACET_KINDS.map((kind) => (
					<option key={kind} value={kind}>
						{t(`outcome.intake.facetKind.${kind}` as never)}
					</option>
				))}
			</select>
		</label>
	);
}
