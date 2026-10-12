import { useState, type RefObject } from "react";
import { useTranslation } from "react-i18next";
import type { HomeFixtureState } from "../../lib/home-fixture";
import type { HomeContextFlow } from "../../lib/home-day-phase";
import { Button } from "../ui/button";
import { HomeCard, HomeCardActions, HomeInset } from "./HomeCards";

type ProposalChoice = "accepted" | "declined";

/**
 * Home's opening, on the BriefCard contract: the date line, the greeting in
 * Mottle, written prose at the reading measure, and at most one proposal in
 * an inset action unit whose accept is the screen's only Spot.
 */
export function HomeBrief({
  fixture,
  onReview,
  reviewRef,
}: {
  fixture: HomeFixtureState;
  onReview: () => void;
  reviewRef: RefObject<HTMLButtonElement | null>;
}) {
  const { t } = useTranslation();
  const [reviewed, setReviewed] = useState(false);
  const [choice, setChoice] = useState<ProposalChoice | null>(null);
  const reviewLabels: Record<HomeContextFlow, string> = {
    catch_up: t("home.visual.brief.review.catchUp"),
    before_next: t("home.visual.brief.review.beforeNext"),
    plans_changed: t("home.visual.brief.review.plansChanged"),
    evening_review: t("home.visual.brief.review.evening"),
    quiet_focus: t("home.visual.brief.review.quietFocus"),
  };
  const hasAttention = fixture.attention.length > 0;
  const showFinish = fixture.contextFlow === "catch_up";
  const proposal = fixture.suggestions[0];

  return (
    <HomeCard aria-labelledby="home-now-heading" hero>
      <div className="flex flex-col gap-2">
        <div className="wd-caption flex flex-wrap items-baseline gap-x-1 text-wd-tertiary">
          <h3 className="wd-caption" id="home-brief-section-heading">
            {fixture.presentation.briefLabel}
          </h3>
          <span aria-hidden="true">·</span>
          <span>{fixture.localDateLabel}</span>
        </div>
        <h2 className="wd-display-2 text-wd-primary" id="home-now-heading">
          {fixture.presentation.greeting}
        </h2>
      </div>

      <div className="wd-body-l flex flex-col gap-3 text-wd-primary">
        <p>
          {hasAttention ? fixture.presentation.attentionSummary : t("home.visual.brief.noReview")}
        </p>
        {fixture.brief.length > 0 ? <p>{fixture.brief.join(" ")}</p> : null}
      </div>

      {hasAttention || showFinish ? (
        <div className="-ml-3 flex flex-wrap items-center gap-1">
          {hasAttention ? (
            <Button onClick={onReview} ref={reviewRef} size="sm" type="button" variant="ghost">
              {reviewLabels[fixture.contextFlow]}
            </Button>
          ) : null}
          {showFinish ? (
            <Button
              aria-pressed={reviewed}
              onClick={() => setReviewed(true)}
              size="sm"
              type="button"
              variant="ghost"
            >
              {reviewed
                ? t("home.visual.catchUp.reviewed", { briefLabel: fixture.presentation.briefLabel })
                : fixture.presentation.finishLabel}
            </Button>
          ) : null}
        </div>
      ) : null}

      {proposal ? (
        <HomeInset aria-label={fixture.presentation.suggestionLabel} className="-mx-3" role="group">
          <div className="wd-caption flex flex-wrap items-baseline justify-between gap-2 text-wd-tertiary">
            <span>{fixture.presentation.suggestionLabel}</span>
            <span>{t("home.visual.brief.proposed")}</span>
          </div>
          <p className="wd-body text-wd-primary">{proposal.label}</p>
          {choice ? (
            <p className="wd-caption text-wd-secondary" role="status">
              {choice === "accepted" ? t("home.visual.brief.accepted") : t("home.visual.brief.declined")}
            </p>
          ) : (
            <HomeCardActions
              lead={(
                <Button data-quiet onClick={() => setChoice("declined")} size="sm" type="button" variant="ghost">
                  {t("home.visual.brief.decline")}
                </Button>
              )}
              trail={(
                <Button onClick={() => setChoice("accepted")} size="sm" type="button" variant="spot">
                  {t("home.visual.brief.accept")}
                </Button>
              )}
            />
          )}
        </HomeInset>
      ) : null}

      <p className="wd-caption flex flex-wrap gap-x-1 text-wd-tertiary">
        <span className="after:content-[':']">{t("home.visual.source")}</span>
        <span>{fixture.sourceLabel}</span>
      </p>
    </HomeCard>
  );
}
