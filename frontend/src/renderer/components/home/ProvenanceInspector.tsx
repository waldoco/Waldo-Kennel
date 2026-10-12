import { useEffect, useRef, useState, type RefObject } from "react";
import { useTranslation } from "react-i18next";
import type { HomeFixtureState } from "../../lib/home-fixture";
import { Button } from "../ui/button";
import { HomeFacts } from "./HomeCards";

export function ProvenanceInspector({
  fixture,
  scrollContainerRef,
}: {
  fixture: HomeFixtureState;
  scrollContainerRef: RefObject<HTMLElement | null>;
}) {
  const { t } = useTranslation();
  const triggerRef = useRef<HTMLButtonElement>(null);
  const dialogRef = useRef<HTMLDivElement>(null);
  const returnPosition = useRef(0);
  const [open, setOpen] = useState(false);
  const inspect = () => {
    returnPosition.current = scrollContainerRef.current?.scrollTop ?? 0;
    setOpen(true);
  };
  const returnToHome = () => {
    setOpen(false);
    triggerRef.current?.focus();
    if (scrollContainerRef.current) scrollContainerRef.current.scrollTop = returnPosition.current;
  };
  useEffect(() => {
    if (open) dialogRef.current?.focus();
  }, [open]);
  const onDialogKeyDown = (event: React.KeyboardEvent<HTMLDivElement>) => {
    if (event.key === "Escape") {
      event.preventDefault();
      returnToHome();
      return;
    }
  };
  return (
    <>
      <Button onClick={inspect} ref={triggerRef} size="sm" type="button" variant="ghost">
        {t("home.visual.provenance.inspect")}
      </Button>
      {open ? (
        <div
          aria-label={t("home.visual.provenance.label")}
          className="ml-3 mt-2 flex flex-col gap-3 rounded-md bg-wd-inset p-3"
          onKeyDown={onDialogKeyDown}
          ref={dialogRef}
          role="dialog"
          tabIndex={-1}
        >
          <h2 className="wd-label text-wd-primary">
            {t("home.visual.provenance.label")}
          </h2>
          <p className="wd-body-s text-wd-secondary">
            {t("home.visual.provenance.fixtureDescription", { sourceLabel: fixture.sourceLabel })}
          </p>
          <HomeFacts
            facts={[
              { label: t("home.visual.provenance.originalSource"), value: t("home.visual.provenance.originalStatement") },
              { label: t("home.visual.knownGap"), tone: "caution", value: t("home.visual.provenance.gapDescription") },
              { label: t("home.visual.provenance.boundary"), value: t("home.visual.provenance.boundaryDescription") },
            ]}
          />
          <div>
            <Button onClick={returnToHome} size="sm" type="button" variant="secondary">
              {t("home.visual.provenance.returnToCatchUp")}
            </Button>
          </div>
        </div>
      ) : null}
    </>
  );
}
