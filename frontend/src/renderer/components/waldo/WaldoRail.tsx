import {
  ArrowUpRight,
  Check,
  ChevronDown,
  Eye,
  LockKeyhole,
  MessageCircle,
  Paperclip,
  ShieldCheck,
  Sparkles,
  X,
} from "lucide-react";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import waldoMark from "../../../../assets/waldo-mark.svg";
import { cn } from "../../lib/utils";
import { Badge } from "../ui/badge";
import { Button } from "../ui/button";
import {
  useWaldoRail,
  type WaldoPreviewEpisode,
  type WaldoPreviewMode,
} from "./WaldoRailContext";
import { NeedsYouBadge, ProjectWaldoConversation } from "./ProjectWaldoConversation";

type WaldoRailProps = {
  contextLabel: string;
  onClose?: () => void;
  onOpenHome?: () => void;
  onReturnToInspector?: () => void;
  presentation?: "destination" | "rail";
  previewEnabled: boolean;
  daemonReady?: boolean;
  outcomeId?: string;
  outcomeTitle?: string;
  projectId?: string;
  projectName?: string;
};

const previewStepStates = ["evidenced", "evidenced", "active", "blocked"] as const;

// Waldo card anatomy: surface, 0.6px hairline, radius-lg, no shadow.
const cardClass = "flex flex-col gap-3 rounded-lg hairline border-wd-hairline bg-wd-surface p-4";
// Inset block: bg-inset, radius-md, no border.
const insetClass = "rounded-md bg-wd-inset p-3";
// Segmented track (inset) with an ink thumb for the selected segment.
const segmentClass =
  "inline-flex items-center justify-center gap-1.5 rounded-full px-3 wd-label text-wd-secondary transition-colors duration-fast ease-out hover:text-wd-primary motion-reduce:transition-none aria-selected:bg-wd-inverse aria-selected:text-wd-on-inverse aria-selected:hover:text-wd-on-inverse";

