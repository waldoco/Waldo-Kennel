import { ChevronRight, Clock } from "lucide-react";
import { useLayoutEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import type {
  HomeFixtureState,
  HomeOpenLoopFixture,
  HomeOpenLoopState,
} from "../../lib/home-fixture";
import { cn } from "../../lib/utils";
import { Button } from "../ui/button";
import { HomeCardActions, HomeStateBadge, NeedsYouBadge } from "./HomeCards";

type OpenLoopPreviewStatus = "initial" | "correct" | "context" | "work";

// Open loops on the Waldo OpenLoopRow contract: each loop is one button row
// (title, then the owner and the next check in caption, a state word, a
// chevron), hairlines between rows, at least 44px tall. The desk keeps its
// master/detail panes, divided by a hairline with no coloured panes.
function LoopStateBadge({ state }: { state: HomeOpenLoopState }) {
  const { t } = useTranslation();
  if (state === "attention") return <NeedsYouBadge />;
  if (state === "ready_to_close") {
    return <HomeStateBadge tone="good">{t("home.visual.openLoops.state.readyToClose")}</HomeStateBadge>;
  }
  return <HomeStateBadge>{t("home.visual.openLoops.state.waiting")}</HomeStateBadge>;
}

function ResponsibilityDetail({
  loop,
  onBack,
  onPreview,
}: {
  loop: HomeOpenLoopFixture;
  onBack: () => void;
  onPreview: (status: OpenLoopPreviewStatus) => void;
}) {
  const { t } = useTranslation();
  return (
    <article className="home-open-loops-detail flex min-w-0 flex-col gap-5" aria-labelledby={`${loop.id}-detail-heading`}>
      <Button
        className="home-open-loops-back -ml-3 self-start"
        onClick={onBack}
        size="sm"
        type="button"
        variant="ghost"
      >
        {t("home.visual.openLoops.back")}
      </Button>
      <div className="flex flex-col gap-2">
        <div className="flex flex-wrap items-center justify-between gap-2">
          <p className="wd-caption text-wd-tertiary">
            {t("home.visual.openLoops.confirmedResponsibility")}
          </p>
          <LoopStateBadge state={loop.state} />
        </div>
        <h3 className="wd-title text-wd-primary" id={`${loop.id}-detail-heading`}>
          {loop.label}
        </h3>
      </div>

      <p className="wd-body-l text-wd-primary">
        {loop.meaning}
      </p>

      <dl className="grid gap-x-8 gap-y-4 border-y-[length:var(--stroke-hairline)] border-wd-hairline py-4 sm:grid-cols-2">
        <div className="flex flex-col gap-0.5">
          <dt className="wd-caption text-wd-secondary">{t("home.visual.openLoops.owner")}</dt>
          <dd className="wd-body-s text-wd-primary">{t("home.visual.openLoops.ownerValue", { owner: loop.owner })}</dd>
        </div>
        <div className="flex flex-col gap-0.5">
          <dt className="wd-caption text-wd-secondary">{t("home.visual.openLoops.sourceStrength")}</dt>
          <dd className="wd-body-s text-wd-primary">{loop.sourceStrength}</dd>
        </div>
        <div className="flex flex-col gap-0.5">
          <dt className="wd-caption text-wd-secondary">{t("home.visual.openLoops.returnTrigger")}</dt>
          <dd className="wd-body-s text-wd-primary">{loop.trigger}</dd>
        </div>
        <div className="flex flex-col gap-0.5">
          <dt className="wd-caption text-wd-secondary">{t("home.visual.openLoops.recheck")}</dt>
          <dd className="wd-body-s text-wd-primary">{loop.recheck}</dd>
        </div>
      </dl>

      <section className="flex flex-col gap-1" aria-labelledby={`${loop.id}-provenance-heading`}>
        <h4 className="wd-label text-wd-secondary" id={`${loop.id}-provenance-heading`}>
          {t("home.visual.openLoops.provenance")}
        </h4>
        <p className="wd-body-s text-wd-primary">{loop.sourceSummary}</p>
        <p className="wd-caption text-wd-tertiary">{loop.lastConfirmedAt}</p>
        {loop.sourceGap ? (
          <p className="wd-body-s mt-2 flex items-start gap-1.5 rounded-md bg-wd-inset p-3 text-caution">
            <Clock aria-hidden="true" className="mt-0.5 size-3.5 shrink-0" />
            {t("home.visual.openLoops.knownGap", { gap: loop.sourceGap })}
          </p>
        ) : null}
      </section>

      <HomeCardActions
        lead={
          <>
            <Button data-quiet onClick={() => onPreview("correct")} size="sm" type="button" variant="ghost">
              {t("home.visual.openLoops.correct")}
            </Button>
            <Button data-quiet onClick={() => onPreview("context")} size="sm" type="button" variant="ghost">
              {t("home.visual.openLoops.addContext")}
            </Button>
          </>
        }
        trail={
          <Button onClick={() => onPreview("work")} size="sm" type="button" variant="primary">
            {t("home.visual.continueInWork")}
          </Button>
        }
      />
    </article>
  );
}

export function HomeOpenLoops({ fixture }: { fixture: HomeFixtureState }) {
  const { t } = useTranslation();
  const stateTabs: Array<{ state: HomeOpenLoopState; label: string }> = [
    { state: "attention", label: t("home.visual.openLoops.state.attention") },
    { state: "waiting", label: t("home.visual.openLoops.state.waiting") },
    { state: "ready_to_close", label: t("home.visual.openLoops.state.readyToClose") },
  ];
  const [activeState, setActiveState] = useState<HomeOpenLoopState>("attention");
  const [selectedId, setSelectedId] = useState<string>(
    () => fixture.openLoops.find((loop) => loop.state === "attention")?.id ?? fixture.openLoops[0]?.id ?? "",
  );
  const [mobileView, setMobileView] = useState<"index" | "detail">("index");
  const [previewStatus, setPreviewStatus] = useState<OpenLoopPreviewStatus>("initial");
  const selectedRowRef = useRef<HTMLButtonElement | null>(null);
  const shouldRestoreFocusRef = useRef(false);
  const visibleLoops = fixture.openLoops.filter((loop) => loop.state === activeState);
  const selectedLoop =
    fixture.openLoops.find((loop) => loop.id === selectedId) ?? visibleLoops[0] ?? fixture.openLoops[0];

  const chooseState = (state: HomeOpenLoopState) => {
    const first = fixture.openLoops.find((loop) => loop.state === state);
    setActiveState(state);
    setSelectedId(first?.id ?? "");
    setMobileView("index");
  };

  const returnToIndex = () => {
    shouldRestoreFocusRef.current = true;
    setMobileView("index");
  };

  useLayoutEffect(() => {
    if (mobileView !== "index" || !shouldRestoreFocusRef.current) return;
    shouldRestoreFocusRef.current = false;
    selectedRowRef.current?.focus({ preventScroll: true });
  }, [mobileView]);

  return (
    <section aria-labelledby="open-loops-heading" className="flex flex-col gap-6">
      <header className="flex max-w-(--layout-measure) flex-col gap-2">
        <p className="wd-caption text-wd-tertiary">
          {t("home.visual.openLoops.confirmedResponsibility")}
        </p>
        <h2 className="wd-display-3 text-wd-primary" id="open-loops-heading">
          {t("home.visual.openLoops.title")}
        </h2>
        <p className="wd-body text-wd-secondary">
          {t("home.visual.openLoops.description")}
        </p>
      </header>

      <div aria-label={t("home.visual.openLoops.states")} className="flex w-fit max-w-full flex-wrap gap-1 rounded-full bg-wd-inset p-1" role="tablist">
        {stateTabs.map((tab) => {
          const count = fixture.openLoops.filter((loop) => loop.state === tab.state).length;
          return (
            <button
              aria-selected={activeState === tab.state}
              className="wd-label inline-flex h-8 items-center gap-1.5 rounded-full px-3 text-wd-secondary transition-colors duration-fast hover:text-wd-primary aria-selected:bg-wd-inverse aria-selected:text-wd-on-inverse"
              key={tab.state}
              onClick={() => chooseState(tab.state)}
              role="tab"
              type="button"
            >
              {tab.label} <span className="wd-data">{count}</span>
            </button>
          );
        })}
      </div>

      <div className="home-open-loops-desk" data-mobile-view={mobileView}>
        <div className="home-open-loops-index flex min-w-0 flex-col gap-3">
          <p className="wd-caption text-wd-tertiary">
            {t("home.visual.openLoops.returnStateBoundary")}
          </p>
          <div className="flex flex-col">
            {visibleLoops.map((loop) => (
              <button
                aria-label={loop.label}
                aria-current={selectedLoop?.id === loop.id ? "true" : undefined}
                className={cn(
                  "-mx-3 flex min-h-11 w-[calc(100%+1.5rem)] items-center gap-3 rounded-md p-3 text-left transition-colors duration-fast hover:bg-wd-hover active:bg-wd-pressed aria-current:bg-wd-selected",
                  "[&+&]:rounded-t-none [&+&]:border-t-[length:var(--stroke-hairline)] [&+&]:border-wd-hairline",
                )}
                key={loop.id}
                onClick={(event) => {
                  selectedRowRef.current = event.currentTarget;
                  setSelectedId(loop.id);
                  setMobileView("detail");
                }}
                type="button"
              >
                <span className="flex min-w-0 flex-1 flex-col gap-0.5">
                  <span className="wd-body text-wd-primary">{loop.label}</span>
                  <span className="wd-caption text-wd-secondary">
                    {t("home.visual.openLoops.ownerValue", { owner: loop.owner })} · {loop.recheck}
                  </span>
                </span>
                <LoopStateBadge state={loop.state} />
                <ChevronRight aria-hidden="true" className="size-4 shrink-0 text-wd-tertiary" />
              </button>
            ))}
          </div>
        </div>

        {selectedLoop ? (
          <ResponsibilityDetail
            loop={selectedLoop}
            onBack={returnToIndex}
            onPreview={setPreviewStatus}
          />
        ) : null}
      </div>

      <p aria-label={t("home.visual.openLoops.statusLabel")} className="wd-caption max-w-(--layout-measure) text-wd-tertiary" role="status">
        {t(`home.visual.openLoops.status.${previewStatus}`)}
      </p>
    </section>
  );
}
