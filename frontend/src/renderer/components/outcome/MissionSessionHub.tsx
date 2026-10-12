import { ExternalLink, Network } from "lucide-react";
import { useMemo, useRef } from "react";
import { useTranslation } from "react-i18next";

import type { useOutcomeMission } from "../../hooks/useOutcome";
import { useEventsConnection } from "../../hooks/useEventsConnection";
import { StateMark } from "@pin4sf/kennel-product-ui";
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
	const connection = useEventsConnection();
	const lastConfirmedRef = useRef(missionQuery.mission);
	if (missionQuery.mission) lastConfirmedRef.current = missionQuery.mission;
	const displayMission = missionQuery.mission ?? lastConfirmedRef.current;
	const frozen = connection === "disconnected" && Boolean(displayMission);
	const refreshFailed = !frozen && Boolean(missionQuery.failure && displayMission);
	const cards = useMemo<SessionCard[]>(
		() =>
			(displayMission?.nodes ?? [])
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
		[displayMission],
	);
	const harnessRollup = useMemo(() => {
		const counts = new Map<string, number>();
		for (const card of cards) counts.set(card.harness, (counts.get(card.harness) ?? 0) + 1);
		return [...counts].sort(([left], [right]) => left.localeCompare(right));
	}, [cards]);

	if (missionQuery.isLoading && !displayMission) {
		return <p className="text-sm text-wd-secondary">{t("mission.list.loading")}</p>;
	}
	if (missionQuery.failure && !displayMission) {
		return (
			<div className="rounded-lg hairline border-wd-hairline bg-wd-surface p-4" data-testid="mission-session-hub-error">
				<h3 className="text-sm font-medium text-wd-primary">{t("mission.list.error.title")}</h3>
				<p className="mt-1 text-sm text-wd-secondary">{missionQuery.failure.message}</p>
				<Button className="mt-3" data-testid="mission-session-hub-retry" onClick={missionQuery.refetch} size="sm" variant="secondary">{t("mission.list.retry")}</Button>
			</div>
		);
	}

	return (
		<section className="flex min-h-0 flex-1 flex-col gap-3" data-testid="mission-session-hub">
		{frozen ? (
			<div className="flex items-center justify-between gap-3 rounded-md bg-wd-inset px-3 py-2" data-testid="mission-session-hub-stale">
				<p className="text-xs text-wd-primary">{t("mission.list.stale.banner", { time: displayMission?.updatedAt })}</p>
				<Button data-testid="mission-session-hub-stale-refresh" onClick={missionQuery.refetch} size="sm" variant="secondary">{t("mission.list.stale.refresh")}</Button>
			</div>
		) : null}
		{refreshFailed ? (
			<div className="flex items-center justify-between gap-3 rounded-md bg-wd-inset px-3 py-2" data-testid="mission-session-hub-refresh-failed">
				<p className="text-xs text-wd-primary">{t("mission.list.refreshFailed.banner", { message: missionQuery.failure?.message ?? "" })}</p>
				<Button data-testid="mission-session-hub-refresh-retry" onClick={missionQuery.refetch} size="sm" variant="secondary">{t("mission.list.retry")}</Button>
			</div>
		) : null}
		<header className="flex flex-wrap items-center justify-between gap-2">
			<div>
				<h3 className="text-sm font-medium text-wd-primary">{t("mission.sessionHub.heading")}</h3>
				<p className="text-xs text-wd-secondary">{t("mission.sessionHub.body")}</p>
			</div>
			<div className="flex flex-wrap gap-1.5" data-testid="mission-session-harness-rollup">
				{harnessRollup.map(([harness, count]) => <Badge key={harness} variant="neutral">{harness} · <span className="tabular-nums">{count}</span></Badge>)}
			</div>
		</header>
		{cards.length === 0 ? (
			<div className="rounded-lg hairline border-wd-hairline bg-wd-surface p-4 text-sm text-wd-secondary" data-testid="mission-session-hub-empty">
				{t("mission.sessionHub.empty")}
			</div>
		) : (
			<div className="grid min-h-0 grid-cols-1 gap-3 overflow-y-auto @[820px]/mission:grid-cols-2">
				{cards.map((card) => (
					<article className="flex flex-col gap-3 rounded-lg hairline border-wd-hairline bg-wd-surface p-4" data-attention={card.attention || undefined} key={card.sessionId}>
						<button className="w-full rounded-sm text-left focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-(--border-focus)" onClick={() => onDrillDown(card.workUnitId)} type="button">
							<span className="flex items-center gap-2">
								<Network aria-hidden="true" className="size-icon-sm shrink-0 text-wd-secondary" />
								<span className="line-clamp-2 text-sm font-medium text-wd-primary">{card.title}</span>
							</span>
							{/* A card whose unit needs the owner leads its line with the
							    Needs-you mark, so the colour is never the only signal. */}
							<span className="mt-2 flex min-w-0 items-center gap-1.5 text-xs text-wd-secondary">
								{card.attention ? <StateMark tone="needsYou" /> : null}
								<span className="min-w-0">
									<span className="tabular-nums">{t("mission.sessionHub.contractRevision", { revision: displayMission?.contractRevisionNumber })}</span> · {card.activity}
								</span>
							</span>
						</button>
						<div className="flex items-center justify-between gap-2">
							<Badge variant="neutral">{card.harness}</Badge>
							<Button aria-label={t("mission.sessionHub.openAria", { title: card.title })} disabled={frozen} onClick={() => onOpenSession(card.sessionId)} size="sm" variant="ghost">
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