export function WaldoRail({
  contextLabel,
  onClose,
  onOpenHome,
  onReturnToInspector,
  presentation = "rail",
  previewEnabled,
  daemonReady = false,
  outcomeId,
  outcomeTitle,
  projectId,
  projectName,
}: WaldoRailProps) {
  const { t } = useTranslation();
  const sharedConversation = useWaldoRail().conversation;
  const [localMode, setLocalMode] = useState<WaldoPreviewMode>("conversation");
  const [localEpisode, setLocalEpisode] = useState<WaldoPreviewEpisode>("contextual");
  const [localContextDetached, setLocalContextDetached] = useState(false);
  const [localDraft, setLocalDraft] = useState("");
  const [proposalReviewed, setProposalReviewed] = useState(false);
  const [resultExpanded, setResultExpanded] = useState(false);
  const mode = sharedConversation?.mode ?? localMode;
  const setMode = sharedConversation?.setMode ?? setLocalMode;
  const episode = sharedConversation?.episode ?? localEpisode;
  const contextDetached = sharedConversation?.contextDetached ?? localContextDetached;
  const setContextDetached = sharedConversation?.setContextDetached ?? setLocalContextDetached;
  const draft = sharedConversation?.draft ?? localDraft;
  const setDraft = sharedConversation?.setDraft ?? setLocalDraft;
  const submittedQuestion = sharedConversation?.submittedQuestions[episode];
  const previewSteps = [
    t("waldo.rail.activity.step.context"),
    t("waldo.rail.activity.step.sources"),
    t("waldo.rail.activity.step.prepare"),
    t("waldo.rail.activity.step.wait"),
  ];
  const episodeTitles: Record<WaldoPreviewEpisode, string> = {
    fresh: t("waldo.rail.episode.fresh"),
    contextual: t("waldo.rail.episode.contextual"),
    returning: t("waldo.rail.episode.returning"),
  };
  const entryStateLabels: Record<WaldoPreviewEpisode, string> = {
    fresh: t("waldo.rail.entry.fresh"),
    contextual: t("waldo.rail.entry.contextual"),
    returning: t("waldo.rail.entry.returning"),
  };
  const selectEpisode = (nextEpisode: WaldoPreviewEpisode) => {
    if (sharedConversation) {
      sharedConversation.selectEpisode(nextEpisode);
    } else {
      setLocalEpisode(nextEpisode);
      setLocalContextDetached(false);
    }
    setContextDetached(false);
    setResultExpanded(false);
  };
  const surfaceLabel = projectId
    ? t("waldo.project.badge")
    : previewEnabled
      ? t("waldo.rail.previewBadge")
      : t("waldo.rail.localBadge");

  return (
    <section
      aria-label={t("waldo.rail.aria")}
      className={cn(
        "waldo-rail flex min-h-0 min-w-0 flex-1 flex-col bg-wd-surface text-wd-primary",
        presentation === "destination" && "rounded-lg hairline border-wd-hairline",
      )}
      id="waldo-rail"
    >
      <header className="shrink-0 hairline-b border-wd-hairline px-4 pb-3 pt-3">
        {onReturnToInspector ? (
          <div
            aria-label={t("waldo.rail.workTabs")}
            className="mb-3 inline-flex items-center gap-0.5 rounded-full bg-wd-inset p-0.5"
            role="tablist"
          >
            <button aria-selected="true" className={cn(segmentClass, "h-7")} role="tab" type="button">
              {t("waldo.rail.identity")}
            </button>
            <button
              aria-selected="false"
              className={cn(segmentClass, "h-7")}
              onClick={onReturnToInspector}
              role="tab"
              type="button"
            >
              {t("waldo.rail.inspector")}
            </button>
          </div>
        ) : null}
        <div className="flex items-center gap-3">
          <img alt="" aria-hidden="true" className="size-6 shrink-0 object-contain" data-brand="waldo" src={waldoMark} />
          <div className="min-w-0 flex-1">
            <h2 className="wd-label-l text-wd-primary">{t("waldo.rail.identity")}</h2>
            <p className="truncate wd-caption text-wd-tertiary">
              {surfaceLabel} · {contextLabel}
            </p>
          </div>
          {presentation === "rail" && onClose ? (
            <button
              aria-label={t("waldo.rail.close")}
              className="waldo-rail-close inline-flex size-9 shrink-0 items-center justify-center gap-1.5 rounded-full text-wd-secondary transition-colors duration-fast ease-out hover:bg-wd-hover hover:text-wd-primary active:bg-wd-pressed motion-reduce:transition-none"
              onClick={onClose}
              type="button"
            >
              <span className="waldo-rail-back-label">{t("waldo.rail.back")}</span>
              <X aria-hidden="true" className="size-4" />
            </button>
          ) : null}
        </div>
      </header>

      <div className={cn("min-h-0 flex-1 overflow-y-auto px-4 py-5", projectId && "overflow-hidden p-0")} data-testid="waldo-rail-body">
        {projectId ? (
          <ProjectWaldoConversation daemonReady={daemonReady} onOpenHome={onOpenHome} outcomeId={outcomeId} outcomeTitle={outcomeTitle} projectId={projectId} projectName={projectName || projectId} />
        ) : !previewEnabled ? (
          <div className="flex min-h-full flex-col justify-between gap-8">
            <div className="flex flex-col gap-2">
              <h3 className="wd-title text-wd-primary">{t("waldo.rail.unconfigured.title")}</h3>
              <p className="wd-body-s text-wd-secondary">{t("waldo.rail.unconfigured.description")}</p>
            </div>
            <div className="flex flex-col gap-1 hairline-t border-wd-hairline pt-4">
              <p className="wd-label text-wd-primary">{t("waldo.rail.unconfigured.localTitle")}</p>
              <p className="wd-caption text-wd-secondary">{t("waldo.rail.unconfigured.localDescription")}</p>
            </div>
          </div>
        ) : (
          <div className="flex flex-col gap-4">
            <div aria-label={t("waldo.rail.previewBoundaryLabel")} className={cn(insetClass, "flex flex-col gap-1")} role="status">
              <p className="wd-label text-wd-primary">{t("waldo.rail.previewBoundaryTitle")}</p>
              <p className="wd-caption text-wd-secondary">{t("waldo.rail.previewBoundaryDescription")}</p>
            </div>

            <div
              aria-label={t("waldo.rail.modeLabel")}
              className="grid grid-cols-2 gap-0.5 rounded-full bg-wd-inset p-0.5"
              role="tablist"
            >
              {(["conversation", "activity"] as const).map((previewMode) => (
                <button
                  aria-selected={mode === previewMode}
                  className={cn(segmentClass, "h-8")}
                  data-waldo-mode={previewMode}
                  key={previewMode}
                  onKeyDown={(event) => {
                    if (event.key !== "ArrowLeft" && event.key !== "ArrowRight") return;
                    event.preventDefault();
                    const nextMode = previewMode === "conversation" ? "activity" : "conversation";
                    setMode(nextMode);
                    event.currentTarget.parentElement
                      ?.querySelector<HTMLButtonElement>(`[data-waldo-mode="${nextMode}"]`)
                      ?.focus();
                  }}
                  onClick={() => setMode(previewMode)}
                  role="tab"
                  tabIndex={mode === previewMode ? 0 : -1}
                  type="button"
                >
                  {previewMode === "conversation" ? (
                    <MessageCircle aria-hidden="true" className="size-3.5" />
                  ) : (
                    <Sparkles aria-hidden="true" className="size-3.5" />
                  )}
                  {t(`waldo.rail.mode.${previewMode}`)}
                </button>
              ))}
            </div>

            <div aria-label={t("waldo.rail.episodeLabel")} className="overflow-x-auto" role="group">
              <div className="flex min-w-max gap-2 pb-0.5">
                {(["fresh", "contextual", "returning"] as const).map((episodeId) => (
                  <button
                    aria-pressed={episode === episodeId}
                    className={cn(
                      "inline-flex h-8 items-center rounded-full hairline border-wd-hairline px-3 wd-caption text-wd-secondary transition-colors duration-fast ease-out hover:bg-wd-hover hover:text-wd-primary motion-reduce:transition-none",
                      episode === episodeId &&
                        "border-transparent bg-wd-inverse text-wd-on-inverse hover:bg-wd-inverse hover:text-wd-on-inverse",
                    )}
                    key={episodeId}
                    onClick={() => selectEpisode(episodeId)}
                    type="button"
                  >
                    {episodeTitles[episodeId]}
                  </button>
                ))}
              </div>
            </div>

            <section className="flex flex-col gap-2 hairline-b border-wd-hairline pb-4" aria-labelledby="waldo-episode-title">
              <div className="flex items-center justify-between gap-3">
                <Badge variant="neutral">{entryStateLabels[episode]}</Badge>
                <span className="wd-caption text-wd-tertiary">{t("waldo.rail.episodeLocal")}</span>
              </div>
              <h3 className="wd-headline text-wd-primary" id="waldo-episode-title">
                {episode === "fresh" ? t("waldo.rail.fresh.title") : episodeTitles[episode]}
              </h3>
              <p className="wd-body-s text-wd-secondary">{t(`waldo.rail.${episode}.description`)}</p>
              {episode !== "fresh" ? (
                <div className="mt-1 flex min-w-0 items-center gap-2 rounded-md bg-wd-inset py-1 pl-3 pr-1">
                  <Paperclip aria-hidden="true" className="size-3.5 shrink-0 text-wd-tertiary" />
                  <span className="min-w-0 flex-1 truncate wd-body-s text-wd-primary">
                    {contextDetached ? t("waldo.rail.noContext") : contextLabel}
                  </span>
                  {!contextDetached ? (
                    <Button
                      aria-label={t("waldo.rail.detachContext")}
                      onClick={() => setContextDetached(true)}
                      size="sm"
                      type="button"
                      variant="ghost"
                    >
                      {t("waldo.rail.detach")}
                    </Button>
                  ) : (
                    <span aria-hidden="true" className="h-control-md" />
                  )}
                </div>
              ) : null}
            </section>

            {mode === "conversation" ? (
              <ConversationPreview
                episode={episode}
                proposalReviewed={proposalReviewed}
                resultExpanded={resultExpanded}
                setProposalReviewed={setProposalReviewed}
                setResultExpanded={setResultExpanded}
                submittedQuestion={submittedQuestion}
              />
            ) : (
              <ActivityPreview contextLabel={contextLabel} previewSteps={previewSteps} />
            )}
          </div>
        )}
      </div>

      {!projectId && previewEnabled && mode === "conversation" ? (
        <footer className="flex shrink-0 flex-col gap-2 hairline-t border-wd-hairline px-3 pb-3 pt-2">
          <div className="min-h-(--size-field) rounded-[calc(var(--size-field)/2)] hairline border-wd-hairline bg-wd-inset px-4 py-3 outline-2 outline-offset-2 outline-transparent transition-[outline-color] duration-fast ease-out focus-within:outline-(--border-focus) motion-reduce:transition-none">
            <textarea
              aria-label={t("waldo.rail.composerLabel")}
              className="block max-h-[6lh] min-h-6 w-full resize-none bg-transparent wd-body text-wd-primary field-sizing-content placeholder:text-wd-tertiary focus-visible:outline-none"
              onChange={(event) => setDraft(event.target.value)}
              placeholder={t("waldo.rail.composerPlaceholder")}
              rows={1}
              value={draft}
            />
          </div>
          <p className="px-4 wd-caption text-wd-tertiary">{t("waldo.rail.composerBoundary")}</p>
        </footer>
      ) : null}
    </section>
  );
}

