import { createFileRoute } from "@tanstack/react-router";
import { useState } from "react";
import { HomeShell } from "../components/home/HomeShell";
import {
  resolveHomeDayPhase,
  type HomeContextFlow,
  type HomeDayPhase,
} from "../lib/home-day-phase";
import { homeFixture } from "../lib/home-fixture";
import { usesPreviewWorkspaceData } from "../lib/preview-mode";
import { useShell } from "../lib/shell-context";

export const Route = createFileRoute("/_shell/home")({
  component: HomeRoute,
});

const previewPhases = new Set<HomeDayPhase>(["morning", "afternoon", "evening"]);
const previewContexts = new Set<HomeContextFlow>([
  "catch_up",
  "before_next",
  "plans_changed",
  "evening_review",
  "quiet_focus",
]);

function readRendererPreviewOverrides() {
  if (!usesPreviewWorkspaceData) return {};
  const query = window.location.hash.split("?", 2)[1];
  if (!query) return {};
  const params = new URLSearchParams(query);
  const phase = params.get("homePhase") as HomeDayPhase | null;
  const context = params.get("homeContext") as HomeContextFlow | null;
  return {
    dayPhase: phase && previewPhases.has(phase) ? phase : undefined,
    contextFlow: context && previewContexts.has(context) ? context : undefined,
  };
}

function HomeRoute() {
  const { daemonStatus } = useShell();
  const [openedAt] = useState(() => new Date());
  const [previewOverrides] = useState(readRendererPreviewOverrides);
  const dayPhase = previewOverrides.dayPhase ?? resolveHomeDayPhase(openedAt);
  const availability =
    usesPreviewWorkspaceData || daemonStatus.state === "ready"
      ? "ready"
      : "offline";
  return (
    <div className="relative flex min-h-0 flex-1">
      <HomeShell
        fixture={homeFixture("today", availability, {
          dayPhase,
          contextFlow: previewOverrides.contextFlow,
        })}
      />
    </div>
  );
}
