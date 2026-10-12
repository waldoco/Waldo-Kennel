import { useRef, useState, type KeyboardEvent } from "react";
import { useTranslation } from "react-i18next";
import type {
  HomeContinuityEvent,
  HomeContinuityEventKind,
  HomeFixtureState,
  HomeInsightCandidate,
} from "../../lib/home-fixture";
import { cn } from "../../lib/utils";
import { Badge } from "../ui/badge";
import { Button } from "../ui/button";
import {
  HomeCard,
  HomeCardActions,
  HomeEyebrow,
  HomeFacts,
  HomeInset,
  HomeStateBadge,
  type HomeStateTone,
} from "./HomeCards";

type InsightLayer = "insights" | "records";
type RecordsLayer = "continuity" | "activity";
type InsightStatus = "Candidate" | "Confirmed" | "Corrected" | "Dismissed";

const statusTone: Record<InsightStatus, HomeStateTone> = {
  Candidate: "neutral",
  Confirmed: "good",
  Corrected: "neutral",
  Dismissed: "neutral",
};

// The inset track with the ink thumb (components/ui Tabs), kept as a hand-built
// tablist so the roving focus and arrow keys stay exactly as they were.
const tabListClass = "flex w-fit max-w-full flex-wrap gap-1 rounded-full bg-wd-inset p-1";
const tabClass =
  "wd-label inline-flex h-8 items-center rounded-full px-3 text-wd-secondary transition-colors duration-fast hover:text-wd-primary aria-selected:bg-wd-inverse aria-selected:text-wd-on-inverse";
const hairlineRows =
  "[&>li+li]:border-t-[length:var(--stroke-hairline)] [&>li+li]:border-wd-hairline";

function moveTab<T extends string>(
  event: KeyboardEvent<HTMLButtonElement>,
  order: readonly T[],
  current: T,
  onSelect: (next: T) => void,
) {
  if (event.key !== "ArrowLeft" && event.key !== "ArrowRight") return;
  event.preventDefault();
  const offset = event.key === "ArrowRight" ? 1 : -1;
  const next = order[(order.indexOf(current) + offset + order.length) % order.length];
  const tablist = event.currentTarget.parentElement;
  tablist?.querySelector<HTMLButtonElement>(`[data-home-layer="${next}"]`)?.focus();
  onSelect(next);
}

