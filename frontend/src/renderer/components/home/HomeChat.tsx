import {
  Activity,
  ArrowLeft,
  ArrowUp,
  Check,
  CircleDashed,
  CirclePause,
  FileText,
  Link2,
  MessageCircle,
  Mic,
  Paperclip,
  Plus,
  RotateCcw,
  Search,
  ShieldCheck,
  Square,
} from "lucide-react";
import { useEffect, useRef, useState, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import waldoMark from "../../../../assets/waldo-mark.svg";
import { cn } from "../../lib/utils";
import { Badge } from "../ui/badge";
import { Button } from "../ui/button";
import { Dialog, DialogContent, DialogTitle } from "../ui/dialog";
import {
  useWaldoRail,
  type WaldoPreviewEpisode,
  type WaldoPreviewMode,
} from "../waldo/WaldoRailContext";
import { HomeCard, HomeCardActions, HomeEyebrow, HomeInset, NeedsYouBadge } from "./HomeCards";

type RunState = "waiting" | "approved" | "paused" | "stopped";
type ApprovalAction = "accept" | "edit" | "reject" | "respond";

// Chat on the Waldo Message, ThreadRow and Composer contracts: the person
// speaks in a right-aligned inset bubble, Waldo answers on the page in body-l
// at the measure under an attribution line, threads are hairline-separated
// rows, and the composer is a pill field with an ink round send. The three
// panes are divided by hairlines with no coloured panes or shadows.
const eyebrowClass = "wd-caption text-wd-tertiary";
const hairlineTop = "border-t-[length:var(--stroke-hairline)] border-wd-hairline";
const hairlineBottom = "border-b-[length:var(--stroke-hairline)] border-wd-hairline";

export function HomeChat({
  contextLabel,
  previewEnabled,
}: {
  contextLabel: string;
  previewEnabled: boolean;
}) {
  const { t } = useTranslation();
  const shared = useWaldoRail().conversation;
  const [localMode, setLocalMode] = useState<WaldoPreviewMode>("conversation");
  const [localEpisode, setLocalEpisode] = useState<WaldoPreviewEpisode>("contextual");
  const [localDetached, setLocalDetached] = useState(false);
  const [localDraft, setLocalDraft] = useState("");
  const [localSubmittedQuestions, setLocalSubmittedQuestions] = useState<
    Partial<Record<WaldoPreviewEpisode, string>>
  >({});
  const [correctionApplied, setCorrectionApplied] = useState(false);
  const [runState, setRunState] = useState<RunState>("waiting");
  const [approvalResponse, setApprovalResponse] = useState<ApprovalAction | null>(null);
  const [detailsLayer, setDetailsLayer] = useState<"context" | "run" | null>(null);
  const detailsTriggerRef = useRef<HTMLButtonElement | null>(null);
  const transcriptRef = useRef<HTMLDivElement>(null);
  const workspaceRef = useRef<HTMLElement>(null);

  const mode = shared?.mode ?? localMode;
  const episode = shared?.episode ?? localEpisode;
  const contextDetached = shared?.contextDetached ?? localDetached;
  const draft = shared?.draft ?? localDraft;
  const setMode = shared?.setMode ?? setLocalMode;
  const setContextDetached = shared?.setContextDetached ?? setLocalDetached;
  const setDraft = shared?.setDraft ?? setLocalDraft;
  const submittedQuestion =
    shared?.submittedQuestions[episode] ?? localSubmittedQuestions[episode] ?? null;
  const setSubmittedQuestion = (question: string) => {
    if (shared) shared.setSubmittedQuestion(episode, question);
    else {
      setLocalSubmittedQuestions((current) => ({ ...current, [episode]: question }));
    }
  };
  const selectEpisode = (next: WaldoPreviewEpisode) => {
    if (shared) shared.selectEpisode(next);
    else {
      setLocalEpisode(next);
      setLocalDetached(false);
    }
    setCorrectionApplied(false);
  };

  useEffect(() => {
    if (!submittedQuestion || !transcriptRef.current) return;
    const transcript = transcriptRef.current;
    const frame = window.requestAnimationFrame(() => {
      transcript.scrollTop = transcript.scrollHeight;
    });
    return () => window.cancelAnimationFrame(frame);
  }, [submittedQuestion]);

  if (!previewEnabled) {
    return (
      <section aria-label={t("waldo.rail.aria")} className="home-chat-workspace rounded-lg border-[length:var(--stroke-hairline)] border-wd-hairline bg-wd-surface">
        <WaldoHeader mode={mode} previewEnabled={false} setMode={setMode} />
        <div className="flex min-h-0 flex-1 flex-col justify-between gap-10 overflow-y-auto p-8">
          <div className="flex max-w-(--layout-measure) flex-col gap-2">
            <div className="mb-2 grid size-10 place-items-center rounded-sm bg-wd-inset text-wd-secondary">
              <MessageCircle aria-hidden="true" className="size-4" />
            </div>
            <h2 className="wd-title text-wd-primary">
              {t("waldo.rail.unconfigured.title")}
            </h2>
            <p className="wd-body-s text-wd-secondary">
              {t("waldo.rail.unconfigured.description")}
            </p>
          </div>
          <div className={cn("flex max-w-(--layout-measure) flex-col gap-1 pt-4", hairlineTop)}>
            <p className="wd-label text-wd-primary">{t("waldo.rail.unconfigured.localTitle")}</p>
            <p className="wd-caption text-wd-secondary">
              {t("waldo.rail.unconfigured.localDescription")}
            </p>
          </div>
        </div>
      </section>
    );
  }

  return (
    <section aria-label={t("waldo.rail.aria")} className="home-chat-workspace rounded-lg border-[length:var(--stroke-hairline)] border-wd-hairline bg-wd-surface" ref={workspaceRef}>
      <WaldoHeader mode={mode} previewEnabled setMode={setMode} />
      {mode === "conversation" ? (
        <div className="home-chat-grid min-h-0 flex-1">
          <ConversationRail episode={episode} onSelect={selectEpisode} />
          <main aria-label={t("home.chat.conversationAria")} className="home-chat-primary" role="region">
            <header className={cn("shrink-0 px-6 py-4", hairlineBottom)}>
              <div className="flex flex-wrap items-start justify-between gap-3">
                <div className="flex min-w-0 flex-col gap-1">
                  <p className={eyebrowClass}>
                    {t("home.chat.relationship")}
                  </p>
                  <h2 className="wd-title text-wd-primary">
                    {episode === "fresh" ? t("waldo.rail.episode.fresh") : episode === "returning" ? t("waldo.rail.episode.returning") : t("waldo.rail.episode.contextual")}
                  </h2>
                  <p className="wd-caption text-wd-secondary">{t("home.chat.purpose")}</p>
                </div>
                <div className="flex items-center gap-2">
                  <Badge>{t("home.chat.localEpisode")}</Badge>
                  <Button
                    className="home-chat-compact-trigger"
                    onClick={(event) => {
                      detailsTriggerRef.current = event.currentTarget;
                      setDetailsLayer("context");
                    }}
                    size="sm"
                    type="button"
                    variant="secondary"
                  >
                    {t("home.chat.openContext")}
                  </Button>
                </div>
              </div>
            </header>

            <div className="home-chat-transcript" aria-live="polite" ref={transcriptRef}>
              {episode === "fresh" ? (
                <div className="mx-auto flex min-h-full max-w-(--layout-measure) flex-col items-center justify-center gap-2 px-6 py-12 text-center">
                  <span className="mb-2 grid size-11 place-items-center rounded-sm bg-wd-inset">
                    <img alt="" aria-hidden="true" className="size-5" src={waldoMark} />
                  </span>
                  <h3 className="wd-headline text-wd-primary">{t("waldo.rail.fresh.title")}</h3>
                  <p className="wd-body-s text-wd-secondary">{t("waldo.rail.fresh.opening")}</p>
                </div>
              ) : (
                <div className="mx-auto flex w-full max-w-(--layout-column) flex-col gap-6 px-6 py-7">
                  <WaldoTurn>
                    <p className="wd-body-l text-wd-primary">{t("home.chat.opening")}</p>
                  </WaldoTurn>

                  {submittedQuestion ? (
                    <>
                      <article className="flex flex-col items-end">
                        <p className="wd-body max-w-[80%] rounded-lg rounded-br-xs bg-wd-inset px-4 py-3 text-wd-primary">
                          {submittedQuestion}
                        </p>
                      </article>
                      <WaldoTurn>
                        <p className="wd-body-l text-wd-primary">{t("home.chat.previewAnswer")}</p>
                        <div className="flex flex-wrap gap-2">
                          <a className="home-chat-source" href="#/home/history?record=pricing-decision-note">
                            <FileText aria-hidden="true" className="size-3.5" />
                            {t("home.chat.sourceDecision")}
                          </a>
                          <a className="home-chat-source" href="#/home/history?record=pricing-workshop-calendar">
                            <Link2 aria-hidden="true" className="size-3.5" />
                            {t("home.chat.sourceCalendar")}
                          </a>
                        </div>
                        <HomeCard className="mt-1">
                          <HomeEyebrow>
                            <NeedsYouBadge />
                          </HomeEyebrow>
                          <p className="wd-headline text-wd-primary">{t("home.chat.boundedStep")}</p>
                          <p className="wd-body-s text-wd-secondary">{t("home.chat.boundedStepDetail")}</p>
                          <HomeCardActions
                            trail={
                              <Button onClick={() => setMode("activity")} size="sm" type="button" variant="primary">
                                {t("home.chat.reviewInActivity")}
                              </Button>
                            }
                          />
                        </HomeCard>
                      </WaldoTurn>
                    </>
                  ) : null}
                </div>
              )}
            </div>

            <footer className={cn("shrink-0 bg-wd-surface px-4 py-3", hairlineTop)}>
              <div className="mx-auto flex max-w-(--layout-column) flex-col gap-2">
                {!contextDetached && episode !== "fresh" ? (
                  <div className="wd-caption flex min-w-0 items-center gap-1.5 pl-12 text-wd-secondary">
                    <Paperclip aria-hidden="true" className="size-3.5 shrink-0" />
                    <span className="truncate">{contextLabel}</span>
                  </div>
                ) : null}
                <div className="flex items-end gap-1">
                  <QuietIconButton label={t("home.chat.attach")}><Plus aria-hidden="true" className="size-4" /></QuietIconButton>
                  <div className="min-w-0 flex-1 rounded-[24px] border-[length:var(--stroke-hairline)] border-wd-hairline bg-wd-inset px-4 py-[11px] focus-within:outline-2 focus-within:outline-offset-2 focus-within:outline-(--border-focus)">
                    <textarea
                      aria-label={t("waldo.rail.composerLabel")}
                      className="wd-body block max-h-36 min-h-6 w-full resize-none bg-transparent text-wd-primary outline-none [field-sizing:content] placeholder:text-wd-tertiary"
                      onChange={(event) => setDraft(event.target.value)}
                      placeholder={t("waldo.rail.composerPlaceholder")}
                      rows={1}
                      value={draft}
                    />
                  </div>
                  <QuietIconButton label={t("home.chat.voice")}><Mic aria-hidden="true" className="size-4" /></QuietIconButton>
                  <Button
                    aria-label={t("home.chat.sendPreview")}
                    className="mb-0.5 size-11"
                    disabled={!draft.trim()}
                    onClick={() => {
                      setSubmittedQuestion(draft.trim());
                      setDraft("");
                    }}
                    size="icon"
                    title={t("home.chat.sendPreview")}
                    type="button"
                    variant="primary"
                  >
                    <ArrowUp aria-hidden="true" className="size-4" />
                  </Button>
                </div>
                <p className="wd-caption text-center text-wd-tertiary">{t("home.chat.previewBoundary")}</p>
              </div>
            </footer>
          </main>

          <aside aria-label={t("home.chat.contextAria")} className="home-chat-inspector">
            <ContextInspector
              contextDetached={contextDetached}
              contextLabel={contextLabel}
              correctionApplied={correctionApplied}
              onCorrect={() => setCorrectionApplied(true)}
              onDetach={() => setContextDetached(true)}
              onReattach={() => setContextDetached(false)}
            />
          </aside>
        </div>
      ) : (
        <ActivityWorkspace
          approvalResponse={approvalResponse}
          contextLabel={contextLabel}
          onApproval={(response) => {
            setApprovalResponse(response);
            setRunState(
              response === "accept"
                ? "approved"
                : response === "reject"
                  ? "stopped"
                  : "waiting",
            );
          }}
          onOpenInspector={(origin) => {
            detailsTriggerRef.current = origin;
            setDetailsLayer("run");
          }}
          onReturn={() => {
            setDetailsLayer(null);
            setMode("conversation");
          }}
          onRunState={setRunState}
          runState={runState}
        />
      )}
      <Dialog open={detailsLayer !== null} onOpenChange={(open) => !open && setDetailsLayer(null)}>
        {detailsLayer ? (
          <DialogContent
            aria-describedby={undefined}
            aria-label={detailsLayer === "context" ? t("home.chat.contextLayerAria") : t("home.chat.runLayerAria")}
            className="home-chat-compact-layer"
            onCloseAutoFocus={(event) => {
              event.preventDefault();
              detailsTriggerRef.current?.focus({ preventScroll: true });
            }}
            overlay={false}
            portalContainer={workspaceRef.current}
            showCloseButton={false}
          >
            <DialogTitle className="sr-only">
              {detailsLayer === "context"
                ? t("home.chat.contextLayerAria")
                : t("home.chat.runLayerAria")}
            </DialogTitle>
            <header className={cn("flex shrink-0 items-center gap-3 px-4 py-2", hairlineBottom)}>
              <Button
                className="-ml-1"
                onClick={() => setDetailsLayer(null)}
                size="sm"
                type="button"
                variant="ghost"
              >
                <ArrowLeft aria-hidden="true" className="size-3.5" />
                {t("home.chat.backToConversation")}
              </Button>
            </header>
            <div className="min-h-0 flex-1 overflow-y-auto">
              {detailsLayer === "context" ? (
                <ContextInspector
                  contextDetached={contextDetached}
                  contextLabel={contextLabel}
                  correctionApplied={correctionApplied}
                  onCorrect={() => setCorrectionApplied(true)}
                  onDetach={() => setContextDetached(true)}
                  onReattach={() => setContextDetached(false)}
                />
              ) : (
                <RunInspectorContent
                  contextLabel={contextLabel}
                  onReturn={() => {
                    setDetailsLayer(null);
                    setMode("conversation");
                  }}
                  onRunState={setRunState}
                />
              )}
            </div>
          </DialogContent>
        ) : null}
      </Dialog>
    </section>
  );
}

/** Waldo answers on the page: the attribution line, then the prose. */
function WaldoTurn({ children }: { children: ReactNode }) {
  const { t } = useTranslation();
  return (
    <article className="flex flex-col gap-2">
      <p className="flex items-center gap-2">
        <img alt="" aria-hidden="true" className="size-4" src={waldoMark} />
        <span className="wd-label text-wd-primary">{t("waldo.rail.identity")}</span>
      </p>
      <div className="flex max-w-(--layout-measure) flex-col gap-3">{children}</div>
    </article>
  );
}

function WaldoHeader({
  mode,
  previewEnabled,
  setMode,
}: {
  mode: WaldoPreviewMode;
  previewEnabled: boolean;
  setMode: (mode: WaldoPreviewMode) => void;
}) {
  const { t } = useTranslation();
  return (
    <header className={cn("flex shrink-0 flex-wrap items-center justify-between gap-4 px-5 py-3", hairlineBottom)}>
      <div className="flex min-w-0 items-center gap-3">
        <span className="grid size-9 shrink-0 place-items-center rounded-sm bg-wd-inset">
          <img alt="" aria-hidden="true" className="size-4.5" src={waldoMark} />
        </span>
        <div className="flex min-w-0 flex-col gap-0.5">
          <div className="flex items-center gap-2">
            <h1 className="wd-label text-wd-primary">{t("waldo.rail.identity")}</h1>
            <Badge>
              {previewEnabled ? t("waldo.rail.previewBadge") : t("waldo.rail.localBadge")}
            </Badge>
          </div>
          <p className="wd-caption truncate text-wd-tertiary">{t("home.chat.relationship")}</p>
        </div>
      </div>
      {previewEnabled ? (
        <div aria-label={t("waldo.rail.modeLabel")} className="grid grid-cols-2 gap-1 rounded-full bg-wd-inset p-1" role="tablist">
          {(["conversation", "activity"] as const).map((item) => (
            <button
              aria-selected={mode === item}
              className={cn(
                "wd-label inline-flex h-8 items-center justify-center gap-1.5 rounded-full px-3 text-wd-secondary transition-colors duration-fast hover:text-wd-primary",
                mode === item && "bg-wd-inverse text-wd-on-inverse hover:text-wd-on-inverse",
              )}
              key={item}
              data-home-chat-mode={item}
              onKeyDown={(event) => {
                if (event.key !== "ArrowLeft" && event.key !== "ArrowRight") return;
                event.preventDefault();
                const nextMode = item === "conversation" ? "activity" : "conversation";
                setMode(nextMode);
                event.currentTarget.parentElement
                  ?.querySelector<HTMLButtonElement>(`[data-home-chat-mode="${nextMode}"]`)
                  ?.focus();
              }}
              onClick={() => setMode(item)}
              role="tab"
              tabIndex={mode === item ? 0 : -1}
              type="button"
            >
              {item === "conversation" ? <MessageCircle aria-hidden="true" className="size-3.5" /> : <Activity aria-hidden="true" className="size-3.5" />}
              {t(`waldo.rail.mode.${item}`)}
            </button>
          ))}
        </div>
      ) : null}
    </header>
  );
}

function ConversationRail({ episode, onSelect }: { episode: WaldoPreviewEpisode; onSelect: (episode: WaldoPreviewEpisode) => void }) {
  const { t } = useTranslation();
  const episodes = [
    ["fresh", t("waldo.rail.episode.fresh"), t("home.chat.newEpisodeMeta")],
    ["contextual", t("waldo.rail.episode.contextual"), t("home.chat.pricingMeta")],
    ["returning", t("waldo.rail.episode.returning"), t("home.chat.returningMeta")],
  ] as const;
  return (
    <nav aria-label={t("home.chat.conversationsAria")} className="home-chat-side-rail">
      <div className="flex gap-1 p-3">
        <Button className="flex-1" onClick={() => onSelect("fresh")} size="sm" type="button" variant="secondary">
          <Plus aria-hidden="true" className="size-3.5" />
          {t("home.chat.newConversation")}
        </Button>
        <QuietIconButton label={t("home.chat.search")}><Search aria-hidden="true" className="size-4" /></QuietIconButton>
      </div>
      <p className={cn(eyebrowClass, "px-4 pb-1 pt-2")}>{t("home.chat.recent")}</p>
      <div className="flex flex-col px-1 pb-2">
        {episodes.map(([id, title, meta]) => (
          <button
            aria-current={episode === id ? "page" : undefined}
            className={cn(
              "relative flex min-h-11 w-full items-start gap-3 rounded-md py-2.5 pl-3 pr-2 text-left transition-colors duration-fast hover:bg-wd-hover active:bg-wd-pressed",
              "[&+&]:before:absolute [&+&]:before:top-0 [&+&]:before:right-0 [&+&]:before:left-[calc(0.75rem+8px+0.75rem)] [&+&]:before:border-t-[length:var(--stroke-hairline)] [&+&]:before:border-wd-hairline [&+&]:before:content-['']",
              episode === id && "bg-wd-selected",
            )}
            key={id}
            onClick={() => onSelect(id)}
            type="button"
          >
            <span aria-hidden="true" className="mt-1.5 size-2 shrink-0 rounded-full" />
            <span className="flex min-w-0 flex-1 flex-col gap-0.5">
              <span className="wd-label block truncate text-wd-primary">{title}</span>
              <span className="wd-caption block truncate text-wd-secondary">{meta}</span>
            </span>
          </button>
        ))}
      </div>
    </nav>
  );
}

function ContextInspector({ contextDetached, contextLabel, correctionApplied, onCorrect, onDetach, onReattach }: { contextDetached: boolean; contextLabel: string; correctionApplied: boolean; onCorrect: () => void; onDetach: () => void; onReattach: () => void }) {
  const { t } = useTranslation();
  return (
    <div className="flex flex-col gap-5 p-4">
      <div className="flex flex-col gap-1">
        <p className={eyebrowClass}>{t("home.chat.context")}</p>
        <h3 className="wd-label text-wd-primary">{contextDetached ? t("waldo.rail.noContext") : contextLabel}</h3>
      </div>
      {contextDetached ? (
        <Button className="w-full" onClick={onReattach} size="sm" type="button" variant="secondary">
          {t("home.chat.reattach")}
        </Button>
      ) : (
        <>
          <dl className="flex flex-col gap-3">
            <InspectorRow label={t("home.chat.coverage")} value={t("home.chat.coverageValue")} />
            <InspectorRow label={t("home.chat.freshness")} value={t("home.chat.freshnessValue")} />
            <InspectorRow label={t("home.chat.gaps")} value={t("home.chat.gapsValue")} />
          </dl>
          <HomeInset>
            <p className="wd-label text-wd-primary">{t("home.chat.attachedSources")}</p>
            <ul className="wd-body-s flex flex-col gap-1 text-wd-secondary">
              <li>{t("home.chat.sourceDecision")}</li>
              <li>{t("home.chat.sourceCalendar")}</li>
            </ul>
          </HomeInset>
          <div className="flex flex-col gap-1">
            <Button onClick={onCorrect} size="sm" type="button" variant="secondary">
              {t("home.chat.correct")}
            </Button>
            <Button aria-label={t("waldo.rail.detachContext")} onClick={onDetach} size="sm" type="button" variant="ghost">
              {t("waldo.rail.detachContext")}
            </Button>
          </div>
        </>
      )}
      {correctionApplied ? (
        <p role="status" className="wd-body-s flex items-start gap-1.5 rounded-md bg-wd-inset p-3 text-wd-primary">
          <Check aria-hidden="true" className="mt-0.5 size-3.5 shrink-0 text-wd-secondary" />
          {t("home.chat.correctionApplied")}
        </p>
      ) : null}
      <p className={cn("wd-caption pt-4 text-wd-tertiary", hairlineTop)}>{t("home.chat.contextBoundary")}</p>
    </div>
  );
}

function ActivityWorkspace({
  approvalResponse,
  contextLabel,
  onApproval,
  onOpenInspector,
  onReturn,
  onRunState,
  runState,
}: {
  approvalResponse: ApprovalAction | null;
  contextLabel: string;
  onApproval: (response: ApprovalAction) => void;
  onOpenInspector: (origin: HTMLButtonElement) => void;
  onReturn: () => void;
  onRunState: (state: RunState) => void;
  runState: RunState;
}) {
  const { t } = useTranslation();
  return (
    <div className="home-chat-grid min-h-0 flex-1">
      <nav aria-label={t("home.chat.specialistsAria")} className="home-chat-side-rail">
        <div className={cn("p-3", hairlineBottom)}>
          <Button
            aria-describedby="home-chat-specialist-create-boundary"
            className="w-full"
            disabled
            size="sm"
            type="button"
            variant="secondary"
          >
            <Plus aria-hidden="true" className="size-3.5" />
            {t("waldo.rail.specialist.create")}
          </Button>
          <p className="sr-only" id="home-chat-specialist-create-boundary">{t("home.chat.runBoundary")}</p>
        </div>
        <p className={cn(eyebrowClass, "px-4 pb-2 pt-4")}>{t("home.chat.coordinator")}</p>
        <div className="mx-2 flex flex-col gap-1 rounded-md bg-wd-inset px-3 py-2.5">
          <div className="flex items-center gap-2">
            <img alt="" aria-hidden="true" className="size-4" src={waldoMark} />
            <span className="wd-label text-wd-primary">{t("waldo.rail.identity")}</span>
          </div>
          <p className="wd-caption text-wd-secondary">{t("home.chat.coordinatorMeta")}</p>
        </div>
        <p className={cn(eyebrowClass, "px-4 pb-2 pt-5")}>{t("home.chat.specialists")}</p>
        <button aria-current="page" className="mx-2 flex min-h-11 w-[calc(100%-1rem)] flex-col gap-1 rounded-md bg-wd-selected px-3 py-2.5 text-left" type="button">
          <span className="flex w-full items-center justify-between gap-2">
            <span className="wd-label text-wd-primary">{t("home.chat.researchSpecialist")}</span>
            <span aria-hidden="true" className="size-2 rounded-full bg-spot" />
          </span>
          <span className="wd-caption block text-wd-secondary">{t("home.chat.specialistMeta")}</span>
        </button>
      </nav>

      <main aria-label={t("home.chat.runAria")} className="home-chat-primary" role="region">
        <header className={cn("shrink-0 px-6 py-4", hairlineBottom)}>
          <div className="flex flex-wrap items-start justify-between gap-3">
            <div className="flex min-w-0 flex-col gap-1">
              <p className={eyebrowClass}>{t("home.chat.coordinatorStatement")}</p>
              <h2 className="wd-title text-wd-primary">{t("home.chat.runTitle")}</h2>
              <p className="wd-caption text-wd-secondary">{t("home.chat.runPurpose")}</p>
            </div>
            <div className="flex items-center gap-2">
              <Badge>{t(`home.chat.runState.${runState}`)}</Badge>
              <Button className="home-chat-compact-trigger" onClick={(event) => onOpenInspector(event.currentTarget)} size="sm" type="button" variant="secondary">
                {t("home.chat.openRunInspector")}
              </Button>
            </div>
          </div>
        </header>
        <div className="home-chat-transcript">
          <div className="mx-auto flex w-full max-w-(--layout-column) flex-col gap-6 px-6 py-7">
            <div className="flex flex-col [&>div+div]:border-t-[length:var(--stroke-hairline)] [&>div+div]:border-wd-hairline">
              <TimelineItem complete title={t("home.chat.timelineContext")} detail={t("home.chat.timelineContextDetail")} />
              <TimelineItem complete title={t("home.chat.timelineSources")} detail={t("home.chat.timelineSourcesDetail")} />
              <TimelineItem title={t("home.chat.timelineDraft")} detail={t("home.chat.timelineDraftDetail")} />
            </div>
            <HomeCard>
              <HomeEyebrow>
                {runState === "waiting" ? <NeedsYouBadge /> : <Badge>{t(`home.chat.runState.${runState}`)}</Badge>}
              </HomeEyebrow>
              <p className="wd-headline text-wd-primary">{t("home.chat.approvalTitle")}</p>
              <p className="wd-body text-wd-primary">{t("home.chat.approvalDetail")}</p>
              <p className="wd-caption text-wd-secondary">{t("home.chat.approvalConsequence")}</p>
              <HomeCardActions
                hint={approvalResponse ? <span role="status">{t("home.chat.approvalPreview", { action: t(`home.chat.approval.${approvalResponse}`) })}</span> : undefined}
                lead={
                  <>
                    <Button data-quiet onClick={() => onApproval("reject")} size="sm" type="button" variant="ghost">
                      {t("home.chat.approval.reject")}
                    </Button>
                    <Button data-quiet onClick={() => onApproval("respond")} size="sm" type="button" variant="ghost">
                      {t("home.chat.approval.respond")}
                    </Button>
                  </>
                }
                trail={
                  <>
                    <Button onClick={() => onApproval("edit")} size="sm" type="button" variant="secondary">
                      {t("home.chat.approval.edit")}
                    </Button>
                    <Button onClick={() => onApproval("accept")} size="sm" type="button" variant="primary">
                      {t("home.chat.approval.accept")}
                    </Button>
                  </>
                }
              />
            </HomeCard>
          </div>
        </div>
        <footer className={cn("flex shrink-0 flex-wrap items-center justify-between gap-3 px-5 py-3", hairlineTop)}>
          <p className="wd-caption text-wd-tertiary">{t("home.chat.runBoundary")}</p>
          <div className="flex flex-wrap gap-2">
            <Button
              disabled={runState !== "approved" && runState !== "paused"}
              onClick={() => onRunState(runState === "paused" ? "approved" : "paused")}
              size="sm"
              type="button"
              variant="secondary"
            >
              <CirclePause aria-hidden="true" className="size-3.5" />
              {runState === "paused" ? t("waldo.rail.specialist.resume") : t("waldo.rail.specialist.pause")}
            </Button>
            <Button disabled={runState === "stopped"} onClick={() => onRunState("stopped")} size="sm" type="button" variant="critical">
              <Square aria-hidden="true" className="size-3.5" />
              {t("home.chat.stop")}
            </Button>
          </div>
        </footer>
      </main>

      <aside aria-label={t("home.chat.runInspectorAria")} className="home-chat-inspector">
        <RunInspectorContent contextLabel={contextLabel} onReturn={onReturn} onRunState={onRunState} />
      </aside>
    </div>
  );
}

function RunInspectorContent({ contextLabel, onReturn, onRunState }: { contextLabel: string; onReturn: () => void; onRunState: (state: RunState) => void }) {
  const { t } = useTranslation();
  return (
    <div className="flex flex-col gap-5 p-4">
      <div className="flex flex-col gap-1">
        <p className={eyebrowClass}>{t("home.chat.authorityEvidence")}</p>
        <h3 className="wd-label text-wd-primary">{t("home.chat.researchSpecialist")}</h3>
      </div>
      <dl className="flex flex-col gap-3">
        <InspectorRow label={t("waldo.rail.specialist.scope")} value={t("home.chat.scopeValue")} />
        <InspectorRow label={t("waldo.rail.specialist.sources")} value={t("home.chat.sourcesValue")} />
        <InspectorRow label={t("waldo.rail.specialist.authority")} value={t("home.chat.authorityValue")} />
        <InspectorRow label={t("waldo.rail.specialist.budget")} value={t("home.chat.budgetValue")} />
        <InspectorRow label={t("waldo.rail.specialist.completion")} value={t("home.chat.completionValue")} />
        <InspectorRow label={t("waldo.rail.specialist.returnDestination")} value={contextLabel} />
      </dl>
      <HomeInset>
        <p className="wd-label flex items-center gap-2 text-wd-primary"><ShieldCheck aria-hidden="true" className="size-3.5" />{t("home.chat.evidenceTitle")}</p>
        <p className="wd-caption text-wd-secondary">{t("home.chat.evidenceDetail")}</p>
      </HomeInset>
      <div className="flex flex-col gap-1">
        <Button onClick={() => onRunState("waiting")} size="sm" type="button" variant="secondary"><RotateCcw aria-hidden="true" className="size-3.5" />{t("home.chat.retry")}</Button>
        <Button onClick={onReturn} size="sm" type="button" variant="ghost"><ArrowLeft aria-hidden="true" className="size-3.5" />{t("home.chat.return")}</Button>
      </div>
      <p className={cn("wd-caption pt-4 text-wd-tertiary", hairlineTop)}>{t("home.chat.runInspectorBoundary")}</p>
    </div>
  );
}

function TimelineItem({ complete = false, detail, title }: { complete?: boolean; detail: string; title: string }) {
  return (
    <div className="flex gap-3 py-3 first:pt-0 last:pb-0">
      <span className="mt-0.5 grid size-5 shrink-0 place-items-center rounded-full bg-wd-inset text-wd-secondary">
        {complete ? <Check aria-hidden="true" className="size-3" /> : <CircleDashed aria-hidden="true" className="size-3" />}
      </span>
      <div className="flex min-w-0 flex-col gap-0.5">
        <p className="wd-label text-wd-primary">{title}</p>
        <p className="wd-body-s text-wd-secondary">{detail}</p>
      </div>
    </div>
  );
}

function InspectorRow({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex flex-col gap-0.5">
      <dt className="wd-caption text-wd-secondary">{label}</dt>
      <dd className="wd-body-s text-wd-primary">{value}</dd>
    </div>
  );
}

function QuietIconButton({ children, label }: { children: ReactNode; label: string }) {
  return (
    <Button aria-label={label} className="size-11 shrink-0 text-wd-secondary hover:text-wd-primary" size="icon" type="button" variant="ghost">
      {children}
    </Button>
  );
}
