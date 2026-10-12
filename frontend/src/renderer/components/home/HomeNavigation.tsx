import type { HomeDestination } from "../../lib/home-fixture";
import { Link } from "@tanstack/react-router";
import { useTranslation } from "react-i18next";
import { cn } from "../../lib/utils";

export function HomeNavigation({
  destination,
  variant = "panel",
}: {
  destination: HomeDestination;
  variant?: "panel" | "sidebar";
}) {
  const { t } = useTranslation();
  const primaryDestinations: Array<{ destination: HomeDestination; label: string; to: string }> = [
    { destination: "today", label: t("home.visual.navigation.today"), to: "/home" },
    { destination: "chat", label: t("home.visual.navigation.chat"), to: "/home/chat" },
    { destination: "open_loops", label: t("home.visual.openLoops.title"), to: "/home/open-loops" },
  ];
  const continuityDestinations: Array<{ destination: HomeDestination; label: string; to: string }> = [
    { destination: "daily_close", label: t("home.visual.dailyClose.title"), to: "/home/daily-close" },
    { destination: "memory", label: t("home.visual.memory.title"), to: "/home/memory" },
    { destination: "history", label: t("home.visual.history.title"), to: "/home/history" },
  ];
  const destinationLink = (
    item: { destination: HomeDestination; label: string; to: string },
    supporting = false,
  ) => (
    <Link
      aria-current={item.destination === destination ? "page" : undefined}
      className={cn(
        // Pills; the selected destination is ink, never orange.
        "wd-label flex h-9 shrink-0 items-center rounded-full px-4 transition-colors duration-fast motion-reduce:transition-none",
        variant === "sidebar" && "w-full",
        item.destination === destination
          ? "bg-wd-inverse text-wd-on-inverse"
          : cn(
              supporting ? "text-wd-secondary" : "text-wd-primary",
              "hover:bg-wd-hover hover:text-wd-primary active:bg-wd-pressed",
            ),
      )}
      key={item.destination}
      to={item.to}
    >
      {item.label}
    </Link>
  );
  return (
    <nav
      aria-label={t("home.visual.navigation.label")}
      className={cn(variant === "sidebar" ? "flex flex-col gap-1 px-2" : "-mx-1 flex gap-1 overflow-x-auto px-1 pb-1")}
    >
      {variant === "sidebar" ? (
        <>
          <div
            aria-label={t("home.visual.navigation.primary")}
            className="flex flex-col gap-1"
            role="group"
          >
            {primaryDestinations.map((item) => destinationLink(item))}
          </div>
          <div
            aria-label={t("home.visual.navigation.continuity")}
            className="sidebar-expanded-chrome mt-6 flex flex-col gap-1 group-data-[collapsible=icon]:hidden"
            role="group"
          >
            <p className="wd-caption px-4 pb-1 text-wd-tertiary">
              {t("home.visual.navigation.continuity")}
            </p>
            {continuityDestinations.map((item) => destinationLink(item, true))}
          </div>
        </>
      ) : (
        primaryDestinations.map((item) => destinationLink(item))
      )}
    </nav>
  );
}
