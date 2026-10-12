import { useMemo, useState } from "react";
import { StateMark } from "@pin4sf/kennel-product-ui";
import { Plus, Trash2 } from "lucide-react";
import { useTranslation } from "react-i18next";

import type { components } from "../../../api/schema";
import { Button } from "../ui/button";
import { cn } from "../../lib/utils";

type ContractCriterion = components["schemas"]["ContractCriterionResponse"];
type IntakeAuthority = components["schemas"]["ControllersIntakeAuthority"];
type ProposeRequest = components["schemas"]["ProposeDecompositionRequest"];
type DecompositionRecord = components["schemas"]["DecompositionResponse"];

/** One contributing Outcome being drafted. Local until Propose is pressed. */
type DraftContribution = {
	ref: string;
	title: string;
	goal: string;
	/** One success criterion per line, matching how the contract states them. */
	successCriteria: string;
	review: string;
	claimedCriteria: string[];
};

type DecompositionEditorProps = {
	criteria: ContractCriterion[];
	/** The parent's ceiling. Contributors inherit it; they may narrow, never widen. */
	parentAuthority: IntakeAuthority;
	expectedContractRevision: number;
	/** Seeds the draft when re-opening an existing proposal. */
	existing?: DecompositionRecord;
	/**
	 * Seeds from a REFUSED agent draft. That draft is retained precisely so one
	 * wrong field can be corrected instead of regenerating, which only helps if
	 * it can be reopened here. It takes precedence over `existing`: the refused
	 * draft is what the owner came back to fix.
	 */
	draft?: ProposeRequest;
	pending: boolean;
	failureMessage?: string;
	onPropose: (request: ProposeRequest) => void;
	onCancel: () => void;
};

/**
 * The fail-closed ceiling used when a parent's own is absent. Contributors may
 * narrow their parent's authority and never widen it, so an unknown parent
 * ceiling has to mean "nothing" rather than a guess — the daemon would refuse
 * a widened claim anyway, and refusing here says so before the round trip.
 */
export const NO_AUTHORITY: IntakeAuthority = {
	readWorkspace: false, writeWorkspace: false, executeLocal: false, useNetwork: false,
	commitLocal: false, createPr: false, deploy: false, externalEffect: false,
};

function blankContribution(index: number): DraftContribution {
	return { ref: `c${index + 1}`, title: "", goal: "", successCriteria: "", review: "", claimedCriteria: [] };
}

function seedFrom(
	existing: DecompositionRecord | undefined,
	draft: ProposeRequest | undefined,
	criteria: ContractCriterion[],
): DraftContribution[] {
	if (draft && draft.contributors && draft.contributors.length > 0) {
		return draft.contributors.map((contributor, index) => ({
			ref: contributor.ref || `c${index + 1}`,
			title: contributor.title ?? "",
			goal: contributor.goal ?? "",
			successCriteria: (contributor.successCriteria ?? []).join("\n"),
			review: contributor.review ?? "",
			claimedCriteria: [...(contributor.claimedCriteria ?? [])],
		}));
	}
	if (existing && existing.contributors.length > 0) {
		return existing.contributors.map((contributor) => ({
			ref: contributor.ref,
			title: contributor.title,
			goal: contributor.goal,
			successCriteria: contributor.successCriteria.join("\n"),
			review: contributor.review,
			claimedCriteria: [...contributor.claimedCriteria],
		}));
	}
	// One contributor per criterion is the same shape the daemon derives when
	// asked for nothing — a starting point to correct, not a recommendation.
	return criteria.map((criterion, index) => ({
		ref: `c${index + 1}`,
		title: criterion.text,
		goal: criterion.text,
		successCriteria: criterion.text,
		review: "",
		claimedCriteria: [criterion.criterionId],
	}));
}

/**
 * Author a decomposition before proposing it.
 *
 * Everything here is a DRAFT. The daemon is the only gate: coverage, authority
 * containment, unknown criteria, and dependency cycles are all decided by
 * ProposeDecomposition, and this surface never claims a draft is valid. The
 * coverage hint below is exactly that — a hint that saves a round trip, not a
 * verdict.
 */
