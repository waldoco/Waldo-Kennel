import { Check, Clock } from "lucide-react";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import type { HomeFixtureState } from "../../lib/home-fixture";
import { Badge } from "../ui/badge";
import { Button } from "../ui/button";
import { HomeCard, HomeCardActions } from "./HomeCards";

type DailyCloseStage = "review" | "preview_receipt";
type PreviewDisposition = "still_open" | "defer" | "release" | "resume_tomorrow";

// The close of the day on the Waldo CloseCard contract: one authored column,
// each list under its visible label, hairlines between groups, the known
// limitation as a caution lead-in with its word and shape, and the close as
// the one ink effect trailing the action unit.
const groupLabel = "wd-label text-wd-secondary";
const groupDivider = "border-t-[length:var(--stroke-hairline)] border-wd-hairline pt-4";

export function HomeDailyClose({ fixture }: { fixture: HomeFixtureState }) {
  const { t } = useTranslation();
  const dispositionOptions: Array<{ value: PreviewDisposition; label: string }> = [
    { value: "still_open", label: t("home.visual.dailyClose.disposition.stillOpen") },
    { value: "defer", label: t("home.visual.defer") },
    { value: "release", label: t("home.visual.release") },
    { value: "resume_tomorrow", label: t("home.visual.dailyClose.disposition.resumeTomorrow") },
  ];
  const dispositionLabels: Record<PreviewDisposition, string> = {
    still_open: t("home.visual.dailyClose.disposition.stillOpen"),
    defer: t("home.visual.dailyClose.disposition.deferredPreview"),
    release: t("home.visual.dailyClose.disposition.releasedPreview"),
    resume_tomorrow: t("home.visual.dailyClose.disposition.resumeTomorrow"),
  };
  const [stage, setStage] = useState<DailyCloseStage>("review");
  const [dispositions, setDispositions] = useState<Record<string, PreviewDisposition>>(
    () => Object.fromEntries(
      fixture.closureReview.unresolved.map((item) => [item.id, "still_open"]),
    ),
  );
  const [reentryId, setReentryId] = useState(
    fixture.closureReview.reentryOptions[0]?.id ?? "",
  );
  const [showCloseNote, setShowCloseNote] = useState(false);
  const [closeNote, setCloseNote] = useState("");
  const reentry = fixture.closureReview.reentryOptions.find((item) => item.id === reentryId);

  if (stage === "preview_receipt") {
    return (
      <section aria-labelledby="daily-close-preview-heading" className="flex flex-col gap-6">
        <header className="flex flex-col gap-2">
          <p className="wd-caption text-wd-tertiary">
            {t("home.visual.dailyClose.closureReview")}
          </p>
          <h2 className="wd-display-3 text-wd-primary" id="daily-close-preview-heading">
            {t("home.visual.dailyClose.previewTitle")}
          </h2>
          <p className="wd-body-s text-wd-secondary">
            {t("home.visual.dailyClose.previewReceiptBoundary")}
          </p>
        </header>

        <HomeCard className="home-daily-close-receipt">
          <section className="flex flex-col gap-2" aria-labelledby="receipt-became-true-heading">
            <h3 className={groupLabel} id="receipt-became-true-heading">
              {t("home.visual.dailyClose.reviewedFacts")}
            </h3>
            <ul className="flex flex-col gap-3">
              {fixture.closureReview.becameTrue.map((fact) => (
                <li className="flex items-start gap-2" key={fact.id}>
                  <Check aria-hidden="true" className="mt-0.5 size-4 shrink-0 text-wd-secondary" />
                  <div className="flex min-w-0 flex-col gap-0.5">
                    <p className="wd-body-s text-wd-primary">{fact.label}</p>
                    <p className="wd-caption text-wd-secondary">{fact.evidence}</p>
                  </div>
                </li>
              ))}
            </ul>
          </section>

          <section className={`flex flex-col gap-2 ${groupDivider}`} aria-labelledby="receipt-dispositions-heading">
            <h3 className={groupLabel} id="receipt-dispositions-heading">
              {t("home.visual.dailyClose.previewDispositions")}
            </h3>
            <ul className="flex flex-col gap-2">
              {fixture.closureReview.unresolved.map((item) => (
                <li className="flex flex-wrap items-center justify-between gap-2" key={item.id}>
                  <span className="wd-body-s text-wd-primary">{item.label}</span>
                  <Badge>{dispositionLabels[dispositions[item.id] ?? "still_open"]}</Badge>
                </li>
              ))}
            </ul>
          </section>

          <section className={`flex flex-col gap-2 ${groupDivider}`} aria-labelledby="receipt-gaps-heading">
            <h3 className="wd-label flex items-center gap-1 text-caution" id="receipt-gaps-heading">
              <Clock aria-hidden="true" className="size-3.5" />
              {t("home.visual.knownSourceGaps")}
            </h3>
            {fixture.closureReview.sourceGaps.map((gap) => (
              <p className="wd-body-s text-wd-secondary" key={gap.id}>{gap.label}</p>
            ))}
          </section>

          <section className={`flex flex-col gap-2 ${groupDivider}`} aria-labelledby="receipt-reentry-heading">
            <h3 className={groupLabel} id="receipt-reentry-heading">
              {t("home.visual.dailyClose.tomorrowReentry")}
            </h3>
            <p className="wd-body-s text-wd-primary">{reentry?.label}</p>
            {closeNote ? (
              <p className="wd-caption text-wd-secondary">{closeNote}</p>
            ) : null}
          </section>

          <HomeCardActions
            hint={t("home.visual.dailyClose.receiptBoundary")}
            lead={
              <Button data-quiet onClick={() => setStage("review")} size="sm" type="button" variant="ghost">
                {t("home.visual.dailyClose.returnReview")}
              </Button>
            }
            trail={
              <>
                <Button asChild size="sm" variant="secondary">
                  <a href="#/home/history">{t("home.visual.dailyClose.inspectHistory")}</a>
                </Button>
                <Button asChild size="sm" variant="primary">
                  <a href="#/home">{t("home.visual.dailyClose.returnToday")}</a>
                </Button>
              </>
            }
          />
        </HomeCard>
      </section>
    );
  }

  return (
    <section aria-labelledby="close-day-heading" className="flex flex-col gap-6">
      <h2 className="sr-only">{t("home.visual.dailyClose.title")}</h2>
      <header className="flex flex-col gap-2">
        <p className="wd-caption text-wd-tertiary">
          {t("home.visual.dailyClose.eyebrow")}
        </p>
        <h2 className="wd-display-3 text-wd-primary" id="close-day-heading">
          {t("home.visual.dailyClose.heading")}
        </h2>
        <p className="wd-body text-wd-secondary">
          {t("home.visual.dailyClose.description")}
        </p>
        <p className="wd-caption text-wd-tertiary">
          {t("home.visual.dailyClose.reviewBoundary")}
        </p>
      </header>

      <HomeCard className="home-daily-close-review-grid">
        <section className="flex flex-col gap-2" aria-labelledby="became-true-heading">
          <h3 className={groupLabel} id="became-true-heading">{t("home.visual.whatBecameTrue")}</h3>
          <ul className="flex flex-col gap-3">
            {fixture.closureReview.becameTrue.map((fact) => (
              <li className="flex items-start gap-2" key={fact.id}>
                <Check aria-hidden="true" className="mt-0.5 size-4 shrink-0 text-wd-secondary" />
                <div className="flex min-w-0 flex-col gap-0.5">
                  <p className="wd-body-s text-wd-primary">{fact.label}</p>
                  <p className="wd-caption text-wd-secondary">{fact.evidence}</p>
                </div>
              </li>
            ))}
          </ul>
        </section>

        <section className={`flex flex-col gap-2 ${groupDivider}`} aria-labelledby="remains-heading">
          <h3 className={groupLabel} id="remains-heading">{t("home.visual.dailyClose.remainsUnresolved")}</h3>
          <div className="flex flex-col">
            {fixture.closureReview.unresolved.map((item) => (
              <article
                className="flex flex-col gap-1 py-3 first:pt-0 last:pb-0 [&+&]:border-t-[length:var(--stroke-hairline)] [&+&]:border-wd-hairline"
                key={item.id}
              >
                <p className="wd-label text-wd-primary">{item.label}</p>
                <p className="wd-caption text-wd-secondary">{item.meaning}</p>
                <div aria-label={t("home.visual.dailyClose.dispositionFor", { label: item.label })} className="mt-2 flex flex-wrap gap-2" role="radiogroup">
                  {dispositionOptions.map((option) => (
                    <label className="cursor-pointer" key={option.value}>
                      <input
                        checked={(dispositions[item.id] ?? "still_open") === option.value}
                        className="peer sr-only"
                        name={`disposition-${item.id}`}
                        onChange={() => setDispositions((current) => ({ ...current, [item.id]: option.value }))}
                        type="radio"
                        value={option.value}
                      />
                      <span className="wd-label inline-flex h-8 items-center rounded-full border border-wd-strong px-3 text-wd-primary transition-colors duration-fast hover:bg-wd-hover peer-checked:border-transparent peer-checked:bg-wd-inverse peer-checked:text-wd-on-inverse peer-focus-visible:outline-2 peer-focus-visible:outline-offset-2 peer-focus-visible:outline-(--border-focus)">
                        {option.label}
                      </span>
                    </label>
                  ))}
                </div>
              </article>
            ))}
          </div>
        </section>

        <section className={`flex flex-col gap-2 ${groupDivider}`} aria-labelledby="source-gaps-heading">
          <h3 className="wd-label flex items-center gap-1 text-caution" id="source-gaps-heading">
            <Clock aria-hidden="true" className="size-3.5" />
            {t("home.visual.knownSourceGaps")}
          </h3>
          {fixture.closureReview.sourceGaps.map((gap) => (
            <p className="wd-body-s text-wd-secondary" key={gap.id}>{gap.label}</p>
          ))}
        </section>

        <fieldset className={`flex min-w-0 flex-col gap-2 ${groupDivider}`}>
          <legend className="wd-label float-left mb-2 w-full text-wd-primary">{t("home.visual.dailyClose.tomorrowQuestion")}</legend>
          <div className="clear-both flex flex-col gap-2" role="radiogroup" aria-label={t("home.visual.dailyClose.tomorrowReentry")}>
            {fixture.closureReview.reentryOptions.map((option) => {
              const selected = reentryId === option.id;
              return (
                <label
                  className={`flex min-h-11 cursor-pointer items-start gap-3 rounded-md border-[length:var(--stroke-hairline)] bg-wd-surface p-3 transition-colors duration-fast hover:bg-wd-hover has-[:focus-visible]:outline-2 has-[:focus-visible]:outline-offset-2 has-[:focus-visible]:outline-(--border-focus) ${selected ? "border-wd-strong" : "border-wd-hairline"}`}
                  key={option.id}
                >
                  <input checked={selected} className="sr-only" name="tomorrow-reentry" onChange={() => setReentryId(option.id)} type="radio" />
                  <span
                    aria-hidden="true"
                    className={`mt-0.5 grid size-4 shrink-0 place-items-center rounded-full border ${selected ? "border-transparent bg-wd-inverse" : "border-wd-control"}`}
                  >
                    {selected ? <span className="size-1.5 rounded-full bg-wd-on-inverse" /> : null}
                  </span>
                  <span className="wd-body-s text-wd-primary">{option.label}</span>
                </label>
              );
            })}
          </div>
        </fieldset>

        <div className={groupDivider}>
          {!showCloseNote ? (
            <Button className="-ml-3" data-quiet onClick={() => setShowCloseNote(true)} size="sm" type="button" variant="ghost">
              {t("home.visual.dailyClose.addNote")}
            </Button>
          ) : (
            <label className="wd-label flex flex-col gap-2 text-wd-secondary">
              {t("home.visual.dailyClose.closeNote")}
              <textarea
                aria-label={t("home.visual.dailyClose.closeNote")}
                className="wd-body min-h-20 w-full resize-y rounded-md bg-wd-inset px-3 py-2 text-wd-primary placeholder:text-wd-tertiary focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-(--border-focus)"
                onChange={(event) => setCloseNote(event.target.value)}
                placeholder={t("home.visual.dailyClose.notePlaceholder")}
                value={closeNote}
              />
            </label>
          )}
        </div>

        <HomeCardActions
          hint={t("home.visual.dailyClose.stateBoundary")}
          trail={
            <Button onClick={() => setStage("preview_receipt")} size="sm" type="button" variant="primary">
              {t("home.visual.dailyClose.reviewComplete")}
            </Button>
          }
        />
      </HomeCard>
    </section>
  );
}
