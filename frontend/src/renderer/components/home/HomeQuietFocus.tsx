import type { RefObject } from "react";
import { useTranslation } from "react-i18next";
import type { HomeFixtureState } from "../../lib/home-fixture";
import { HomeContextFrame } from "./HomeContextFrame";

/**
 * Nothing earned an interruption: one true line and what Waldo does next.
 * The way back into the work is the Catch up card below, not a second link.
 */
export function HomeQuietFocus({
  fixture,
  headingRef,
}: {
  fixture: HomeFixtureState;
  headingRef: RefObject<HTMLHeadingElement | null>;
}) {
  const { t } = useTranslation();
  return (
    <HomeContextFrame
      eyebrow={t("home.visual.quietFocus.eyebrow")}
      fixture={fixture}
      headingRef={headingRef}
      title={t("home.visual.quietFocus.title")}
    >
      <p className="wd-headline text-wd-primary">{t("home.visual.quietFocus.empty")}</p>
      <p className="wd-body-s max-w-(--layout-measure) text-wd-secondary">
        {t("home.visual.quietFocus.description")}
      </p>
    </HomeContextFrame>
  );
}
