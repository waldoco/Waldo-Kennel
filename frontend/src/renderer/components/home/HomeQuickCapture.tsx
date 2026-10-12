import { ArrowUp } from "lucide-react";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "../ui/button";

let sessionDraft = "";

/**
 * Capture at the foot of the column, on the Composer contract: a pill field
 * on bg-inset and an ink round send, never Spot. Capture is a preview here,
 * so send stays unavailable and the line under the field says why.
 */
export function HomeQuickCapture({
  placeholder,
}: {
  placeholder?: string;
}) {
  const { t } = useTranslation();
  const [draft, setDraft] = useState(sessionDraft);
  const resolvedPlaceholder = placeholder ?? t("home.visual.quickCapture.placeholder");
  const reason = t("home.visual.quickCapture.disclosure");

  return (
    <section aria-label={t("home.visual.quickCapture.label")} className="flex flex-col gap-1.5">
      <div className="flex items-end gap-1">
        <label className="flex min-h-12 min-w-0 flex-1 items-center rounded-full border-[length:var(--stroke-hairline)] border-wd-hairline bg-wd-inset px-4 transition-[outline-color] duration-fast focus-within:outline-2 focus-within:outline-offset-2 focus-within:outline-(--border-focus) motion-reduce:transition-none">
          <input
            aria-label={t("home.visual.quickCapture.label")}
            className="wd-body min-w-0 flex-1 bg-transparent py-3 text-wd-primary outline-none placeholder:text-wd-tertiary focus-visible:outline-none"
            onChange={(event) => {
              sessionDraft = event.target.value;
              setDraft(event.target.value);
            }}
            placeholder={resolvedPlaceholder}
            type="text"
            value={draft}
          />
        </label>
        <span className="grid size-12 shrink-0 place-items-center" title={reason}>
          <Button
            aria-label={t("home.visual.quickCapture.send")}
            className="size-11 rounded-full"
            disabled
            size="icon"
            type="button"
            variant="primary"
          >
            <ArrowUp aria-hidden="true" />
          </Button>
        </span>
      </div>
      <p className="wd-caption px-4 text-wd-tertiary">{reason}</p>
    </section>
  );
}
