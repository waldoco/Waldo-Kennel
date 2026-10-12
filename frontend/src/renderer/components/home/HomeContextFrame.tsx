import type { ReactNode, RefObject } from "react";
import { useTranslation } from "react-i18next";
import type { HomeFixtureState } from "../../lib/home-fixture";
import { HomeCard, HomeEyebrow, HomeSection } from "./HomeCards";

/**
 * One Today context section in the authored column: a sentence-case section
 * header (focus target for the Brief's review action) over a single card
 * whose eyebrow carries the moment and where the facts came from.
 */
export function HomeContextFrame({
  children,
  eyebrow,
  fixture,
  footer,
  headingRef,
  title,
}: {
  children: ReactNode;
  eyebrow: string;
  fixture: HomeFixtureState;
  footer?: ReactNode;
  headingRef: RefObject<HTMLHeadingElement | null>;
  title: string;
}) {
  const { t } = useTranslation();
  return (
    <HomeSection headingId="home-context-heading" headingRef={headingRef} title={title}>
      <HomeCard>
        <HomeEyebrow meta={`${fixture.sourceLabel} ${t("home.visual.contextFixtureBoundary")}`}>
          <span className="wd-caption text-wd-secondary">{eyebrow}</span>
        </HomeEyebrow>
        {children}
        {footer}
      </HomeCard>
    </HomeSection>
  );
}
