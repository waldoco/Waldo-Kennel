import { useState, type RefObject } from "react";
import { useTranslation } from "react-i18next";
import type { HomeFixtureState } from "../../lib/home-fixture";
import { Button } from "../ui/button";
import {
  HomeCard,
  HomeCardActions,
  HomeEyebrow,
  HomeFacts,
  HomeInset,
  HomeSection,
  NeedsYouBadge,
} from "./HomeCards";
import { ProvenanceInspector } from "./ProvenanceInspector";

/**
 * The one decision under Needs you, on the NeedsYouCard contract: the
 * subject names the action, the inset shows exactly what was said, the facts
 * are inline sentences, Skip leads quietly and the effect trails in ink.
 */
export function HomeCatchUp({
  fixture,
  headingRef,
  scrollContainerRef,
}: {
  fixture: HomeFixtureState;
  headingRef: RefObject<HTMLHeadingElement | null>;
  scrollContainerRef: RefObject<HTMLElement | null>;
}) {
  const { t } = useTranslation();
  const [handoffOpen, setHandoffOpen] = useState(false);
  const item = fixture.attention[0];
  if (!item) return null;

  return (
    <HomeSection
      headingId="needs-you-heading"
      headingRef={headingRef}
      title={t("home.visual.attention.needsYou")}
    >
      <HomeCard aria-labelledby="needs-you-subject">
        <HomeEyebrow meta={t("home.visual.catchUp.preview")}>
          <NeedsYouBadge />
          <span className="wd-caption text-wd-tertiary">
            {t("home.visual.position", { current: 1, total: fixture.attention.length })}
          </span>
        </HomeEyebrow>

        <h3 className="wd-title text-wd-primary [text-wrap:balance]" id="needs-you-subject">
          {item.proposedMeaning}
        </h3>

        <HomeInset className="-mx-3">
          <div className="-my-1 flex items-center justify-between gap-2">
            <span className="wd-caption text-wd-secondary">{t("home.visual.catchUp.userStatement")}</span>
            <Button className="-mr-3" size="sm" type="button" variant="ghost">
              {t("home.visual.correct")}
            </Button>
          </div>
          <blockquote className="wd-body-s border-l-2 border-wd-strong pl-3 text-wd-primary">
            {item.statement}
          </blockquote>
          <p className="wd-caption text-wd-tertiary">{t("home.visual.catchUp.proposalBoundary")}</p>
        </HomeInset>

        <HomeFacts
          facts={[
            { label: t("home.visual.catchUp.ifApprove"), value: t("home.visual.catchUp.ifApproveValue") },
            { label: t("home.visual.catchUp.ifSkip"), value: t("home.visual.catchUp.ifSkipValue") },
            {
              label: t("home.visual.catchUp.knownCaptureGap"),
              tone: "caution",
              value: (
                <>
                  <span>{item.sourceGap}</span>
                  <span>{t("home.visual.catchUp.gapSuffix")}</span>
                </>
              ),
            },
            { label: t("home.visual.catchUp.sourceSummary"), value: item.sourceSummary },
          ]}
        />

        <div className="flex flex-col gap-2">
          <p className="wd-caption text-wd-tertiary">{t("home.visual.catchUp.sourceBoundary")}</p>
          <div className="-ml-3">
            <ProvenanceInspector fixture={fixture} scrollContainerRef={scrollContainerRef} />
          </div>
        </div>

        <HomeCardActions
          lead={(
            <>
              <Button data-quiet size="sm" type="button" variant="ghost">{t("home.visual.catchUp.dismiss")}</Button>
              <Button data-quiet size="sm" type="button" variant="ghost">{t("home.visual.defer")}</Button>
              <Button data-quiet size="sm" type="button" variant="ghost">{t("home.visual.catchUp.keepNote")}</Button>
            </>
          )}
          trail={(
            <Button size="sm" type="button" variant="primary">
              {t("home.visual.catchUp.confirmOpenLoop")}
            </Button>
          )}
        />

        <div className="flex flex-col gap-2 border-t-[length:var(--stroke-hairline)] border-wd-hairline pt-3">
          <div className="-ml-3">
            <Button onClick={() => setHandoffOpen(true)} size="sm" type="button" variant="ghost">
              {t("home.visual.continueInWork")}
            </Button>
          </div>
          {handoffOpen ? (
            <HomeInset aria-label={t("home.visual.catchUp.handoffLabel")} className="-mx-3" role="region">
              <p className="wd-caption text-wd-secondary">{t("home.visual.catchUp.handoffPreview")}</p>
              <p className="wd-label text-wd-primary">{t("home.visual.catchUp.handoffTarget")}</p>
              <p className="wd-body-s text-wd-secondary">{t("home.visual.catchUp.handoffBoundary")}</p>
            </HomeInset>
          ) : null}
        </div>
      </HomeCard>
    </HomeSection>
  );
}
