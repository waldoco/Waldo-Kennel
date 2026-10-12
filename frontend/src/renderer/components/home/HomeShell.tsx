import { Clock } from "lucide-react";
import { useRef } from "react";
import { useTranslation } from "react-i18next";
import { CenterPanelShell } from "../CenterPanelShell";
import { WaldoRail } from "../waldo/WaldoRail";
import { useWaldoRail } from "../waldo/WaldoRailContext";
import {
  homeFixture,
  type HomeAvailability,
  type HomeDestination,
  type HomeFixtureState,
} from "../../lib/home-fixture";
import { cn } from "../../lib/utils";
import { HomeBrief } from "./HomeBrief";
import { HomeContextPanel } from "./HomeContextPanel";
import { HomeDestinationView } from "./HomeDestinationView";
import { HomeCarrying, HomeReentry, HomeTodayGlance } from "./HomeOutcomes";
import { HomeQuickCapture } from "./HomeQuickCapture";
import { usesWaldoUiPreview } from "../../lib/preview-mode";

function HomeAvailabilityStatus({ fixture }: { fixture: HomeFixtureState }) {
  const { t } = useTranslation();
  const availabilityContent: Record<HomeAvailability, { title: string; description: string }> = {
    ready: {
      title: t("home.visual.availability.ready.title"),
      description: t("home.visual.availability.ready.description"),
    },
    partial: {
      title: t("home.visual.availability.partial.title"),
      description: t("home.visual.availability.partial.description"),
    },
    capture_off: {
      title: t("home.visual.availability.captureOff.title"),
      description: t("home.visual.availability.captureOff.description"),
    },
    stale: {
      title: t("home.visual.availability.stale.title"),
      description: t("home.visual.availability.stale.description"),
    },
    offline: {
      title: t("home.visual.availability.offline.title"),
      description: t("home.visual.availability.offline.description"),
    },
  };
  const content = availabilityContent[fixture.availability];
  // Missing or old facts are recoverable: caution with a clock and the words.
  const caution =
    fixture.availability === "partial" ||
    fixture.availability === "stale" ||
    fixture.availability === "offline";
  return (
    <section
      aria-label={t("home.visual.availability.label")}
      className="wd-caption flex flex-wrap items-baseline gap-x-2 gap-y-1"
      role="status"
    >
      <p className={cn("inline-flex items-center gap-1.5", caution ? "text-caution" : "text-wd-secondary")}>
        {caution ? (
          <Clock aria-hidden="true" className="size-3.5 shrink-0" />
        ) : (
          <span aria-hidden="true" className="size-1.5 shrink-0 rounded-full bg-wd-control" />
        )}
        {content.title}
      </p>
      <p className="text-wd-tertiary">{content.description}</p>
    </section>
  );
}

export function HomeShell({
  fixture = homeFixture("today"),
  destination = "today",
  initialRecordId,
  previewEnabled = usesWaldoUiPreview,
}: {
  fixture?: HomeFixtureState;
  destination?: HomeDestination;
  initialRecordId?: string;
  previewEnabled?: boolean;
}) {
  const { t } = useTranslation();
  const waldo = useWaldoRail();
  const scrollContainerRef = useRef<HTMLElement>(null);
  const reviewRef = useRef<HTMLButtonElement>(null);
  const contextHeadingRef = useRef<HTMLHeadingElement>(null);
  const showToday = destination === "today";
  const offlineToday = showToday && fixture.availability === "offline";
  const focusContext = () => contextHeadingRef.current?.focus({ preventScroll: false });
  const destinationLabel: Record<HomeDestination, string> = {
    today: t("home.visual.navigation.today"),
    chat: t("home.visual.navigation.chat"),
    open_loops: t("home.visual.openLoops.title"),
    daily_close: t("home.visual.dailyClose.title"),
    memory: t("home.visual.memory.title"),
    history: t("home.visual.history.title"),
  };
  const waldoContextLabel = `${t("home.visual.title")} · ${destinationLabel[destination]}`;
  // Pages with a master/detail desk keep their two panes up to the page
  // width; everything else is one authored column, extra width is margin.
  const destinationWidth =
    destination === "chat"
      ? "max-w-none"
      : destination === "daily_close"
        ? "max-w-(--layout-column)"
        : "max-w-(--layout-page)";

  return (
    <CenterPanelShell titlebarAlign={false}>
      <div className="relative flex min-h-0 flex-1">
        <section
          aria-labelledby="home-heading"
          className="flex min-h-0 flex-1 flex-col overflow-y-auto bg-wd-canvas"
          ref={scrollContainerRef}
        >
          <h1 className="sr-only" id="home-heading">{t("home.visual.title")}</h1>

          {offlineToday ? (
            <div className="home-today-column">
              <div className="home-today-body">
                <HomeAvailabilityStatus fixture={fixture} />
                <section aria-labelledby="offline-heading" className="flex flex-col gap-2">
                  <h2 className="wd-title text-wd-primary" id="offline-heading">
                    {t("home.visual.offline.title")}
                  </h2>
                  <p className="wd-body max-w-(--layout-measure) text-wd-secondary">
                    {t("home.visual.offline.description")}
                  </p>
                </section>
              </div>
              <div className="home-today-foot">
                <HomeQuickCapture placeholder={fixture.presentation.capturePlaceholder} />
              </div>
            </div>
          ) : null}

          {showToday && !offlineToday ? (
            <div className="home-today-layout">
              <div className="home-today-column">
                <div className="home-today-body">
                  <HomeAvailabilityStatus fixture={fixture} />
                  <HomeBrief
                    fixture={fixture}
                    onReview={focusContext}
                    reviewRef={reviewRef}
                  />
                  <HomeContextPanel
                    fixture={fixture}
                    headingRef={contextHeadingRef}
                    scrollContainerRef={scrollContainerRef}
                  />
                  <HomeCarrying fixture={fixture} />
                  <HomeReentry fixture={fixture} />
                </div>
                <div className="home-today-foot">
                  <HomeQuickCapture placeholder={fixture.presentation.capturePlaceholder} />
                </div>
              </div>
              {waldo.isOpen ? null : <HomeTodayGlance fixture={fixture} />}
            </div>
          ) : null}

          {destination !== "today" ? (
            <div
              className={cn(
                "home-destination-surface mx-auto flex w-full min-h-full flex-1 flex-col gap-6",
                destination === "chat" ? "px-5 pb-6 pt-8 sm:px-6" : "px-6 pb-12 pt-12",
                destinationWidth,
              )}
            >
              <div className="flex flex-col gap-1">
                <HomeAvailabilityStatus fixture={fixture} />
                <p className="wd-caption text-wd-tertiary">{fixture.localDateLabel}</p>
              </div>
              <HomeDestinationView destination={destination} fixture={fixture} initialRecordId={initialRecordId} previewEnabled={previewEnabled} />
            </div>
          ) : null}
        </section>
        {waldo.isOpen ? (
          <div className="waldo-home-layer">
            <WaldoRail
              contextLabel={waldoContextLabel}
              onClose={waldo.close}
              previewEnabled={previewEnabled}
            />
          </div>
        ) : null}
      </div>
    </CenterPanelShell>
  );
}