function ConversationPreview({
  episode,
  proposalReviewed,
  resultExpanded,
  setProposalReviewed,
  setResultExpanded,
  submittedQuestion,
}: {
  episode: WaldoPreviewEpisode;
  proposalReviewed: boolean;
  resultExpanded: boolean;
  setProposalReviewed: (reviewed: boolean) => void;
  setResultExpanded: (expanded: boolean) => void;
  submittedQuestion?: string;
}) {
  const { t } = useTranslation();

  if (episode === "fresh") {
    return (
      <div className="flex flex-col gap-4">
        <p className="wd-body-l text-wd-primary">{t("waldo.rail.fresh.opening")}</p>
        <div aria-label={t("waldo.rail.suggestedPrompts")} className="flex flex-wrap gap-2" role="group">
          {(["orient", "explain", "prepare"] as const).map((prompt) => (
            <button
              className="inline-flex min-h-8 items-center rounded-full hairline border-wd-hairline px-3 py-1.5 text-left wd-caption text-wd-primary transition-colors duration-fast ease-out hover:bg-wd-hover active:bg-wd-pressed motion-reduce:transition-none"
              key={prompt}
              type="button"
            >
              {t(`waldo.rail.prompt.${prompt}`)}
            </button>
          ))}
        </div>
      </div>
    );
  }

  if (episode === "returning") {
    return (
      <section aria-label={t("waldo.rail.resultLabel")} className={cardClass}>
        <div className="flex min-h-6 items-center gap-2">
          <ShieldCheck aria-hidden="true" className="size-4 shrink-0 text-wd-secondary" />
          <span className="wd-caption text-wd-secondary">{t("waldo.rail.resultReady")}</span>
          <Badge className="ml-auto" variant="neutral">{t("waldo.rail.outcomeUnknown")}</Badge>
        </div>
        <h4 className="wd-headline text-wd-primary">{t("waldo.rail.resultTitle")}</h4>
        <p className="wd-body-s text-wd-secondary">{t("waldo.rail.resultSummary")}</p>
        {resultExpanded ? (
          <div className={cn(insetClass, "flex flex-col gap-2 wd-body-s")}>
            <p className="text-wd-primary">{t("waldo.rail.resultDetail")}</p>
            <p className="text-wd-secondary">{t("waldo.rail.resultBoundary")}</p>
          </div>
        ) : null}
        <Button
          aria-expanded={resultExpanded}
          className="-ml-3 self-start"
          onClick={() => setResultExpanded(!resultExpanded)}
          size="sm"
          type="button"
          variant="ghost"
        >
          <ChevronDown aria-hidden="true" className={cn("size-3.5 transition-transform duration-fast ease-out motion-reduce:transition-none", resultExpanded && "rotate-180")} />
          {resultExpanded ? t("waldo.rail.hideResult") : t("waldo.rail.showResult")}
        </Button>
      </section>
    );
  }

  return (
    <div className="flex flex-col gap-5">
      <div className="flex flex-col gap-5" aria-label={t("waldo.rail.conversation")}>
        <div className="flex justify-end">
          <p className="max-w-[80%] rounded-lg rounded-br-xs bg-wd-inset px-4 py-3 wd-body text-wd-primary">
            {submittedQuestion ?? t("waldo.rail.previewRequest")}
          </p>
        </div>
        <div className="flex flex-col gap-2">
          <p className="wd-caption text-wd-secondary">{t("waldo.rail.identity")}</p>
          <p className="wd-body-l text-wd-primary">{t("waldo.rail.previewResponse")}</p>
        </div>
      </div>

      <section aria-label={t("waldo.rail.observationLabel")} className={cardClass}>
        <div className="flex min-h-6 items-center gap-2">
          <Eye aria-hidden="true" className="size-4 shrink-0 text-wd-secondary" />
          <span className="wd-caption text-wd-secondary">{t("waldo.rail.noticed")}</span>
          <Badge className="ml-auto" variant="neutral">{t("waldo.rail.candidate")}</Badge>
        </div>
        <h4 className="wd-headline text-wd-primary">{t("waldo.rail.observationTitle")}</h4>
        <p className="wd-body-s text-wd-secondary">{t("waldo.rail.observationDescription")}</p>
        <p className="wd-note text-wd-tertiary">{t("waldo.rail.observationBoundary")}</p>
      </section>

      <section aria-label={t("waldo.rail.proposalPreviewLabel")} className={cardClass}>
        <div className="flex min-h-6 items-center gap-2">
          <ArrowUpRight aria-hidden="true" className="size-4 shrink-0 text-wd-secondary" />
          <NeedsYouBadge>{t("waldo.project.needsYou")}</NeedsYouBadge>
          <span className="ml-auto wd-caption text-wd-tertiary">{t("waldo.rail.proposal")}</span>
        </div>
        <h4 className="wd-title text-wd-primary">{t("waldo.rail.proposalTitle")}</h4>
        <p className={cn(insetClass, "wd-body-s text-wd-primary")}>{t("waldo.rail.proposalDescription")}</p>
        <p
          aria-label={t("waldo.rail.proposalStatusLabel")}
          className="wd-note text-wd-secondary"
          role="status"
        >
          {proposalReviewed
            ? t("waldo.rail.proposalStatusReviewed")
            : t("waldo.rail.proposalStatusInitial")}
        </p>
        <div className="flex items-center justify-end">
          <Button onClick={() => setProposalReviewed(true)} size="sm" type="button" variant="primary">
            {t("waldo.rail.proposalReview")}
          </Button>
        </div>
      </section>
    </div>
  );
}

