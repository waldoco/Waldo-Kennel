import { useState, type RefObject } from "react";
import { useTranslation } from "react-i18next";
import type { HomeFixtureState } from "../../lib/home-fixture";
import { cn } from "../../lib/utils";
import { Button } from "../ui/button";
import { HomeFacts, HomeInset } from "./HomeCards";
import { HomeContextFrame } from "./HomeContextFrame";

export function HomePlansChanged({
  fixture,
  headingRef,
}: {
  fixture: HomeFixtureState;
  headingRef: RefObject<HTMLHeadingElement | null>;
}) {
  const { t } = useTranslation();
  const [choice, setChoice] = useState<string | null>(null);
  const change = fixture.planChange;
  const choices = [t("home.visual.keep"), t("home.visual.defer"), t("home.visual.release")];

  return (
    <HomeContextFrame
      eyebrow={t("home.visual.plansChanged.eyebrow")}
      fixture={fixture}
      headingRef={headingRef}
      title={t("home.visual.plansChanged.title")}
    >
      <HomeInset className="-mx-3">
        <div className="grid grid-cols-[minmax(0,7.5rem)_minmax(0,1fr)] items-baseline gap-3 text-wd-secondary">
          <span className="wd-caption">{t("home.visual.plansChanged.earlierAssumption")}</span>
          <p className="wd-body-s line-through decoration-(--text-tertiary)">{change.previousAssumption}</p>
        </div>
        <div className="grid grid-cols-[minmax(0,7.5rem)_minmax(0,1fr)] items-baseline gap-3 text-wd-primary">
          <span className="wd-caption text-wd-secondary">{t("home.visual.plansChanged.whatChanged")}</span>
          <p className="wd-body-s">{change.newFact}</p>
        </div>
      </HomeInset>

      <HomeFacts facts={[{ label: t("home.visual.waldoProposal"), value: change.proposal }]} />
      <p className="wd-caption text-wd-tertiary">
        {t("home.visual.plansChanged.proposalBoundary")} {change.sourceSummary}
      </p>

      <div className="flex flex-wrap gap-2">
        {choices.map((label) => (
          <Button
            aria-pressed={choice === label}
            className={cn(choice === label && "bg-wd-inverse text-wd-on-inverse hover:bg-wd-inverse")}
            key={label}
            onClick={() => setChoice(label)}
            size="sm"
            type="button"
            variant="secondary"
          >
            {label}
          </Button>
        ))}
      </div>
      {choice ? (
        <p className="wd-caption text-wd-secondary" role="status">
          {t("home.visual.plansChanged.selectionBoundary", { choice })}
        </p>
      ) : null}
    </HomeContextFrame>
  );
}