export function DecompositionEditor({
	criteria,
	parentAuthority,
	expectedContractRevision,
	existing,
	draft,
	pending,
	failureMessage,
	onPropose,
	onCancel,
}: DecompositionEditorProps) {
	const { t } = useTranslation();
	const [contributions, setContributions] = useState<DraftContribution[]>(() => seedFrom(existing, draft, criteria));
	const [retained, setRetained] = useState<string[]>(() => [...(draft?.retainedCriteria ?? existing?.retainedCriteria ?? [])]);
	const [dependencies, setDependencies] = useState<{ fromRef: string; toRef: string }[]>(
		() =>
			(draft?.dependencies ?? existing?.dependencies ?? []).map((d) => ({
				fromRef: d.fromRef,
				toRef: d.toRef,
			})),
	);
	const [rationale, setRationale] = useState(draft?.rationale ?? existing?.rationale ?? "");

	const claimed = useMemo(() => {
		const owned = new Set<string>();
		for (const contribution of contributions) for (const id of contribution.claimedCriteria) owned.add(id);
		return owned;
	}, [contributions]);

	// Neither claimed nor retained. The daemon refuses on this; showing it here
	// just means you find out before the round trip.
	const unclassified = criteria.filter((c) => !claimed.has(c.criterionId) && !retained.includes(c.criterionId));

	function update(index: number, patch: Partial<DraftContribution>) {
		setContributions((current) => current.map((entry, i) => (i === index ? { ...entry, ...patch } : entry)));
	}

	function toggleClaim(index: number, criterionId: string) {
		const entry = contributions[index];
		const has = entry.claimedCriteria.includes(criterionId);
		update(index, {
			claimedCriteria: has
				? entry.claimedCriteria.filter((id) => id !== criterionId)
				: [...entry.claimedCriteria, criterionId],
		});
	}

	function submit() {
		onPropose({
			expectedContractRevision,
			rationale: rationale.trim(),
			contributors: contributions.map((entry) => ({
				ref: entry.ref,
				title: entry.title.trim(),
				goal: entry.goal.trim(),
				successCriteria: entry.successCriteria.split("\n").map((line) => line.trim()).filter(Boolean),
				review: entry.review.trim(),
				// Contributors inherit the parent's ceiling. Narrowing is a later
				// refinement; widening is refused by the daemon either way.
				authority: parentAuthority,
				claimedCriteria: entry.claimedCriteria,
			})),
			retainedCriteria: retained,
			dependencies,
		});
	}

	return (
		<div className="flex flex-col gap-4" data-testid="decomposition-editor">
			<div className="max-w-xl">
				<h3 className="text-sm font-medium text-wd-primary">{t("outcome.editor.heading")}</h3>
				<p className="text-sm text-wd-secondary">{t("outcome.editor.intro")}</p>
			</div>

			<div className="flex flex-col gap-3">
				{contributions.map((entry, index) => (
					<div className="rounded-lg hairline border-wd-hairline bg-wd-surface p-4" data-testid={`editor-contribution-${entry.ref}`} key={entry.ref}>
						<div className="flex items-start justify-between gap-3">
							<span className="text-xs text-wd-tertiary">{entry.ref}</span>
							<Button
								aria-label={t("outcome.editor.removeContribution")}
								disabled={contributions.length === 1}
								onClick={() => setContributions((c) => c.filter((_, i) => i !== index))}
								size="sm"
								type="button"
								variant="ghost"
							>
								<Trash2 aria-hidden="true" className="size-3.5" />
							</Button>
						</div>
						<EditorField label={t("outcome.editor.title")} onChange={(v) => update(index, { title: v })} value={entry.title} />
						<EditorField label={t("outcome.editor.goal")} onChange={(v) => update(index, { goal: v })} value={entry.goal} />
						<EditorField
							label={t("outcome.editor.criteria")}
							multiline
							onChange={(v) => update(index, { successCriteria: v })}
							value={entry.successCriteria}
						/>
						<EditorField label={t("outcome.editor.review")} onChange={(v) => update(index, { review: v })} value={entry.review} />

						<fieldset className="mt-3">
							<legend className="text-xs text-wd-secondary">{t("outcome.editor.claims")}</legend>
							<div className="mt-1.5 flex flex-col gap-1.5">
								{criteria.map((criterion) => (
									<label className="flex items-start gap-2 text-sm text-wd-primary" key={criterion.criterionId}>
										<input
											checked={entry.claimedCriteria.includes(criterion.criterionId)}
											disabled={retained.includes(criterion.criterionId)}
											onChange={() => toggleClaim(index, criterion.criterionId)}
											type="checkbox"
										/>
										<span className={cn(retained.includes(criterion.criterionId) && "text-wd-tertiary line-through")}>
											{criterion.text}
										</span>
									</label>
								))}
							</div>
						</fieldset>
					</div>
				))}
			</div>

			<Button
				className="self-start"
				onClick={() => setContributions((c) => [...c, blankContribution(c.length)])}
				size="sm"
				type="button"
				variant="secondary"
			>
				<Plus aria-hidden="true" className="size-3.5" />
				{t("outcome.editor.addContribution")}
			</Button>

			<RetainedCriteria criteria={criteria} claimed={claimed} onChange={setRetained} retained={retained} />
			<DependencyEditor contributions={contributions} dependencies={dependencies} onChange={setDependencies} />

			<div className="rounded-lg hairline border-wd-hairline bg-wd-surface p-4">
				<EditorField
					label={t("outcome.editor.rationale")}
					multiline
					onChange={setRationale}
					value={rationale}
				/>
				<p className="mt-1.5 text-xs text-wd-secondary">{t("outcome.editor.rationaleNote")}</p>
			</div>

			{unclassified.length > 0 ? (
				<p className="flex items-start gap-1.5 text-xs text-wd-primary" data-testid="editor-unclassified">
					<StateMark className="mt-0.5 text-caution" tone="caution" />
					<span>{t("outcome.editor.unclassified", { n: unclassified.length })}</span>
				</p>
			) : null}
			{failureMessage ? (
				<p className="text-sm text-critical" data-testid="editor-failure" role="alert">{failureMessage}</p>
			) : null}

			<div className="flex items-center gap-2">
				{/* Discard leads as the quiet choice; the effect-named ink action
				    trails. While proposing, the button stays disabled and busy -
				    no spinner. */}
				<Button onClick={onCancel} type="button" variant="ghost">
					{t("outcome.editor.cancel")}
				</Button>
				<Button aria-busy={pending || undefined} data-testid="editor-propose" disabled={pending} onClick={submit} type="button">
					{t("outcome.editor.propose")}
				</Button>
			</div>
		</div>
	);
}

