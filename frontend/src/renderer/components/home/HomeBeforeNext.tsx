import { useState, type RefObject } from "react";
import { useTranslation } from "react-i18next";
import type { HomeFixtureState } from "../../lib/home-fixture";
import { Button } from "../ui/button";
import { HomeCardActions, HomeFacts } from "./HomeCards";
import { HomeContextFrame } from "./HomeContextFrame";

export function HomeBeforeNext({
  fixture,
  headingRef,
}: {
  fixture: HomeFixtureState;
  headingRef: RefObject<HTMLHeadingElement | null>;
}) {
  const { t } = useTranslation();
  const [previewOpen, setPreviewOpen] = useState(false);
  const next = fixture.nextThing;

  return (
    <HomeContextFrame
      eyebrow={next.startsAt}
      fixture={fixture}
      headingRef={headingRef}
      title={t("home.visual.beforeNext.title")}
    >
      <div aria-label={next.title} className="flex flex-col gap-3" role="article">
        <h3 className="wd-headline text-wd-primary">{next.title}</h3>
        <p className="wd-body max-w-(--layout-measure) text-wd-primary">{next.framing}</p>

        <section aria-labelledby="promises-heading" className="flex flex-col gap-1">
          <h4 className="wd-label text-wd-secondary" id="promises-heading">
            {t("home.visual.beforeNext.promises")}
          </h4>
          <ul className="flex flex-col" role="list">
            {next.promises.map((promise) => (
              <li
                className="wd-body-s border-wd-hairline py-2 text-wd-primary [&+&]:border-t-[length:var(--stroke-hairline)]"
                key={promise.id}
              >
                {promise.label}
              </li>
            ))}
          </ul>
        </section>

        <section aria-labelledby="questions-heading" className="flex flex-col gap-1">
          <h4 className="wd-label text-wd-secondary" id="questions-heading">
            {t("home.visual.beforeNext.openQuestion")}
          </h4>
          {next.openQuestions.map((question) => (
            <p className="wd-body-s text-wd-primary" key={question.id}>
              {question.label}
            </p>
          ))}
        </section>

        <HomeFacts
          facts={[
            { label: t("home.visual.sourcesSearched"), value: next.sourceSummary },
            { label: t("home.visual.knownGap"), tone: "caution", value: next.sourceGap },
          ]}
        />

        <HomeCardActions
          hint={previewOpen ? (
            <span role="status">{t("home.visual.workPreviewBoundary")}</span>
          ) : undefined}
          trail={(
            <Button onClick={() => setPreviewOpen(true)} size="sm" type="button" variant="primary">
              {t("home.visual.prepareInWork")}
            </Button>
          )}
        />
      </div>
    </HomeContextFrame>
  );
}
