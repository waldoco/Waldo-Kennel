import { ExternalLink, Network } from "lucide-react";
import { useMemo } from "react";
import { useTranslation } from "react-i18next";

import type { useOutcomeMission } from "../../hooks/useOutcome";
import { cn } from "../../lib/utils";
import { Badge } from "../ui/badge";
import { Button } from "../ui/button";

export type MissionSessionHubProps = {
	missionQuery: ReturnType<typeof useOutcomeMission>;
	onDrillDown: (workUnitId: string) => void;
	onOpenSession: (sessionId: string) => void;
};

type SessionCard = {
	workUnitId: string;
	title: string;
	sessionId: string;
	harness: string;
	activity: string;
	attention: boolean;
};

export function MissionSessionHub({ missionQuery, onDrillDown, onOpenSession }: MissionSessionHubProps) {
	const { t } = useTranslation();
	const cards = useMemo<SessionCard[]>(
		() =>
			(missionQuery.mission?.nodes ?? [])
				.flatMap((node) => {
					const session = node.currentAttempt?.status === "running" ? node.currentAttempt.session : undefined;
					if (!session?.sessionId) return [];
					return [{
						workUnitId: node.workUnitId,
						title: node.title,
						sessionId: session.sessionId,
						harness: session.harness,
						activity: node.attention?.summary ?? node.scheduleState,
						attention: Boolean(node.attention),
					}];
				})
				.sort((left, right) => Number(right.attention) - Number(left.attention) || left.title.localeCompare(right.title)),
		[missionQuery.mission],
	);
	const harnessRollup = useMemo(() => {
		const counts = new Map<string, number>();
		for (const card of cards) counts.set(card.harness, (counts.get(card.harness) ?? 0) + 1);
		return [...counts].sort(([left], [right]) => left.localeCompare(right));
	}, [cards]);

	if (missionQuery.isLoading && !missionQuery.mission) {
		return <p className="text-sm text-muted-foreground">{t("mission.list.loading")}</p>;
	}

	return (
		<section className="flex min-h-0 flex-1 flex-col gap-3" data-testid="mission-session-hub">
		<header className="flex flex-wrap items-center justify-between gap-2">
			<div>
				<h3 className="text-sm font-medium text-foreground">{t("mission.sessionHub.heading")}</h3>
				<p className="text-xs text-muted-foreground">{t("mission.sessionHub.body")}</p>
			</div>
			<div className="flex flex-wrap gap-1.5" data-testid="mission-session-harness-rollup">
				{harnessRollup.map(([harness, count]) => <Badge key={harness} variant="outline">{harness} · {count}</Badge>)}
			</div>
		</header>
		{cards.length === 0 ? (
			<div className="rounded-group hairline border-border bg-card px-4.5 py-3.5 text-sm text-muted-foreground" data-testid="mission-session-hub-empty">
				{t("mission.sessionHub.empty")}
			</div>
		) : (
			<div className="grid min-h-0 grid-cols-1 gap-3 overflow-y-auto @[820px]/mission:grid-cols-2">
				{cards.map((card) => (
					<article className={cn("rounded-card hairline bg-card p-4", card.attention ? "border-warning/50" : "border-border")} key={card.sessionId}>
						<button className="w-full text-left focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/70" onClick={() => onDrillDown(card.workUnitId)} type="button">
							<span className="flex items-center gap-2">
								<Network aria-hidden="true" className="size-icon-sm text-muted-foreground" />
								<span className="line-clamp-2 text-sm font-medium">{card.title}</span>
							</span>
							<span className="mt-2 block text-xs text-muted-foreground">{t("mission.sessionHub.contractRevision", { revision: missionQuery.mission?.contractRevisionNumber })} · {card.activity}</span>
						</button>
						<div className="mt-3 flex items-center justify-between gap-2">
							<Badge variant="outline">{card.harness}</Badge>
							<Button aria-label={t("mission.sessionHub.openAria", { title: card.title })} onClick={() => onOpenSession(card.sessionId)} size="sm" variant="ghost">
								<ExternalLink aria-hidden="true" className="size-icon-sm" />
								{t("mission.sessionHub.open")}
							</Button>
						</div>
					</article>
				))}
			</div>
		)}
	</section>
	);
}