function InsightCard({ candidate }: { candidate: HomeInsightCandidate }) {
  const { t } = useTranslation();
  const [status, setStatus] = useState<InsightStatus>("Candidate");
  const [whyVisible, setWhyVisible] = useState(false);
  const [correcting, setCorrecting] = useState(false);
  const [correction, setCorrection] = useState("");
  const [appliedCorrection, setAppliedCorrection] = useState("");

  return (
    <HomeCard aria-labelledby={`${candidate.id}-heading`}>
      <HomeEyebrow meta={candidate.freshness}>
        <HomeStateBadge tone={statusTone[status]}>{status}</HomeStateBadge>
      </HomeEyebrow>
      <h3 className="wd-headline text-wd-primary" id={`${candidate.id}-heading`}>
        {candidate.title}
      </h3>
      <HomeFacts
        facts={[
          { id: "observed", label: t("home.visual.insights.directlyObserved"), value: candidate.observation },
          { id: "window", label: t("home.visual.insights.sourceWindow"), value: candidate.sourceWindow },
          { id: "inference", label: t("home.visual.insights.inferenceBoundary"), value: candidate.inferenceBoundary },
          { id: "gaps", label: t("home.visual.insights.knownGaps"), tone: "caution", value: candidate.gaps },
          { id: "route", label: t("home.visual.insights.providerDisclosure"), value: candidate.providerDisclosure },
        ]}
      />
      {whyVisible ? (
        <HomeInset>
          <p className="wd-body-s text-wd-secondary">{candidate.whyItMayMatter}</p>
          <p className="wd-body-s text-wd-primary">{candidate.suggestedJudgment}</p>
        </HomeInset>
      ) : null}
      {correcting ? (
        <HomeInset>
          <label className="wd-label text-wd-primary" htmlFor={`${candidate.id}-correction`}>
            {t("home.visual.insights.correction")}
          </label>
          <textarea
            aria-label={t("home.visual.insights.correctionFor", {
              title: candidate.title,
            })}
            className="wd-body min-h-20 w-full resize-y rounded-sm bg-wd-surface px-3 py-2 text-wd-primary placeholder:text-wd-tertiary focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-(--border-focus)"
            id={`${candidate.id}-correction`}
            onChange={(event) => setCorrection(event.target.value)}
            placeholder={t("home.visual.insights.correctionPlaceholder")}
            value={correction}
          />
          <div className="flex flex-wrap items-center justify-end gap-2">
            <Button className="mr-auto -ml-3" onClick={() => setCorrecting(false)} size="sm" type="button" variant="ghost">
              {t("home.visual.insights.cancel")}
            </Button>
            <Button
              disabled={correction.trim() === ""}
              onClick={() => {
                setAppliedCorrection(correction.trim());
                setStatus("Corrected");
                setCorrecting(false);
              }}
              size="sm"
              type="button"
              variant="primary"
            >
              {t("home.visual.insights.applyCorrection")}
            </Button>
          </div>
        </HomeInset>
      ) : null}
      {appliedCorrection ? (
        <HomeInset>
          <p className="wd-caption text-wd-tertiary">
            {t("home.visual.insights.correctionApplied")}
          </p>
          <p className="wd-body-s text-wd-primary">
            {appliedCorrection}
          </p>
        </HomeInset>
      ) : null}
      <div
        aria-label={t("home.visual.insights.review", {
          title: candidate.title,
        })}
        role="group"
      >
        <HomeCardActions
          hint={t("home.visual.insights.reviewBoundary")}
          lead={
            <>
              <Button
                aria-expanded={whyVisible}
                data-quiet
                onClick={() => setWhyVisible(!whyVisible)}
                size="sm"
                type="button"
                variant="ghost"
              >
                {t("home.visual.insights.whyThis")}
              </Button>
              <Button data-quiet onClick={() => setStatus("Dismissed")} size="sm" type="button" variant="ghost">
                {t("home.visual.insights.dismiss")}
              </Button>
            </>
          }
          trail={
            <>
              <Button onClick={() => setCorrecting(true)} size="sm" type="button" variant="secondary">
                {t("home.visual.insights.correct")}
              </Button>
              <Button onClick={() => setStatus("Confirmed")} size="sm" type="button" variant="primary">
                {t("home.visual.insights.confirm")}
              </Button>
            </>
          }
        />
      </div>
    </HomeCard>
  );
}

function EventDetail({
  event,
  kindLabel,
  onBack,
}: {
  event: HomeContinuityEvent;
  kindLabel: string;
  onBack: () => void;
}) {
  const { t } = useTranslation();
  return (
    <article
      className="home-history-detail flex min-w-0 flex-col gap-5"
      aria-labelledby={`${event.id}-history-detail-heading`}
    >
      <Button
        className="home-history-back -ml-3 self-start"
        onClick={onBack}
        size="sm"
        type="button"
        variant="ghost"
      >
        {t("home.visual.history.back")}
      </Button>
      <div className="flex flex-col gap-2">
        <div className="flex flex-wrap items-center justify-between gap-2">
          <p className="wd-caption text-wd-tertiary">{kindLabel}</p>
          <time className="wd-data text-wd-tertiary">{event.time}</time>
        </div>
        <h3
          className="wd-title text-wd-primary"
          id={`${event.id}-history-detail-heading`}
        >
          {event.title}
        </h3>
      </div>
      <section
        className="flex flex-col gap-2 border-y-[length:var(--stroke-hairline)] border-wd-hairline py-4"
        aria-labelledby={`${event.id}-statement-heading`}
      >
        <h4
          className="wd-label text-wd-secondary"
          id={`${event.id}-statement-heading`}
        >
          {t("home.visual.history.recordSays")}
        </h4>
        <p className="wd-body-l text-wd-primary">
          {event.detail}
        </p>
      </section>
      <HomeFacts
        facts={[
          { id: "source", label: t("home.visual.source"), value: event.sourceSummary },
          { id: "boundary", label: t("home.visual.architectureBoundary"), value: event.boundary },
        ]}
      />
    </article>
  );
}

