// Home's card anatomy on the Waldo system (see the Waldo design system's
// BriefCard, NeedsYouCard, OutcomeCard, ReEntryCard, SectionHeader and
// StateBadge contracts). Home-scoped building blocks over components/ui:
// cards are surface + hairline at radius-lg with no shadow, the inset unit is
// bg-inset at radius-md with no border, facts are inline sentences with a
// secondary lead-in, and the action unit puts the way out leading (quiet) and
// the effect trailing.
import { Check, Clock, X } from "lucide-react";
import type { ComponentProps, ReactNode, Ref } from "react";
import { useTranslation } from "react-i18next";
import { cn } from "../../lib/utils";
import { Badge } from "../ui/badge";

export function HomeSection({
  children,
  className,
  headingId,
  headingRef,
  title,
}: {
  children: ReactNode;
  className?: string;
  headingId: string;
  headingRef?: Ref<HTMLHeadingElement>;
  title: string;
}) {
  return (
    <section aria-labelledby={headingId} className={cn("flex flex-col gap-3", className)}>
      <h2
        className="wd-title text-wd-primary"
        id={headingId}
        ref={headingRef}
        tabIndex={headingRef ? -1 : undefined}
      >
        {title}
      </h2>
      {children}
    </section>
  );
}

export function HomeCard({
  className,
  hero = false,
  ...props
}: ComponentProps<"article"> & { hero?: boolean }) {
  return (
    <article
      className={cn(
        "flex min-w-0 flex-col gap-3 border-[length:var(--stroke-hairline)] border-wd-hairline bg-wd-surface p-5 text-wd-primary",
        hero ? "gap-4 rounded-xl" : "rounded-lg",
        className,
      )}
      {...props}
    />
  );
}

/** The one nested frame inside a card: bg-inset, radius-md, no border. */
export function HomeInset({ className, ...props }: ComponentProps<"div">) {
  return (
    <div
      className={cn("flex min-w-0 flex-col gap-2 rounded-md bg-wd-inset p-3", className)}
      {...props}
    />
  );
}

export function HomeEyebrow({
  children,
  meta,
}: {
  children: ReactNode;
  meta?: ReactNode;
}) {
  return (
    <div className="flex min-h-7 flex-wrap items-center gap-2">
      {children}
      {meta ? <span className="wd-caption ml-auto text-wd-tertiary">{meta}</span> : null}
    </div>
  );
}

/** The Needs You badge: the spot dot plus the words. */
export function NeedsYouBadge() {
  const { t } = useTranslation();
  return (
    <Badge variant="spot">
      <span aria-hidden="true" className="size-1.5 rounded-full bg-spot" />
      {t("home.visual.attention.needsYou")}
    </Badge>
  );
}

export type HomeStateTone = "neutral" | "good" | "caution" | "critical";

/** One state word with a tone and a shape, so colour never carries it alone. */
export function HomeStateBadge({ children, tone = "neutral" }: { children: ReactNode; tone?: HomeStateTone }) {
  const variant = tone === "good" ? "success" : tone === "caution" ? "warning" : tone === "critical" ? "error" : "neutral";
  return (
    <Badge variant={variant}>
      {tone === "good" ? <Check aria-hidden="true" /> : null}
      {tone === "caution" ? <Clock aria-hidden="true" /> : null}
      {tone === "critical" ? <X aria-hidden="true" /> : null}
      {children}
    </Badge>
  );
}

export type HomeFact = { id?: string; label: string; value: ReactNode; tone?: "caution" };

/**
 * Inline facts on the content axis: the lead-in in text-secondary, the
 * sentence in text-primary. The colon is drawn, so the lead-in's text stays
 * the plain words.
 */
export function HomeFacts({ className, facts }: { className?: string; facts: HomeFact[] }) {
  return (
    <dl className={cn("wd-body-s flex flex-col gap-2", className)}>
      {facts.map((fact) => (
        <div key={fact.id ?? fact.label}>
          <dt
            className={cn(
              "inline after:content-[':'] mr-1",
              fact.tone === "caution" ? "text-caution" : "text-wd-secondary",
            )}
          >
            {fact.label}
          </dt>
          <dd className="inline text-wd-primary">{fact.value}</dd>
        </div>
      ))}
    </dl>
  );
}

/** The card's action unit: the way out leading, the effect trailing. */
export function HomeCardActions({
  className,
  hint,
  lead,
  trail,
}: {
  className?: string;
  hint?: ReactNode;
  lead?: ReactNode;
  trail?: ReactNode;
}) {
  return (
    <div className={cn("mt-1 flex flex-col gap-2", className)}>
      <div className="flex flex-wrap items-center justify-end gap-2">
        {lead ? (
          <div className="mr-auto flex flex-wrap items-center gap-1 [&>[data-quiet]:first-child]:-ml-3">
            {lead}
          </div>
        ) : null}
        {trail}
      </div>
      {hint ? <p className="wd-caption text-wd-tertiary">{hint}</p> : null}
    </div>
  );
}