/** TextField contract: inset fill, radius-md, no heavy border, the system
 *  focus ring. */
const FIELD_CLASS =
	"rounded-md border border-transparent bg-wd-inset px-3 py-2 text-sm text-wd-primary outline-none placeholder:text-wd-tertiary focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-(--border-focus)";

function EditorField({
	label,
	value,
	onChange,
	multiline,
}: {
	label: string;
	value: string;
	onChange: (value: string) => void;
	multiline?: boolean;
}) {
	return (
		<label className="mt-3 flex flex-col gap-1.5">
			<span className="text-xs text-wd-secondary">{label}</span>
			{multiline ? (
				<textarea
					className={FIELD_CLASS}
					onChange={(event) => onChange(event.target.value)}
					rows={3}
					value={value}
				/>
			) : (
				<input
					className={FIELD_CLASS}
					onChange={(event) => onChange(event.target.value)}
					value={value}
				/>
			)}
		</label>
	);
}

/** Criteria the owner keeps. Retention decides WHO proves a criterion. */
function RetainedCriteria({
	criteria,
	claimed,
	retained,
	onChange,
}: {
	criteria: ContractCriterion[];
	claimed: Set<string>;
	retained: string[];
	onChange: (next: string[]) => void;
}) {
	const { t } = useTranslation();
	return (
		<fieldset className="rounded-lg hairline border-wd-hairline bg-wd-surface p-4" data-testid="editor-retained">
			<legend className="sr-only">{t("outcome.editor.retainedHeading")}</legend>
			<p aria-hidden="true" className="text-sm font-medium text-wd-primary">{t("outcome.editor.retainedHeading")}</p>
			<p className="mt-0.5 text-xs text-wd-secondary">{t("outcome.editor.retainedNote")}</p>
			<div className="mt-3 flex flex-col gap-1.5">
				{criteria.map((criterion) => (
					<label className="flex items-start gap-2 text-sm text-wd-primary" key={criterion.criterionId}>
						<input
							checked={retained.includes(criterion.criterionId)}
							// A criterion cannot be both delegated and owner-proved.
							disabled={claimed.has(criterion.criterionId)}
							onChange={(event) =>
								onChange(
									event.target.checked
										? [...retained, criterion.criterionId]
										: retained.filter((id) => id !== criterion.criterionId),
								)
							}
							type="checkbox"
						/>
						<span className={cn(claimed.has(criterion.criterionId) && "text-wd-tertiary")}>{criterion.text}</span>
					</label>
				))}
			</div>
		</fieldset>
	);
}