function RecordsView({ fixture, initialRecordId }: { fixture: HomeFixtureState; initialRecordId?: string }) {
  const { t } = useTranslation();
  const [layer, setLayer] = useState<RecordsLayer>("continuity");
  const selectedFromRoute = fixture.continuity.some((event) => event.id === initialRecordId)
    ? initialRecordId
    : undefined;
  const [selectedId, setSelectedId] = useState(selectedFromRoute ?? fixture.continuity[0]?.id ?? "");
  const [mobileView, setMobileView] = useState<"index" | "detail">(
    selectedFromRoute ? "detail" : "index",
  );
  const selectedRowRef = useRef<HTMLButtonElement | null>(null);
  const selected =
    fixture.continuity.find((event) => event.id === selectedId) ??
    fixture.continuity[0];
  const latestId = fixture.continuity[fixture.continuity.length - 1]?.id;
  const kindLabels: Record<HomeContinuityEventKind, string> = {
    observation: t("home.visual.history.kind.observation"),
    correction: t("home.visual.history.kind.correction"),
    work_link_preview: t("home.visual.history.kind.workLinkPreview"),
    close_receipt_preview: t("home.visual.history.kind.closePreview"),
    reentry: t("home.visual.history.kind.reentry"),
  };

  return (
    <section className="flex flex-col gap-5" aria-label={t("home.visual.insights.records")}>
      <p className="wd-body-s max-w-(--layout-measure) text-wd-secondary">
        {t("home.visual.insights.recordsDescription")}
      </p>
      <div
        aria-label={t("home.visual.insights.recordLayers")}
        className={tabListClass}
        role="tablist"
      >
        <button
          aria-selected={layer === "continuity"}
          className={tabClass}
          data-home-layer="continuity"
          onKeyDown={(event) => moveTab(event, ["continuity", "activity"], "continuity", setLayer)}
          onClick={() => setLayer("continuity")}
          role="tab"
          tabIndex={layer === "continuity" ? 0 : -1}
          type="button"
        >
          {t("home.visual.insights.continuity")}
        </button>
        <button
          aria-selected={layer === "activity"}
          className={tabClass}
          data-home-layer="activity"
          onKeyDown={(event) => moveTab(event, ["continuity", "activity"], "activity", setLayer)}
          onClick={() => setLayer("activity")}
          role="tab"
          tabIndex={layer === "activity" ? 0 : -1}
          type="button"
        >
          {t("home.visual.history.supportingActivity")}
        </button>
      </div>
      {layer === "continuity" ? (
        <div className="home-history-desk" data-mobile-view={mobileView}>
          <ol
            aria-label={t("home.visual.history.continuityHistory")}
            className={cn("home-history-spine flex min-w-0 flex-col", hairlineRows)}
          >
            {fixture.continuity.map((event) => (
              <li className="relative py-1 first:pt-0 last:pb-0" key={event.id}>
                {event.id === latestId ? (
                  <span
                    aria-hidden="true"
                    className="absolute top-1/2 -left-4 h-5 w-0.5 -translate-y-1/2 rounded-full bg-wd-inverse"
                  />
                ) : null}
                <button
                  aria-current={selected?.id === event.id ? "true" : undefined}
                  aria-label={event.title}
                  className="-mx-3 flex min-h-11 w-[calc(100%+1.5rem)] flex-col gap-1 rounded-md px-3 py-2.5 text-left transition-colors duration-fast hover:bg-wd-hover active:bg-wd-pressed aria-current:bg-wd-selected"
                  onClick={(clickEvent) => {
                    selectedRowRef.current = clickEvent.currentTarget;
                    setSelectedId(event.id);
                    setMobileView("detail");
                  }}
                  type="button"
                >
                  <span className="flex w-full items-baseline gap-3">
                    <time className="wd-data shrink-0 text-wd-tertiary">
                      {event.time}
                    </time>
                    <span
                      className={cn(
                        "wd-label min-w-0 flex-1",
                        event.id === latestId ? "text-wd-primary" : "text-wd-secondary",
                      )}
                    >
                      {event.title}
                    </span>
                  </span>
                  <Badge className="self-start">{kindLabels[event.kind]}</Badge>
                </button>
              </li>
            ))}
          </ol>
          {selected ? (
            <EventDetail
              event={selected}
              kindLabel={kindLabels[selected.kind]}
              onBack={() => {
                setMobileView("index");
                selectedRowRef.current?.focus({ preventScroll: true });
              }}
            />
          ) : null}
        </div>
      ) : (
        <section
          className="flex max-w-(--layout-column) flex-col gap-3"
          aria-labelledby="activity-evidence-heading"
        >
          <h3
            className="wd-headline text-wd-primary"
            id="activity-evidence-heading"
          >
            {t("home.visual.history.activityTitle")}
          </h3>
          <p className="wd-body-s text-wd-secondary">
            {t("home.visual.history.activityDescription")}
          </p>
          <ol
            className={cn("mt-2 flex flex-col", hairlineRows)}
            aria-label={t("home.visual.history.activityEvidence")}
          >
            {fixture.continuity.map((event) => (
              <li
                className="grid gap-1 py-3 first:pt-0 last:pb-0 sm:grid-cols-[5rem_minmax(0,1fr)] sm:gap-3"
                key={event.id}
              >
                <time className="wd-data text-wd-tertiary">
                  {event.time}
                </time>
                <div className="flex min-w-0 flex-col gap-0.5">
                  <p className="wd-body-s text-wd-primary">
                    {event.sourceSummary}
                  </p>
                  <p className="wd-caption text-wd-secondary">
                    {t("home.visual.history.evidenceFor", {
                      title: event.title,
                    })}
                  </p>
                </div>
              </li>
            ))}
          </ol>
        </section>
      )}
    </section>
  );
}

