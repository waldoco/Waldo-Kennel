import { Check } from "lucide-react";
import type { RefObject } from "react";
import { useTranslation } from "react-i18next";
import type { HomeFixtureState } from "../../lib/home-fixture";
import { Button } from "../ui/button";
import { HomeCardActions } from "./HomeCards";
import { HomeContextFrame } from "./HomeContextFrame";

/** The evening transition on the CloseCard contract; only the person closes. */
export function HomeEveningReview({
  fixture,
  headingRef,
}: {
  fixture: HomeFixtureState;
  headingRef: RefObject<HTMLHeadingElement | null>;
}) {
  const { t } = useTranslation();
  const review = fixture.closureReview;
  const gap = review.sourceGaps[0]?.label;
  return (
    <HomeContextFrame
      eyebrow={t("home.visual.evening.transitionBoundary")}
      fixture={fixture}
      footer={(
        <HomeCardActions
          hint={t("home.visual.evening.reviewBoundary")}
          trail={(
            <Button asChild size="sm" variant="primary">
              <a href="#/home/daily-close">{t("home.visual.evening.startClosure")}</a>
            </Button>
          )}
        />
      )}
      headingRef={headingRef}
      title={t("home.visual.evening.title")}
    >
      {review.becameTrue.length > 0 ? (
        <section aria-labelledby="evening-became-true-heading" className="flex flex-col gap-2">
          <h3 className="wd-label text-wd-secondary" id="evening-became-true-heading">
            {t("home.visual.whatBecameTrue")}
          </h3>
          <ul className="flex flex-col gap-2" role="list">
            {review.becameTrue.map((fact) => (
              <li className="flex items-start gap-2" key={fact.id}>
                <Check aria-hidden="true" className="mt-0.5 size-4 shrink-0 text-good" />
                <div className="flex flex-col gap-0.5">
                  <p className="wd-body-s text-wd-primary">{fact.label}</p>
                  <p className="wd-caption text-wd-secondary">{fact.evidence}</p>
                </div>
              </li>
            ))}
          </ul>
        </section>
      ) : null}
      {review.unresolved.length > 0 ? (
        <section aria-labelledby="evening-unresolved-heading" className="flex flex-col gap-2">
          <h3 className="wd-label text-wd-secondary" id="evening-unresolved-heading">
            {t("home.visual.stillUnresolved")}
          </h3>
          <ul className="flex flex-col" role="list">
            {review.unresolved.map((item) => (
              <li
                className="flex flex-col gap-0.5 border-wd-hairline py-2 first:pt-0 [&+&]:border-t-[length:var(--stroke-hairline)]"
                key={item.id}
              >
                <p className="wd-body-s text-wd-primary">{item.label}</p>
                <p className="wd-caption text-wd-secondary">{item.meaning}</p>
              </li>
            ))}
          </ul>
        </section>
      ) : null}
      {gap ? (
        <section aria-labelledby="evening-gap-heading" className="flex flex-col gap-1">
          <h3 className="wd-label text-caution" id="evening-gap-heading">
            {t("home.visual.knownSourceGap")}
          </h3>
          <p className="wd-body-s text-wd-secondary">{gap}</p>
        </section>
      ) : null}
    </HomeContextFrame>
  );
}
