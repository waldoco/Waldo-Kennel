import { ChevronRight, Clock } from "lucide-react";
import { useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import type { HomeFixtureState, HomeMemoryCandidate } from "../../lib/home-fixture";
import { cn } from "../../lib/utils";
import { Badge } from "../ui/badge";
import { Button } from "../ui/button";
import { HomeCardActions, HomeFacts } from "./HomeCards";

type MemoryPreviewStatus = "review" | "correction" | "rejected" | "deferred";

// Memory review on the Waldo MemoryClaim contract: a radius-md frame with the
// kind tag leading, the statement in body, the provenance as quiet facts, and
// the action unit with Forget and Defer leading as quiet buttons and Correct
// trailing as the secondary button. The desk keeps its master/detail panes,
// divided by a hairline.
function CandidateDetail({
  candidate,
  correction,
  onBack,
  onCorrectionChange,
  onOutcome,
  showCorrection,
}: {
  candidate: HomeMemoryCandidate;
  correction: string;
  onBack: () => void;
  onCorrectionChange: (value: string) => void;
  onOutcome: (outcome: "correct" | "reject" | "defer") => void;
  showCorrection: boolean;
}) {
  const { t } = useTranslation();
  const sensitivityLabel: Record<HomeMemoryCandidate["sensitivity"], string> = {
    ordinary: t("home.visual.memory.sensitivity.ordinary"),
    sensitive: t("home.visual.memory.sensitivity.sensitive"),
  };
  return (
    <article className="home-memory-review-detail flex min-w-0 flex-col gap-4" aria-labelledby={`${candidate.id}-memory-heading`}>
      <Button className="home-memory-review-back -ml-3 self-start" onClick={onBack} size="sm" type="button" variant="ghost">
        {t("home.visual.memory.back")}
      </Button>

      <div className="flex min-w-0 flex-col gap-3 rounded-md border-[length:var(--stroke-hairline)] border-wd-hairline bg-wd-surface p-4">
        <div className="flex flex-wrap items-center gap-2">
          <p className="wd-caption rounded-xs border border-wd-control px-2 py-0.5 text-wd-secondary">
            {t("home.visual.memory.candidateBoundary")}
          </p>
          <span className="wd-caption ml-auto text-wd-tertiary">
            {sensitivityLabel[candidate.sensitivity]}
          </span>
        </div>
        <h3 className="wd-headline text-wd-primary" id={`${candidate.id}-memory-heading`}>
          {candidate.label}
        </h3>

        <p className="wd-body text-wd-primary">
          {candidate.statement}
        </p>

        <HomeFacts
          facts={[
            { id: "origin", label: t("home.visual.memory.origin"), value: candidate.sourceSummary },
            { id: "validity", label: t("home.visual.memory.validity"), value: candidate.validUntil },
            { id: "uncertainty", label: t("home.visual.memory.uncertainty"), value: candidate.uncertainty },
          ]}
        />

        {candidate.sourceGap ? (
          <section className="flex flex-col gap-1 rounded-md bg-wd-inset p-3" aria-labelledby={`${candidate.id}-gap-heading`}>
            <h4 className="wd-label flex items-center gap-1 text-caution" id={`${candidate.id}-gap-heading`}>
              <Clock aria-hidden="true" className="size-3.5" />
              {t("home.visual.knownSourceGap")}
            </h4>
            <p className="wd-body-s text-wd-secondary">{candidate.sourceGap}</p>
          </section>
        ) : null}

        {showCorrection ? (
          <label className="wd-label flex flex-col gap-2 text-wd-secondary">
            {t("home.visual.memory.correction")}
            <textarea
              aria-label={t("home.visual.memory.correction")}
              className="wd-body min-h-24 w-full resize-y rounded-sm bg-wd-inset px-3 py-2 text-wd-primary focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-(--border-focus)"
              onChange={(event) => onCorrectionChange(event.target.value)}
              value={correction}
            />
          </label>
        ) : null}

        <HomeCardActions
          lead={
            <>
              <Button data-quiet onClick={() => onOutcome("reject")} size="sm" type="button" variant="ghost">
                {t("home.visual.reject")}
              </Button>
              <Button data-quiet onClick={() => onOutcome("defer")} size="sm" type="button" variant="ghost">
                {t("home.visual.defer")}
              </Button>
            </>
          }
          trail={
            <Button onClick={() => onOutcome("correct")} size="sm" type="button" variant="secondary">
              {t("home.visual.correct")}
            </Button>
          }
        />
      </div>
    </article>
  );
}

export function HomeMemoryReview({ fixture }: { fixture: HomeFixtureState }) {
  const { t } = useTranslation();
  const [selectedId, setSelectedId] = useState(fixture.memoryCandidates[0]?.id ?? "");
  const [mobileView, setMobileView] = useState<"index" | "detail">("index");
  const [status, setStatus] = useState<MemoryPreviewStatus>("review");
  const [correctingId, setCorrectingId] = useState<string>();
  const [corrections, setCorrections] = useState<Record<string, string>>({});
  const selectedRowRef = useRef<HTMLButtonElement | null>(null);
  const selected =
    fixture.memoryCandidates.find((candidate) => candidate.id === selectedId) ??
    fixture.memoryCandidates[0];

  const handleOutcome = (outcome: "correct" | "reject" | "defer") => {
    if (!selected) return;
    if (outcome === "correct") {
      setCorrectingId(selected.id);
      setCorrections((current) => ({
        ...current,
        [selected.id]: current[selected.id] ?? selected.statement,
      }));
      setStatus("correction");
      return;
    }
    setCorrectingId(undefined);
    setStatus(outcome === "reject" ? "rejected" : "deferred");
  };

  return (
    <section aria-labelledby="memory-review-heading" className="flex flex-col gap-6">
      <header className="flex max-w-(--layout-measure) flex-col gap-2">
        <p className="wd-caption text-wd-tertiary">
          {t("home.visual.memory.eyebrow")}
        </p>
        <h2 className="wd-display-3 text-wd-primary" id="memory-review-heading">
          {t("home.visual.memory.title")}
        </h2>
        <p className="wd-body text-wd-secondary">
          {t("home.visual.memory.description")}
        </p>
      </header>

      <div className="home-memory-review-desk" data-mobile-view={mobileView}>
        <div className="home-memory-review-index flex min-w-0 flex-col gap-3">
          <div className="flex items-baseline justify-between gap-3">
            <p className="wd-label text-wd-secondary">{t("home.visual.memory.candidates")}</p>
            <span className="wd-caption text-wd-tertiary">{t("home.visual.memory.toReview", { count: fixture.memoryCandidates.length })}</span>
          </div>
          <div className="flex flex-col">
            {fixture.memoryCandidates.map((candidate) => (
              <button
                aria-current={selected?.id === candidate.id ? "true" : undefined}
                aria-label={candidate.label}
                className={cn(
                  "-mx-3 flex min-h-11 w-[calc(100%+1.5rem)] items-center gap-3 rounded-md p-3 text-left transition-colors duration-fast hover:bg-wd-hover active:bg-wd-pressed aria-current:bg-wd-selected",
                  "[&+&]:rounded-t-none [&+&]:border-t-[length:var(--stroke-hairline)] [&+&]:border-wd-hairline",
                )}
                key={candidate.id}
                onClick={(event) => {
                  selectedRowRef.current = event.currentTarget;
                  setSelectedId(candidate.id);
                  setCorrectingId(undefined);
                  setMobileView("detail");
                }}
                type="button"
              >
                <span className="flex min-w-0 flex-1 flex-col gap-0.5">
                  <span className="wd-body text-wd-primary">{candidate.label}</span>
                </span>
                <Badge>{t("home.visual.memory.needsReview")}</Badge>
                <ChevronRight aria-hidden="true" className="size-4 shrink-0 text-wd-tertiary" />
              </button>
            ))}
          </div>
        </div>

        {selected ? (
          <CandidateDetail
            candidate={selected}
            correction={corrections[selected.id] ?? selected.statement}
            onBack={() => {
              setMobileView("index");
              selectedRowRef.current?.focus({ preventScroll: true });
            }}
            onCorrectionChange={(value) => setCorrections((current) => ({ ...current, [selected.id]: value }))}
            onOutcome={handleOutcome}
            showCorrection={correctingId === selected.id}
          />
        ) : null}
      </div>

      <p aria-label={t("home.visual.memory.statusLabel")} className="wd-caption max-w-(--layout-measure) text-wd-tertiary" role="status">
        {t(`home.visual.memory.status.${status}`)}
      </p>
    </section>
  );
}