export function HomeHistory({
  fixture,
  initialRecordId,
  previewEnabled = true,
}: {
  fixture: HomeFixtureState;
  initialRecordId?: string;
  previewEnabled?: boolean;
}) {
  const { t } = useTranslation();
  const [layer, setLayer] = useState<InsightLayer>(initialRecordId ? "records" : "insights");
  return (
    <section aria-labelledby="history-heading" className="flex flex-col gap-6">
      <header className="flex max-w-(--layout-measure) flex-col gap-2">
        <p className="wd-caption text-wd-tertiary">
          {t("home.visual.history.eyebrow")}
        </p>
        <h2
          className="wd-display-3 text-wd-primary"
          id="history-heading"
        >
          {t("home.visual.history.title")}
        </h2>
        <p className="wd-body text-wd-secondary">
          {t("home.visual.history.description")}
        </p>
      </header>
      {!previewEnabled ? (
        <div className="flex max-w-(--layout-measure) flex-col gap-1 py-2">
          <h3 className="wd-headline text-wd-primary">
            {t("home.visual.insights.emptyTitle")}
          </h3>
          <p className="wd-body-s text-wd-secondary">
            {t("home.visual.insights.emptyDescription")}
          </p>
        </div>
      ) : (
        <>
          <div
            aria-label={t("home.visual.history.layers")}
            className={tabListClass}
            role="tablist"
          >
            <button
              aria-selected={layer === "insights"}
              className={tabClass}
              data-home-layer="insights"
              onKeyDown={(event) => moveTab(event, ["insights", "records"], "insights", setLayer)}
              onClick={() => setLayer("insights")}
              role="tab"
              tabIndex={layer === "insights" ? 0 : -1}
              type="button"
            >
              {t("home.visual.insights.insights")}
            </button>
            <button
              aria-selected={layer === "records"}
              className={tabClass}
              data-home-layer="records"
              onKeyDown={(event) => moveTab(event, ["insights", "records"], "records", setLayer)}
              onClick={() => setLayer("records")}
              role="tab"
              tabIndex={layer === "records" ? 0 : -1}
              type="button"
            >
              {t("home.visual.insights.records")}
            </button>
          </div>
          {layer === "insights" ? (
            <div className="flex max-w-(--layout-column) flex-col gap-4">
              <HomeInset
                aria-label={t("home.visual.insights.previewAria")}
                className="gap-1"
                role="status"
              >
                <p className="wd-label text-wd-primary">
                  {t("home.visual.insights.previewTitle")}
                </p>
                <p className="wd-caption text-wd-secondary">
                  {t("home.visual.insights.previewDescription")}
                </p>
              </HomeInset>
              {fixture.insights.map((candidate) => (
                <InsightCard candidate={candidate} key={candidate.id} />
              ))}
            </div>
          ) : (
            <RecordsView fixture={fixture} initialRecordId={initialRecordId} />
          )}
        </>
      )}
    </section>
  );
}