type SpecialistStatus = "Ready" | "Paused" | "Revoked";

const specialistDefaults = {
  purpose: "Research the open questions for the pricing workshop",
  scope: "This pricing workshop only",
  sources: "Calendar, decision note, current Work context",
  authority: "Read and draft only; no outward action",
  budget: "20 minutes",
  completion: "Return a source-linked brief with unresolved questions",
  evidence: "Cite each claim and distinguish observation from inference",
};

function ActivityPreview({
  contextLabel,
  previewSteps,
}: {
  contextLabel: string;
  previewSteps: string[];
}) {
  const { t } = useTranslation();
  const [expanded, setExpanded] = useState(false);
  const [builderOpen, setBuilderOpen] = useState(false);
  const [specialist, setSpecialist] = useState<typeof specialistDefaults | null>(null);
  const [specialistStatus, setSpecialistStatus] = useState<SpecialistStatus>("Ready");
  return (
    <div className="flex flex-col gap-4">
      <section aria-label={t("waldo.rail.activityLabel")} className={cardClass}>
        <div className="flex items-start justify-between gap-3">
          <div className="flex min-w-0 flex-col gap-1">
            <p className="wd-caption text-wd-secondary">{t("waldo.rail.activityEyebrow")}</p>
            <h3 className="wd-headline text-wd-primary">{t("waldo.rail.activityTitle")}</h3>
          </div>
          <Badge variant="neutral">
            <span aria-hidden="true" className="size-1.5 rounded-full bg-wd-accent" />
            {t("waldo.rail.running")}
          </Badge>
        </div>
        <p className="wd-note text-wd-tertiary">{t("waldo.rail.activityPreviewBoundary")}</p>
        <dl className="grid gap-3 hairline-t border-wd-hairline pt-3">
          <RunDetail label={t("waldo.rail.activityGoalLabel")} value={t("waldo.rail.activityCompletion")} />
          <RunDetail label={t("waldo.rail.activityCurrentStep")} value={t("waldo.rail.activity.step.prepare")} />
        </dl>
        <div className={cn(insetClass, "grid gap-2 wd-body-s text-wd-primary")}>
          <p><span className="text-wd-secondary">{t("waldo.rail.activityApproval")}</span> <span aria-hidden="true" className="text-wd-tertiary">·</span> {t("waldo.rail.activityApprovalDetail")}</p>
          <p><span className="text-wd-secondary">{t("waldo.rail.activityEvidence")}</span> <span aria-hidden="true" className="text-wd-tertiary">·</span> {t("waldo.rail.activityEvidenceDetail")}</p>
          <p><span className="text-wd-secondary">{t("waldo.rail.activityReturn")}</span> <span aria-hidden="true" className="text-wd-tertiary">·</span> {t("waldo.rail.activityReturnDetail")}</p>
        </div>
        <Button
          aria-expanded={expanded}
          className="-ml-3 self-start"
          onClick={() => setExpanded(!expanded)}
          size="sm"
          type="button"
          variant="ghost"
        >
          <ChevronDown aria-hidden="true" className={cn("size-3.5 transition-transform duration-fast ease-out motion-reduce:transition-none", expanded && "rotate-180")} />
          {expanded ? t("waldo.rail.hideRun") : t("waldo.rail.inspectRun")}
        </Button>
        {expanded ? (
          <div className="flex flex-col gap-4 hairline-t border-wd-hairline pt-4">
            <dl className="grid gap-3">
              <RunDetail label={t("waldo.rail.activityGoalLabel")} value={t("waldo.rail.activityCompletion")} />
              <RunDetail label={t("waldo.rail.activityCompletionLabel")} value={t("waldo.rail.activityCompletionCondition")} />
              <RunDetail label={t("waldo.rail.activityScopeLabel")} value={t("waldo.rail.activityScope")} />
              <RunDetail label={t("waldo.rail.activityDelegationLabel")} value={t("waldo.rail.activityDelegation")} />
              <RunDetail label={t("waldo.rail.activitySourcesLabel")} value={t("waldo.rail.activitySources")} />
            </dl>
            <div className="flex flex-col gap-2">
              <p className="wd-caption text-wd-secondary">{t("waldo.rail.activityPlanLabel")}</p>
              <ol className="flex flex-col gap-2.5">
                {previewSteps.map((step, index) => {
                  const state = previewStepStates[index];
                  return (
                    <li className="flex items-start gap-2.5 wd-body-s" key={step}>
                      <span className="mt-0.5 grid size-4 shrink-0 place-items-center">
                        {state === "evidenced" ? <Check aria-hidden="true" className="size-3.5 text-good" /> : null}
                        {state === "active" ? <span aria-hidden="true" className="size-2 rounded-full bg-wd-accent" /> : null}
                        {state === "blocked" ? <LockKeyhole aria-hidden="true" className="size-3.5 text-wd-tertiary" /> : null}
                      </span>
                      <span className="min-w-0 flex-1 text-wd-primary">{step}</span>
                      <span className="shrink-0 wd-caption text-wd-secondary">
                        {t(`waldo.rail.activity.state.${state}`)}
                      </span>
                    </li>
                  );
                })}
              </ol>
            </div>
            <dl className="grid gap-3 hairline-t border-wd-hairline pt-3">
              <RunDetail label={t("waldo.rail.activityAuthorityLabel")} value={t("waldo.rail.activityAuthority")} />
              <RunDetail label={t("waldo.rail.activityApproval")} value={t("waldo.rail.activityApprovalDetail")} />
              <RunDetail label={t("waldo.rail.activityEvidence")} value={t("waldo.rail.activityEvidenceDetail")} />
              <RunDetail label={t("waldo.rail.activityReturn")} value={t("waldo.rail.activityReturnDetail")} />
              <RunDetail label={t("waldo.rail.activityStatusLabel")} value={`${t("waldo.rail.resultReady")} · ${t("waldo.rail.outcomeUnknown")}`} />
            </dl>
            <p className={cn(insetClass, "wd-caption text-wd-secondary")}>
              {t("waldo.rail.activityDelegationBoundary")}
            </p>
          </div>
        ) : null}
      </section>

      <section className={cardClass}>
        <div className="flex items-start justify-between gap-3">
          <div className="flex min-w-0 flex-col gap-1">
            <p className="wd-caption text-wd-secondary">{t("waldo.rail.specialist.sectionTitle")}</p>
            <h3 className="wd-headline text-wd-primary">{t("waldo.rail.specialist.delegateTitle")}</h3>
          </div>
          <Badge variant="neutral">{t("waldo.rail.specialist.previewOnly")}</Badge>
        </div>
        <p className="wd-body-s text-wd-secondary">{t("waldo.rail.specialist.description")}</p>
        <Button
          aria-expanded={builderOpen}
          className="self-start"
          onClick={() => setBuilderOpen(!builderOpen)}
          size="sm"
          type="button"
          variant="secondary"
        >
          {builderOpen ? t("waldo.rail.specialist.hideBuilder") : t("waldo.rail.specialist.create")}
        </Button>

        {builderOpen ? (
          <SpecialistBuilder
            contextLabel={contextLabel}
            onPreview={(nextSpecialist) => {
              setSpecialist(nextSpecialist);
              setSpecialistStatus("Ready");
            }}
          />
        ) : null}

        {specialist ? (
          <article
            aria-label={t("waldo.rail.specialist.profileTitle")}
            className={cn(insetClass, "flex flex-col gap-3")}
          >
            <div className="flex flex-wrap items-start justify-between gap-3">
              <div className="flex min-w-0 flex-col gap-0.5">
                <p className="wd-caption text-wd-tertiary">{t("waldo.rail.specialist.underWaldo")}</p>
                <h4 className="wd-label-l text-wd-primary">{t("waldo.rail.specialist.profileTitle")}</h4>
              </div>
              <Badge
                variant={specialistStatus === "Ready" ? "success" : specialistStatus === "Revoked" ? "error" : "neutral"}
              >
                {specialistStatus === "Ready" ? <Check aria-hidden="true" /> : null}
                {specialistStatus === "Revoked" ? <X aria-hidden="true" /> : null}
                {t("waldo.rail.specialist.status", { status: specialistStatus })}
              </Badge>
            </div>
            <dl className="grid gap-3">
              <RunDetail label={t("waldo.rail.specialist.purpose")} value={specialist.purpose} />
              <RunDetail label={t("waldo.rail.specialist.scope")} value={specialist.scope} />
              <RunDetail label={t("waldo.rail.specialist.sources")} value={specialist.sources} />
              <RunDetail label={t("waldo.rail.specialist.authority")} value={specialist.authority} />
              <RunDetail label={t("waldo.rail.specialist.budget")} value={specialist.budget} />
              <RunDetail label={t("waldo.rail.specialist.completion")} value={specialist.completion} />
              <RunDetail label={t("waldo.rail.specialist.evidence")} value={specialist.evidence} />
              <RunDetail label={t("waldo.rail.specialist.returnDestination")} value={t("waldo.rail.specialist.returnsTo", { destination: contextLabel })} />
            </dl>
            <p className="wd-note text-wd-tertiary">{t("waldo.rail.specialist.profileBoundary")}</p>
            <div className="flex flex-wrap justify-end gap-2">
              <Button
                disabled={specialistStatus === "Revoked"}
                onClick={() => setSpecialistStatus(specialistStatus === "Paused" ? "Ready" : "Paused")}
                size="sm"
                type="button"
                variant="outline"
              >
                {specialistStatus === "Paused" ? t("waldo.rail.specialist.resume") : t("waldo.rail.specialist.pause")}
              </Button>
              <Button
                disabled={specialistStatus === "Revoked"}
                onClick={() => setSpecialistStatus("Revoked")}
                size="sm"
                type="button"
                variant="critical"
              >
                {t("waldo.rail.specialist.revoke")}
              </Button>
            </div>
          </article>
        ) : null}
      </section>
    </div>
  );
}

