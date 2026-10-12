import { ChevronRight, History } from "lucide-react";
import { useTranslation } from "react-i18next";
import type { HomeFixtureState, HomeOpenLoopFixture } from "../../lib/home-fixture";
import { Button } from "../ui/button";
import {
  HomeCard,
  HomeCardActions,
  HomeEyebrow,
  HomeFacts,
  HomeSection,
  HomeStateBadge,
} from "./HomeCards";

/**
 * Waldo is carrying: the confirmed loops someone other than the person is
 * moving, or that Waldo has carried to the point of closing. Each is an
 * OutcomeCard whose whole surface is the link to its detail. The section
 * disappears, header included, when there is nothing to carry.
 */
export function HomeCarrying({ fixture }: { fixture: HomeFixtureState }) {
  const { t } = useTranslation();
  const carried = fixture.openLoops.filter((loop) => loop.state !== "attention");
  if (carried.length === 0) return null;
  return (
    <HomeSection headingId="home-carrying-heading" title={t("home.visual.carrying.title")}>
      {carried.map((loop) => (
        <HomeCarriedLoop key={loop.id} loop={loop} />
      ))}
    </HomeSection>
  );
}

function HomeCarriedLoop({ loop }: { loop: HomeOpenLoopFixture }) {
  const { t } = useTranslation();
  const titleId = `home-carrying-${loop.id}`;
  return (
    <a
      aria-labelledby={titleId}
      className="group block rounded-lg text-inherit no-underline"
      href="#/home/open-loops"
    >
      <HomeCard className="transition-colors duration-fast group-hover:bg-wd-hover group-active:bg-wd-pressed motion-reduce:transition-none">
        <HomeEyebrow
          meta={(
            <HomeStateBadge tone={loop.state === "ready_to_close" ? "good" : "neutral"}>
              {loop.state === "ready_to_close"
                ? t("home.visual.openLoops.state.readyToClose")
                : t("home.visual.openLoops.state.waiting")}
            </HomeStateBadge>
          )}
        >
          <span className="wd-caption text-wd-secondary">
            {t("home.visual.openLoops.ownerValue", { owner: loop.owner })}
          </span>
        </HomeEyebrow>
        <h3 className="wd-headline text-wd-primary" id={titleId}>{loop.label}</h3>
        <HomeFacts
          facts={[
            { label: t("home.visual.carrying.next"), value: loop.meaning },
            { label: t("home.visual.carrying.when"), value: loop.recheck },
            { label: t("home.visual.carrying.evidence"), value: loop.lastConfirmedAt },
          ]}
        />
      </HomeCard>
    </a>
  );
}

/**
 * Catch up, on the ReEntryCard contract: the exact point to pick something
 * back up and one way in.
 */
export function HomeReentry({ fixture }: { fixture: HomeFixtureState }) {
  const { t } = useTranslation();
  return (
    <HomeSection headingId="home-reentry-heading" title={t("home.visual.catchUp.title")}>
      <HomeCard aria-labelledby="home-reentry-title">
        <HomeEyebrow>
          <History aria-hidden="true" className="size-4 text-wd-tertiary" />
          <span className="wd-caption text-wd-secondary">{t("home.visual.reentry.eyebrow")}</span>
        </HomeEyebrow>
        <h3 className="wd-headline text-wd-primary" id="home-reentry-title">
          {fixture.reentry.label}
        </h3>
        <HomeFacts
          facts={[{ label: t("home.visual.reentry.whereLeft"), value: fixture.reentry.detail }]}
        />
        <HomeCardActions
          trail={(
            <Button asChild size="sm" variant="primary">
              <a href="#/home/open-loops">
                {t("home.visual.reentry.action")}
                <ChevronRight aria-hidden="true" />
              </a>
            </Button>
          )}
        />
      </HomeCard>
    </HomeSection>
  );
}

/**
 * The quiet inspector beside the column: a glance at today's names only,
 * with where they came from. Never a second column of content.
 */
export function HomeTodayGlance({ fixture }: { fixture: HomeFixtureState }) {
  const { t } = useTranslation();
  if (fixture.todos.length === 0) return null;
  return (
    <aside aria-label={t("home.visual.inspector.label")} className="home-today-inspector">
      <div className="flex flex-col gap-3 rounded-lg border-[length:var(--stroke-hairline)] border-wd-hairline bg-wd-surface p-4">
        <h2 className="wd-label-l text-wd-primary" id="home-glance-heading">
          {fixture.presentation.primaryListLabel}
        </h2>
        <ul aria-labelledby="home-glance-heading" className="flex flex-col gap-1" role="list">
          {fixture.todos.map((item) => (
            <li className="wd-caption text-wd-secondary" key={item.id}>
              {item.label}
            </li>
          ))}
        </ul>
        <p className="wd-caption text-wd-tertiary">{fixture.sourceLabel}</p>
      </div>
    </aside>
  );
}
