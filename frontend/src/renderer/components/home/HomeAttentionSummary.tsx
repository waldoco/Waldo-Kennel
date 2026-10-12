import type { RefObject } from "react";
import { useTranslation } from "react-i18next";
import type { HomeFixtureState } from "../../lib/home-fixture";
import { NeedsYouBadge } from "./HomeCards";

export function HomeAttentionSummary({
  fixture,
  onReview,
  reviewRef,
}: {
  fixture: HomeFixtureState;
  onReview: () => void;
  reviewRef: RefObject<HTMLButtonElement | null>;
}) {
  const { t } = useTranslation();
  const item = fixture.attention[0];
  if (!item) {
    return (
      <section aria-label={t("home.visual.attention.needsYou")}>
        <p className="wd-body text-wd-primary">{t("home.visual.attention.empty")}</p>
      </section>
    );
  }

  return (
    <section aria-labelledby="needs-you-heading" className="flex flex-col gap-3">
      <h2 className="wd-title text-wd-primary" id="needs-you-heading">
        {t("home.visual.attention.needsYou")}
      </h2>
      <button
        aria-label={t("home.visual.brief.review.catchUp")}
        className="group block rounded-lg text-left"
        onClick={onReview}
        ref={reviewRef}
        type="button"
      >
        <span className="flex flex-col gap-3 rounded-lg border-[length:var(--stroke-hairline)] border-wd-hairline bg-wd-surface p-5 transition-colors duration-fast group-hover:bg-wd-hover group-active:bg-wd-pressed motion-reduce:transition-none">
          <span className="flex min-h-7 items-center gap-2">
            <NeedsYouBadge />
            <span className="wd-caption ml-auto text-wd-tertiary">{t("home.visual.attention.oneDecision")}</span>
          </span>
          <span className="wd-headline block text-wd-primary">
            {t("home.visual.attention.proposedMeaning")}
          </span>
          <span className="wd-body-s block text-wd-secondary">{item.sourceSummary}</span>
          <span className="wd-body-s block text-caution">{item.sourceGap}</span>
        </span>
      </button>
    </section>
  );
}