function SpecialistBuilder({
  contextLabel,
  onPreview,
}: {
  contextLabel: string;
  onPreview: (specialist: typeof specialistDefaults) => void;
}) {
  const { t } = useTranslation();
  const [draft, setDraft] = useState(specialistDefaults);
  const fields = [
    ["purpose", t("waldo.rail.specialist.purpose")],
    ["scope", t("waldo.rail.specialist.scope")],
    ["sources", t("waldo.rail.specialist.sources")],
    ["authority", t("waldo.rail.specialist.authority")],
    ["budget", t("waldo.rail.specialist.budget")],
    ["completion", t("waldo.rail.specialist.completion")],
    ["evidence", t("waldo.rail.specialist.evidence")],
  ] as const;
  const inputClass =
    "h-10 min-w-0 rounded-sm hairline border-wd-hairline bg-wd-inset px-3 wd-body-s";

  return (
    <section aria-label={t("waldo.rail.specialist.builderAria")} className="flex flex-col gap-3 hairline-t border-wd-hairline pt-4">
      <p className="wd-caption text-wd-tertiary">{t("waldo.rail.specialist.previewOnly")}</p>
      <form
        className="grid gap-3"
        onSubmit={(event) => {
          event.preventDefault();
          onPreview(draft);
        }}
      >
        {fields.map(([key, label]) => (
          <label className="grid gap-1.5 wd-caption text-wd-secondary" key={key}>
            {label}
            <input
              aria-label={label}
              className={cn(inputClass, "text-wd-primary")}
              onChange={(event) => setDraft({ ...draft, [key]: event.target.value })}
              value={draft[key]}
            />
          </label>
        ))}
        <label className="grid gap-1.5 wd-caption text-wd-secondary">
          {t("waldo.rail.specialist.returnDestination")}
          <input
            aria-label={t("waldo.rail.specialist.returnDestination")}
            className={cn(inputClass, "text-wd-secondary")}
            readOnly
            value={contextLabel}
          />
        </label>
        <Button className="mt-1 w-fit" size="sm" type="submit" variant="primary">
          {t("waldo.rail.specialist.previewAction")}
        </Button>
        <p className="wd-note text-wd-tertiary">{t("waldo.rail.specialist.builderBoundary")}</p>
      </form>
    </section>
  );
}

function RunDetail({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex flex-col gap-0.5">
      <dt className="wd-caption text-wd-secondary">{label}</dt>
      <dd className="wd-body-s text-wd-primary">{value}</dd>
    </div>
  );
}
