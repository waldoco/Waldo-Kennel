import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";
import { ArrowUp, Check, ChevronDown, Loader2, Mic, Plus } from "lucide-react";
import { useEffect, useMemo, useRef, useState, type FormEvent, type KeyboardEvent, type ReactNode } from "react";
import { Trans, useTranslation } from "react-i18next";

import type { components } from "../../../api/schema";
import { intakeAnalysisRequestQueryKey, proposalProvenance, useIntakeAnalysisRequest } from "../../hooks/useIntakeAnalysisRequest";
import { useWorkspaceQuery } from "../../hooks/useWorkspaceQuery";
import { projectOutcomesQueryKey } from "../../hooks/useOutcome";
import { apiClient, apiErrorMessage, hasTrustedApiBaseUrl } from "../../lib/api-client";
import { usesPreviewWorkspaceData } from "../../lib/preview-mode";
import { createPreviewOutcome } from "../../lib/preview-outcome-store";
import { Button } from "../ui/button";
import { OutcomeIntakeAgentRoles } from "./OutcomeIntakeAgentRoles";
import { IntakeAnalysisRefused, IntakeAnalysisWaiting, ProposalProvenanceNote } from "./IntakeAnalysisWaiting";
import { IntakeAuthorityEditor, IntakeContractReview, IntakeProposalSummary, normalizeProposal, proposalProblems } from "./IntakeContractReview";
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from "../ui/dropdown-menu";

type IntakeSnapshot = components["schemas"]["IntakeSnapshotResponse"];
type ProposalInput = components["schemas"]["IntakeProposalInput"];