/** Declared ordering between siblings. Cycles are refused by the daemon. */
function DependencyEditor({
	contributions,
	dependencies,
	onChange,
}: {
	contributions: DraftContribution[];
	dependencies: { fromRef: string; toRef: string }[];
	onChange: (next: { fromRef: string; toRef: string }[]) => void;
}) {
	const { t } = useTranslation();
	const [fromRef, setFromRef] = useState("");
	const [toRef, setToRef] = useState("");
	const refs = contributions.map((c) => c.ref);

	return (
		<fieldset className="rounded-lg hairline border-wd-hairline bg-wd-surface p-4" data-testid="editor-dependencies">
			<legend className="sr-only">{t("outcome.editor.dependenciesHeading")}</legend>
			<p aria-hidden="true" className="text-sm font-medium text-wd-primary">{t("outcome.editor.dependenciesHeading")}</p>
			<p className="mt-0.5 text-xs text-wd-secondary">{t("outcome.editor.dependenciesNote")}</p>
			<ul className="mt-2 flex flex-col gap-1">
				{dependencies.map((dependency) => (
					<li className="flex items-center gap-2 text-sm text-wd-primary" key={`${dependency.fromRef}->${dependency.toRef}`}>
						<span>{t("outcome.editor.dependencyLine", { from: dependency.fromRef, to: dependency.toRef })}</span>
						<Button
							aria-label={t("outcome.editor.removeDependency")}
							onClick={() => onChange(dependencies.filter((d) => d !== dependency))}
							size="sm"
							type="button"
							variant="ghost"
						>
							<Trash2 aria-hidden="true" className="size-3.5" />
						</Button>
					</li>
				))}
			</ul>
			<div className="mt-3 flex flex-wrap items-end gap-2">
				<label className="flex flex-col gap-1.5">
					<span className="text-xs text-wd-secondary">{t("outcome.editor.dependencyFrom")}</span>
					<select className={FIELD_CLASS} onChange={(e) => setFromRef(e.target.value)} value={fromRef}>
						<option value="">—</option>
						{refs.map((ref) => <option key={ref} value={ref}>{ref}</option>)}
					</select>
				</label>
				<label className="flex flex-col gap-1.5">
					<span className="text-xs text-wd-secondary">{t("outcome.editor.dependencyTo")}</span>
					<select className={FIELD_CLASS} onChange={(e) => setToRef(e.target.value)} value={toRef}>
						<option value="">—</option>
						{refs.map((ref) => <option key={ref} value={ref}>{ref}</option>)}
					</select>
				</label>
				<Button
					disabled={fromRef === "" || toRef === "" || fromRef === toRef}
					onClick={() => {
						onChange([...dependencies, { fromRef, toRef }]);
						setFromRef("");
						setToRef("");
					}}
					size="sm"
					type="button"
					variant="secondary"
				>
					{t("outcome.editor.addDependency")}
				</Button>
			</div>
		</fieldset>
	);
}