export function AdaptiveIntakeSurface({ projectId, intakeId }: { projectId: string; intakeId?: string }) {
	const navigate = useNavigate();
	const queryClient = useQueryClient();
	const { t } = useTranslation();
	const [editing, setEditing] = useState(false);
	const [statement, setStatement] = useState("");
	const [answer, setAnswer] = useState("");
	const [cancellationReason, setCancellationReason] = useState("");
	const [snapshot, setSnapshot] = useState<IntakeSnapshot | null>(null);
	const [draft, setDraft] = useState<ProposalInput | null>(null);
	const [initialDraft, setInitialDraft] = useState("");
	const [pending, setPending] = useState(false);
	const [error, setError] = useState<string | null>(null);
	const analyzed = useRef<string | null>(null);
	const captureIntent = useRef<{ statement: string; key: string } | null>(null);
	const openOutcome = (outcomeId: string) => navigate({
		to: "/work",
		search: { project: projectId, portfolio: projectId, stage: "decide_authorize", outcome: outcomeId },
	});
	const confirmationIntent = useRef<{ intakeId: string; revision: number; key: string } | null>(null);

	const query = useQuery({
		queryKey: ["intake", intakeId ?? ""],
		enabled: Boolean(intakeId && hasTrustedApiBaseUrl()),
		// While an analysis is in flight the answer arrives from a spawned
		// agent over a callback, not from anything this client did — so
		// nothing here would ever learn it landed without asking again.
		refetchInterval: (q) => (q.state.data?.session.status === "analyzing" ? 3_000 : false),
		// An agent answers whether or not anyone is looking at this tab, and
		// the waiting screen is exactly where a person walks away. Without
		// this the poll pauses while the document is hidden and the surface
		// sits on a stale "still working" long after the answer landed.
		refetchIntervalInBackground: true,
		queryFn: async () => {
			const { data, error: apiError } = await apiClient.GET("/api/v1/intakes/{intakeId}", { params: { path: { intakeId: intakeId as string } } });
			if (apiError) throw apiError;
			return data.intake;
		},
	});

	// The intake prompt names the project it will create an Outcome in, so the
	// person can see (and correct) where this is landing before they type. The
	// name is a fact from the daemon's project list, never derived from the id.
	const workspaces = useWorkspaceQuery();
	const projects = useMemo(
		() => (workspaces.data ?? []).map((workspace) => ({ id: workspace.id, name: workspace.name })),
		[workspaces.data],
	);
	const projectName = projects.find((project) => project.id === projectId)?.name;

	// An agent may be reading the project to propose the Contract. The daemon
	// keeps the intake in `analyzing` while it works, so "an agent is working"
	// is read from the durable ask beside it rather than from the status.
	const analysisRequest = useIntakeAnalysisRequest(intakeId, {
		poll: snapshot?.session.status === "analyzing",
	});
	const openAsk = analysisRequest.request?.status === "requested" && !analysisRequest.request.expired;
	const refusedAsk =
		analysisRequest.request &&
		(analysisRequest.request.status === "rejected" || analysisRequest.request.status === "expired")
			? analysisRequest.request
			: undefined;

	async function refreshIntake(next?: IntakeSnapshot) {
		if (next) setSnapshot(next);
		await queryClient.invalidateQueries({ queryKey: intakeAnalysisRequestQueryKey(intakeId) });
	}


	/**
	 * Release the intake rather than wait. This is a durable cancellation, not
	 * navigation: leaving the page would abandon an intake that still says an
	 * agent is working on it.
	 */
	async function releaseWhileWaiting() {
		if (!snapshot || !intakeId || pending) return;
		setPending(true); setError(null);
		try {
			if (openAsk) {
				await apiClient.POST("/api/v1/intakes/{intakeId}/analysis-request/cancellation", { params: { path: { intakeId } } });
			}
			const { error: apiError } = await apiClient.POST("/api/v1/intakes/{intakeId}/cancellation", {
				params: { path: { intakeId } },
				body: { expectedProposalRevision: snapshot.session.currentProposalRevision, reason: t("outcome.intake.waiting.releaseReason") },
			});
			if (apiError) throw apiError;
			await navigate({ to: "/work", search: { project: projectId } });
		} catch (cause) { setError(apiErrorMessage(cause)); } finally { setPending(false); }
	}

	/** Ask an agent again, from a refused or expired draft. */
	async function retryAgentAnalysis() {
		if (!snapshot || !intakeId || pending) return;
		setPending(true); setError(null);
		try {
			const { data, error: apiError } = await apiClient.POST("/api/v1/intakes/{intakeId}/analysis", {
				params: { path: { intakeId } },
				body: { expectedProposalRevision: snapshot.session.currentProposalRevision, repositoryToolUse: false },
			});
			if (apiError) throw apiError;
			await refreshIntake(data.intake);
		} catch (cause) { setError(apiErrorMessage(cause)); } finally { setPending(false); }
	}

	useEffect(() => { if (query.data) setSnapshot(query.data); }, [query.data]);
	// A poll that lands while an agent is answering has to invalidate the ask
	// too, so the review screen can say who authored what it is about to show.
	useEffect(() => {
		if (query.data?.session.status === "ready") {
			void queryClient.invalidateQueries({ queryKey: intakeAnalysisRequestQueryKey(intakeId) });
		}
	}, [query.data?.session.status, intakeId, queryClient]);
	useEffect(() => {
		if (!snapshot?.proposal) return;
		const next = proposalInput(snapshot);
		setDraft(next);
		setInitialDraft(JSON.stringify(normalizeProposal(next)));
	}, [snapshot?.proposal?.id]);

	useEffect(() => {
		// Only a freshly captured intake analyzes itself. A failed one used to
		// auto-retry here, which was harmless when analysis was a local
		// function and is not now: arriving at a refused draft would spawn
		// another agent immediately, spending real work to re-derive a
		// refusal the owner has not even read yet. Retrying is now a choice
		// they make, beside the reason it failed.
		if (!intakeId || !snapshot || snapshot.session.status !== "captured" || analyzed.current === intakeId) return;
		analyzed.current = intakeId;
		setPending(true); setError(null);
		void apiClient.POST("/api/v1/intakes/{intakeId}/analysis", { params: { path: { intakeId } }, body: { expectedProposalRevision: snapshot.session.currentProposalRevision, repositoryToolUse: false } })
			.then(({ data, error: apiError }) => { if (apiError) throw apiError; setSnapshot(data.intake); })
			.catch((cause) => {
				setError(apiErrorMessage(cause));
				// The daemon has already recorded a durable failure; without
				// re-reading it this surface keeps the pre-analysis snapshot and
				// falls through to a bare error message with nothing to click.
				// Re-reading is what puts the person on the recovery surface,
				// where reasoning setup and explicit retry are available.
				void query.refetch();
			})
			.finally(() => setPending(false));
	}, [intakeId, snapshot]);

	async function capture(event: FormEvent) {
		event.preventDefault();
		if (!statement.trim() || pending) return;
		setPending(true); setError(null);
		try {
			const normalized = statement.trim();
			if (captureIntent.current?.statement !== normalized) captureIntent.current = { statement: normalized, key: requestKey("capture") };
			if (usesPreviewWorkspaceData) {
				// The daemon-backed intake conversation (capture -> analysis ->
				// clarification -> confirm) has no browser-preview equivalent, so a
				// preview session goes straight to a confirmed preview Outcome using
				// the statement verbatim, reusing the same store the rest of the
				// Outcome lifecycle already previews against.
				const outcome = createPreviewOutcome(projectId, {
					title: normalized,
					goal: normalized,
					successCriteria: [normalized],
					review: t("outcome.intake.previewReviewMethod"),
					requestKey: captureIntent.current.key,
				});
				// This preview write bypasses useCreateOutcome, whose invalidation
				// is what refreshes the board's project Outcome queries. Without
				// the same reconcile here the board keeps its pre-create cache and
				// claims "No Outcomes match this view" beside the very Outcome the
				// focused panel is showing.
				void queryClient.invalidateQueries({ queryKey: projectOutcomesQueryKey(projectId) });
				await openOutcome(outcome.id);
				return;
			}
			const { data, error: apiError } = await apiClient.POST("/api/v1/projects/{id}/intakes", { params: { path: { id: projectId } }, body: { sourceSurface: "work", statement: normalized, requestKey: captureIntent.current.key } });
			if (apiError) throw apiError;
			// Captured durably, so the box must not keep it: this surface stays
			// mounted across the intake it just created, and a statement left
			// behind would pre-fill the NEXT Outcome with the last one. Cleared
			// only on success — a rejected capture keeps the text, which is the
			// whole point of saying it was not saved.
			setStatement("");
			captureIntent.current = null;
			await navigate({ to: "/work", search: { project: projectId, intake: data.intake.session.id } });
		} catch (cause) { setError(apiErrorMessage(cause)); } finally { setPending(false); }
	}

	async function answerQuestion(event: FormEvent) {
		event.preventDefault(); if (!snapshot || !intakeId || !answer.trim() || pending) return;
		setPending(true); setError(null);
		try { const { data, error: apiError } = await apiClient.POST("/api/v1/intakes/{intakeId}/clarification", { params: { path: { intakeId } }, body: { expectedProposalRevision: snapshot.session.currentProposalRevision, answer: answer.trim(), repositoryToolUse: false } }); if (apiError) throw apiError; setSnapshot(data.intake); }
		catch (cause) { setError(apiErrorMessage(cause)); } finally { setPending(false); }
	}

	async function confirm() {
		if (!snapshot || !draft || !intakeId || pending) return;
		setPending(true); setError(null);
		try {
			let current = snapshot;
			// Trimmed and blank-stripped before comparison as well as before the
			// send, so trailing whitespace or an emptied list row is not itself
			// treated as a revision worth appending.
			const normalized = normalizeProposal(draft);
			if (JSON.stringify(normalized) !== initialDraft) {
				const revised = await apiClient.POST("/api/v1/intakes/{intakeId}/proposals", { params: { path: { intakeId } }, body: { expectedProposalRevision: current.session.currentProposalRevision, proposal: normalized } });
				if (revised.error) throw revised.error; current = revised.data.intake; setSnapshot(current);
			}
			const revision = current.session.currentProposalRevision;
			if (confirmationIntent.current?.intakeId !== intakeId || confirmationIntent.current.revision !== revision) confirmationIntent.current = { intakeId, revision, key: requestKey("confirm") };
			const confirmed = await apiClient.POST("/api/v1/intakes/{intakeId}/confirmation", { params: { path: { intakeId } }, body: { expectedProposalRevision: revision, requestKey: confirmationIntent.current.key } });
			if (confirmed.error) throw confirmed.error;
			const outcomeId = confirmed.data.intake.confirmedOutcome?.id; if (!outcomeId) throw new Error("The daemon confirmed intake without returning its Outcome.");
			await openOutcome(outcomeId);
		} catch (cause) { setError(apiErrorMessage(cause)); } finally { setPending(false); }
	}

	async function cancel() {
		if (!snapshot || !intakeId || !cancellationReason.trim() || pending) return;
		setPending(true); setError(null);
		try {
			const { data, error: apiError } = await apiClient.POST("/api/v1/intakes/{intakeId}/cancellation", { params: { path: { intakeId } }, body: { expectedProposalRevision: snapshot.session.currentProposalRevision, reason: cancellationReason.trim() } });
			if (apiError) throw apiError;
			setSnapshot(data.intake);
		} catch (cause) { setError(apiErrorMessage(cause)); } finally { setPending(false); }
	}

	if (!usesPreviewWorkspaceData && !hasTrustedApiBaseUrl()) {
		return <TruthMessage title={t("outcome.intake.offlineTitle")} body={t("outcome.intake.offlineBody")} />;
	}
	if (!intakeId) return (
		<form
			className="mx-auto flex h-full w-full max-w-3xl flex-col items-center justify-center gap-8 px-4 py-10 sm:px-8"
			onSubmit={capture}
		>
			{/* A heading, not a <label>: the project name inside it is an
			    interactive switcher, and a control nested in a label would steal
			    the label's click-to-focus. The textarea keeps its own name below. */}
			<h1 className="max-w-2xl text-balance text-center text-2xl font-medium leading-tight tracking-wide-sm text-foreground sm:text-[28px]">
				{projectName ? (
					<Trans
						components={{
							project: <IntakeProjectSwitcher currentProjectId={projectId} projects={projects} />,
						}}
						i18nKey="outcome.intake.promptForProject"
						values={{ project: projectName }}
					/>
				) : (
					t("outcome.intake.prompt")
				)}
			</h1>
			<div className="flex min-h-44 w-full flex-col rounded-lg hairline border-border bg-card p-4 shadow-sm sm:min-h-48 sm:p-5">
				<textarea
					id="outcome-statement"
					data-testid="intake-statement-input"
					aria-label={projectName ? t("outcome.intake.promptForProjectPlain", { project: projectName }) : t("outcome.intake.prompt")}
					autoFocus
					className="min-h-24 w-full flex-1 resize-none bg-transparent text-base leading-relaxed text-foreground outline-none placeholder:text-muted-foreground/70 sm:min-h-28"
					onChange={(event) => setStatement(event.target.value)}
					onKeyDown={(event: KeyboardEvent<HTMLTextAreaElement>) => {
						if ((event.metaKey || event.ctrlKey) && event.key === "Enter") {
							event.preventDefault();
							event.currentTarget.form?.requestSubmit();
						}
					}}
					placeholder={t("outcome.intake.placeholder")}
					value={statement}
				/>
				<div className="mt-3 flex items-end justify-between gap-3">
					{/* Who will do this, decided beside what is being asked for.
					    Writes the project's durable worker/orchestrator agents. */}
					<details className="group relative text-xs text-muted-foreground">
						<summary
							aria-label={t("outcome.intake.agentPreferences")}
							className="flex size-control-md cursor-pointer list-none items-center justify-center rounded-full border border-border bg-background transition-colors hover:bg-muted focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring [&::-webkit-details-marker]:hidden"
						>
							<Plus aria-hidden="true" className="size-4 transition-transform group-open:rotate-45" />
						</summary>
						<div className="absolute bottom-full left-0 z-10 mb-2 flex gap-2 rounded-md border border-border bg-card p-3 shadow-sm"><OutcomeIntakeAgentRoles projectId={projectId} /></div>
					</details>
					<div className="flex shrink-0 items-center gap-2">
						<Button
							aria-label={t("outcome.intake.voiceUnavailable")}
							className="rounded-full"
							disabled
							size="icon-sm"
							type="button"
							variant="ghost"
						>
							<Mic aria-hidden="true" className="size-4" />
						</Button>
						<Button
							aria-label={pending ? t("outcome.intake.saving") : t("outcome.intake.continue")}
							className="rounded-full"
							data-testid="intake-capture-submit"
							disabled={pending || !statement.trim()}
							size="icon-sm"
							type="submit"
						>
							{pending ? <Loader2 aria-hidden="true" className="size-3.5 animate-spin" /> : <ArrowUp aria-hidden="true" className="size-4" />}
						</Button>
					</div>
				</div>
				<p className="sr-only">{t("outcome.intake.hint")}</p>
			</div>
			{error ? (
				<p className="text-sm text-destructive" role="alert">
					{error} {t("outcome.intake.unsaved")}
				</p>
			) : null}
		</form>
	);
	if (query.isLoading && !snapshot) return <TruthMessage title={t("outcome.intake.loadingTitle")} body={t("outcome.intake.loadingBody")} />;
	if (query.error && !snapshot) return <TruthMessage title={t("outcome.intake.unavailableTitle")} body={apiErrorMessage(query.error)} />;
	if (!snapshot) return <TruthMessage title={t("outcome.intake.unavailableTitle")} body={t("outcome.intake.noState")} />;
	// An agent is reading the project. This is derived from the durable ask
	// rather than the status, because the daemon deliberately keeps such an
	// intake in `analyzing` instead of adding a second representation of the
	// same fact.
	if (snapshot.session.status === "analyzing") {
		return (
			<IntakeAnalysisWaiting
				onRelease={() => void releaseWhileWaiting()}
				pending={pending}
				request={openAsk ? analysisRequest.request : undefined}
			/>
		);
	}
	// Any failed analysis lands here, whether an agent produced a draft the
	// daemon refused or nothing was ever asked. Setup and explicit retry are
	// available; only the refused one also has a draft to inspect.
	if (snapshot.session.status === "analysis_failed" && !pending) {
		return (
			<IntakeAnalysisRefused
				failureCode={snapshot.session.failureCode}
				onRetry={() => void retryAgentAnalysis()}
				pending={pending}
				request={refusedAsk}
			/>
		);
	}
	if (pending && (snapshot.session.status === "captured" || snapshot.session.status === "analysis_failed")) return <TruthMessage title={t("outcome.intake.analyzingTitle")} body={t("outcome.intake.analyzingBody")} />;
	if (snapshot.session.status === "needs_user" && snapshot.clarification) return <form className="mx-auto flex w-full max-w-2xl flex-col gap-4 px-4 py-8 sm:px-8" onSubmit={answerQuestion}><p className="text-xs font-medium uppercase tracking-wide text-muted-foreground">{t("outcome.intake.question")}</p><h2 className="text-xl font-medium">{snapshot.clarification.question}</h2><p className="text-sm text-muted-foreground">{snapshot.clarification.reason}</p><label className="text-sm font-medium" htmlFor="clarification-answer">{t("outcome.intake.answer")}</label><input id="clarification-answer" autoFocus className="rounded-lg border border-border bg-background px-3 py-2" onChange={(event) => setAnswer(event.target.value)} value={answer} /><p className="text-xs text-muted-foreground">{t("outcome.intake.recommended", { recommendation: snapshot.clarification.recommendation })}</p><Button disabled={pending || !answer.trim()} type="submit">{t("outcome.intake.continue")}</Button><label className="text-sm font-medium" htmlFor="intake-cancellation-reason">{t("outcome.intake.cancelReason")}</label><input id="intake-cancellation-reason" className="rounded-lg border border-border bg-background px-3 py-2" onChange={(event) => setCancellationReason(event.target.value)} value={cancellationReason} /><Button disabled={pending || !cancellationReason.trim()} type="button" variant="outline" onClick={() => void cancel()}>{t("outcome.intake.cancel")}</Button>{error ? <p role="alert" className="text-sm text-destructive">{error}</p> : null}</form>;
	if (snapshot.session.status === "ready" && draft) {
		const problems = proposalProblems(draft);
		return (
			<section className="mx-auto grid w-full max-w-4xl gap-4 px-4 py-6 sm:px-8 lg:grid-cols-[minmax(0,1fr)_18rem]">
				<div className="flex min-w-0 flex-col gap-3">
					<div>
						<p className="text-xs font-medium uppercase tracking-wide text-muted-foreground">{t("outcome.intake.reviewTitle")}</p>
						<p className="mt-1 text-sm text-muted-foreground">{t("outcome.intake.reviewBody")}</p>
						{/* Whether anything analyzed this. An offline proposal and an
						    agent-authored one look identical otherwise, while being
						    worth very different amounts of trust. */}
						<div className="mt-1.5">
							<ProposalProvenanceNote {...proposalProvenance(analysisRequest.request)} />
						</div>
					</div>
					<Button className="self-start" variant="outline" disabled={pending} onClick={() => setEditing(value => !value)}>
						{t(editing ? "outcome.intake.readDraft" : "outcome.intake.editDraft")}
					</Button>
					{editing ? <IntakeContractReview draft={draft} onChange={setDraft} /> : <IntakeProposalSummary draft={draft} />}
				</div>
				<aside className="flex flex-col gap-3 rounded-group hairline border-border bg-card px-4.5 py-3.5 lg:sticky lg:top-4 lg:self-start">
					<div>
						<p className="text-xs font-medium text-foreground">{t("outcome.intake.authorityTitle")}</p>
						<p className="mt-0.5 text-2xs leading-body text-passive">{t("outcome.intake.authorityBody")}</p>
					</div>
					{/* The proposal's real ceiling, editable. This was static prose
					    that merely happened to match what the rule-based analyzer
					    always emitted; a narrowed or model-authored ceiling would
					    have been described wrongly. */}
					<IntakeAuthorityEditor
						readOnly={pending}
						onChange={(authorityCeiling) => setDraft({ ...draft, authorityCeiling })}
						value={draft.authorityCeiling}
					/>
					{problems.length > 0 ? (
						<ul className="flex list-disc flex-col gap-1 pl-4 text-2xs leading-body text-warning" data-testid="intake-problems">
							{problems.map((problem) => (
								<li key={problem}>{t(problem as never)}</li>
							))}
						</ul>
					) : null}
					<Button data-testid="intake-confirm" disabled={pending || problems.length > 0} onClick={() => void confirm()}>
						{pending ? t("outcome.intake.confirming") : t("outcome.intake.confirm")}
					</Button>
					<details className="border-t border-border pt-3">
						<summary className="cursor-pointer text-xs text-muted-foreground">{t("outcome.intake.cancel")}</summary>
						<div className="mt-3 flex flex-col gap-2">
					<label className="text-xs font-medium text-muted-foreground" htmlFor="intake-cancellation-reason">{t("outcome.intake.cancelReason")}</label>
					<input id="intake-cancellation-reason" className="rounded-md hairline border-border bg-background px-2.5 py-1.5 text-xs" onChange={(event) => setCancellationReason(event.target.value)} value={cancellationReason} />
					<Button disabled={pending || !cancellationReason.trim()} variant="outline" onClick={() => void cancel()}>{t("outcome.intake.cancel")}</Button>
						</div>
					</details>
					{error ? <p role="alert" className="text-sm text-destructive">{error}</p> : null}
				</aside>
			</section>
		);
	}
	if (snapshot.session.status === "confirmed" && snapshot.confirmedOutcome) {
		const outcomeId = snapshot.confirmedOutcome.id;
		return (
			<section className="mx-auto flex max-w-2xl flex-col gap-4 p-8">
				<h2 className="text-xl font-medium">{t("outcome.intake.confirmedTitle")}</h2>
				<p className="text-sm text-muted-foreground">{t("outcome.intake.confirmedBody")}</p>
				<Button className="self-start" onClick={() => void openOutcome(outcomeId)}>
					{t("outcome.intake.openOutcome")}
				</Button>
			</section>
		);
	}
	if (snapshot.session.status === "cancelled") return <TruthMessage title={t("outcome.intake.cancelledTitle")} body={snapshot.session.cancellationReason || t("outcome.intake.cancelledBody")} />;
	return <TruthMessage title={t("outcome.intake.attentionTitle")} body={error ?? t("outcome.intake.state", { status: snapshot.session.status })} />;
}

/**
 * The project the intake will create its Outcome in, rendered inline in the
 * prompt and switchable in place. Switching is navigation only — it changes
 * which project the next capture targets and writes nothing.
 */
function IntakeProjectSwitcher({
	children,
	currentProjectId,
	projects,
}: {
	children?: ReactNode;
	currentProjectId: string;
	projects: { id: string; name: string }[];
}) {
	const navigate = useNavigate();
	const { t } = useTranslation();
	return (
		<DropdownMenu>
			<DropdownMenuTrigger asChild>
				<button
					aria-label={t("outcome.intake.switchProject")}
					className="inline-flex items-center gap-1.5 rounded-md border border-border bg-card px-2.5 py-1 text-foreground shadow-sm transition-colors hover:bg-muted focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/70"
					data-testid="intake-project-switcher"
					type="button"
				>
					{children}
					<ChevronDown aria-hidden="true" className="size-4 shrink-0 text-muted-foreground" />
				</button>
			</DropdownMenuTrigger>
			<DropdownMenuContent align="start">
				{projects.map((project) => (
					<DropdownMenuItem
						key={project.id}
						onSelect={() => void navigate({ to: "/work", search: { project: project.id } })}
					>
						<Check
							aria-hidden="true"
							className={project.id === currentProjectId ? "size-icon-sm" : "size-icon-sm opacity-0"}
						/>
						<span className="min-w-0 truncate">{project.name}</span>
					</DropdownMenuItem>
				))}
			</DropdownMenuContent>
		</DropdownMenu>
	);
}

function TruthMessage({ title, body }: { title: string; body: string }) { return <div className="mx-auto flex h-full max-w-xl flex-col justify-center gap-2 px-4 sm:px-8"><h2 className="text-lg font-medium">{title}</h2><p className="text-sm text-muted-foreground">{body}</p></div>; }
function proposalInput(snapshot: IntakeSnapshot): ProposalInput { const proposal = snapshot.proposal as NonNullable<IntakeSnapshot["proposal"]>; return { title: proposal.title, desiredState: proposal.desiredState, criteria: proposal.criteria.map((criterion) => ({ id: criterion.id, text: criterion.text, evidenceExpected: criterion.evidenceExpected })), reviewMethod: proposal.reviewMethod, constraints: proposal.constraints, nonGoals: proposal.nonGoals, authorityCeiling: proposal.authorityCeiling, stopConditions: proposal.stopConditions, clarificationNotes: proposal.clarificationNotes, temporalCondition: proposal.temporalCondition, facets: proposal.facets }; }
function requestKey(prefix: string) { return `${prefix}-${typeof crypto !== "undefined" && "randomUUID" in crypto ? crypto.randomUUID() : `${Date.now()}-${Math.random()}`}`; }
