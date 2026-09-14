import { agentLabel } from "@pin4sf/kennel-product-ui";
import { Plus, X } from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";

import type { AttemptRecord } from "../../hooks/useOutcome";
import {
	useCloseShellTerminal,
	useOpenShellTerminal,
	useRenameShellTerminal,
	useShellTerminals,
	type ShellTerminal,
} from "../../hooks/useShellTerminals";
import { useWorkspaceQuery } from "../../hooks/useWorkspaceQuery";
import { TERMINAL_FONT_SIZE_DEFAULT } from "../../lib/design-tokens";
import { formatTimeCompact } from "../../lib/format-time";
import { useShellMaybe } from "../../lib/shell-context";
import { cn } from "../../lib/utils";
import { useUiStore } from "../../stores/ui-store";
import type { TerminalTarget } from "../../types/terminal";
import { Badge } from "../ui/badge";
import { ShellTerminalTab } from "../ShellTerminalTab";
import { TerminalPane } from "../TerminalPane";
import { SessionChatSurface } from "../chat/SessionChatSurface";
import { newestAttemptSession } from "./OutcomeRunBoardAdapters";
import { ENDED_ATTEMPT_PHASES } from "./attemptPhases";

type OutcomeAttemptTerminalPanelProps = {
	attempt: AttemptRecord;
	onClose: () => void;
};

/**
 * Right-hand drill-in panel for one attempt, opened from its Board/List card
 * (Figma nodes 3144:21609 / 22189 / 25210 board-side, 3144:26623 / 29541
 * list-side). It reuses the exact terminal stack the session view already
 * ships — `TerminalPane` for a live PTY, `ShellTerminalTab` +
 * useShellTerminals for auxiliary terminals — against the REAL
 * WorkspaceSession the daemon bound to this attempt (`attempt.sessions[]`).
 *
 * When no such session can be resolved (preview data, or the attempt has not
 * bound a provider session yet), the panel renders the real attempt facts it
 * does have instead of a fabricated transcript.
 */
export function OutcomeAttemptTerminalPanel({ attempt, onClose }: OutcomeAttemptTerminalPanelProps) {
	const { t } = useTranslation();
	const theme = useUiStore((state) => state.resolvedTheme);
	const shell = useShellMaybe();
	const daemonReady = shell ? shell.daemonStatus.state === "ready" : false;
	const workspaceQuery = useWorkspaceQuery();
	const shellTerminalsQuery = useShellTerminals();
	const openShellTerminal = useOpenShellTerminal();
	const closeShellTerminal = useCloseShellTerminal();
	const renameShellTerminal = useRenameShellTerminal();

	// One tab per distinct session the daemon has ever bound this attempt to
	// (almost always exactly one — the array only grows on a real rebind).
	// Ordered oldest to newest, deduplicated by sessionId with the latest
	// harness/binding for that session winning.
	const sessionBindings = useMemo(() => {
		const byId = new Map<string, { sessionId: string; harness: string; boundAt: string }>();
		for (const ref of attempt.sessions) {
			byId.set(ref.sessionId, { sessionId: ref.sessionId, harness: ref.harness, boundAt: ref.boundAt });
		}
		return [...byId.values()];
	}, [attempt.sessions]);
	const latestBinding = newestAttemptSession(attempt);

	const [selectedSessionId, setSelectedSessionId] = useState<string | undefined>(latestBinding?.sessionId);
	const [selectedTarget, setSelectedTarget] = useState<TerminalTarget>({ kind: "worker" });

	// Follow the attempt to its newest binding if the panel opens (or the
	// attempt rebinds) before the user has picked a tab of their own.
	useEffect(() => {
		setSelectedSessionId((current) =>
			current && sessionBindings.some((binding) => binding.sessionId === current) ? current : latestBinding?.sessionId,
		);
	}, [latestBinding?.sessionId, sessionBindings]);

	const workspaceSessions = useMemo(
		() => (workspaceQuery.data ?? []).flatMap((workspace) => workspace.sessions),
		[workspaceQuery.data],
	);
	const resolvedSession = workspaceSessions.find((candidate) => candidate.id === selectedSessionId);

	const shellTerminalsForSession: ShellTerminal[] = useMemo(
		() => (shellTerminalsQuery.data ?? []).filter((terminal) => terminal.sessionId === resolvedSession?.id),
		[shellTerminalsQuery.data, resolvedSession?.id],
	);

	// Switching which bound session is in view always lands back on that
	// session's own worker terminal — a shell tab from a previous session is
	// never a valid target for this one.
	function selectSession(sessionId: string) {
		setSelectedSessionId(sessionId);
		setSelectedTarget({ kind: "worker" });
	}

	function selectShell(handleId: string) {
		const target = shellTerminalsForSession.find((terminal) => terminal.handleId === handleId);
		if (!target || !resolvedSession) return;
		setSelectedTarget({
			generation: target.createdAt,
			kind: "shell",
			handleId: target.handleId,
			sessionId: resolvedSession.id,
			title: target.title,
		});
	}

	function addShellTerminal() {
		if (!resolvedSession) return;
		openShellTerminal.mutate(
			{ projectId: resolvedSession.workspaceId, sessionId: resolvedSession.id },
			{
				onSuccess: (created) => {
					setSelectedTarget({
						generation: created.createdAt,
						kind: "shell",
						handleId: created.handleId,
						sessionId: resolvedSession.id,
						title: created.title,
					});
				},
			},
		);
	}

	function closeShell(handleId: string) {
		if (selectedTarget.kind === "shell" && selectedTarget.handleId === handleId) {
			const closingIndex = shellTerminalsForSession.findIndex((terminal) => terminal.handleId === handleId);
			const next = shellTerminalsForSession[closingIndex - 1] ?? shellTerminalsForSession[closingIndex + 1];
			setSelectedTarget(
				next && resolvedSession
					? { generation: next.createdAt, kind: "shell", handleId: next.handleId, sessionId: resolvedSession.id, title: next.title }
					: { kind: "worker" },
			);
		}
		closeShellTerminal.mutate(handleId);
	}

	return (
		<aside
			aria-label={t("outcome.run.panelHeading")}
			className="flex w-[26rem] shrink-0 flex-col gap-1.5 rounded-panel hairline border-border bg-shell p-1.25"
			data-testid="outcome-run-attempt-panel"
		>
			<div className="flex shrink-0 items-center justify-between gap-2 px-0.75 py-0.75">
				<div
					aria-label={t("outcome.run.panelSessionTabsAria")}
					className="scrollbar-none flex min-w-0 flex-1 items-center gap-1 overflow-x-auto"
					role="tablist"
				>
					{sessionBindings.map((binding) => (
						<button
							aria-selected={binding.sessionId === selectedSessionId}
							className={cn(
								"inline-flex shrink-0 items-center gap-1.5 rounded-md px-2.5 py-1.5 text-xs transition-colors",
								binding.sessionId === selectedSessionId
									? "hairline border-border bg-popover font-medium text-foreground"
									: "text-passive hover:bg-interactive-hover/60 hover:text-foreground",
							)}
							key={binding.sessionId}
							onClick={() => selectSession(binding.sessionId)}
							role="tab"
							type="button"
						>
							{t("outcome.run.panelSessionTab", { agent: agentLabel(binding.harness) })}
						</button>
					))}
					{resolvedSession && resolvedSession.mode !== "chat" && shellTerminalsForSession.length > 0 ? (
						<>
							<span aria-hidden="true" className="h-4 w-px shrink-0 bg-border" />
							{shellTerminalsForSession.map((terminal) => (
								<ShellTerminalTab
									isActive={selectedTarget.kind === "shell" && selectedTarget.handleId === terminal.handleId}
									key={terminal.handleId}
									onClose={() => closeShell(terminal.handleId)}
									onRename={(title) => renameShellTerminal.mutate({ handleId: terminal.handleId, title })}
									onSelect={() => selectShell(terminal.handleId)}
									shell={terminal}
								/>
							))}
						</>
					) : null}
				</div>
				<div className="flex shrink-0 items-center gap-0.5">
					{resolvedSession && resolvedSession.mode !== "chat" ? (
						<button
							aria-label={t("outcome.run.panelNewTerminal")}
							className="inline-flex size-control-sm items-center justify-center rounded-md text-passive transition-colors hover:bg-interactive-hover hover:text-foreground"
							onClick={addShellTerminal}
							title={t("outcome.run.panelNewTerminal")}
							type="button"
						>
							<Plus aria-hidden="true" className="size-icon-sm" />
						</button>
					) : null}
					<button
						aria-label={t("common.close")}
						className="inline-flex size-control-sm items-center justify-center rounded-md text-passive transition-colors hover:bg-interactive-hover hover:text-foreground"
						data-testid="outcome-run-attempt-panel-close"
						onClick={onClose}
						title={t("common.close")}
						type="button"
					>
						<X aria-hidden="true" className="size-icon-sm" />
					</button>
				</div>
			</div>

			{attempt.presentation && ENDED_ATTEMPT_PHASES.has(attempt.presentation.phase) ? <EndedAttemptFacts attempt={attempt} /> : null}
			<div className="min-h-0 flex-1 overflow-hidden rounded-group hairline border-border bg-card">
				{resolvedSession?.mode === "chat" ? (
					<SessionChatSurface
						daemonReady={daemonReady}
						onOpenShell={addShellTerminal}
						openingShell={openShellTerminal.isPending}
						onSelectChat={() => setSelectedTarget({ kind: "worker" })}
						onSelectShellTerminal={selectShell}
						session={resolvedSession}
						shellError={openShellTerminal.error ? String(openShellTerminal.error) : undefined}
						shellTarget={selectedTarget.kind === "shell" ? selectedTarget : undefined}
						shellTerminals={shellTerminalsForSession}
						onCloseShellTerminal={closeShell}
						onRenameShellTerminal={(handleId, title) => renameShellTerminal.mutate({ handleId, title })}
						theme={theme}
					/>
				) : resolvedSession ? (
					<TerminalPane
						daemonReady={daemonReady}
						fontSize={TERMINAL_FONT_SIZE_DEFAULT}
						session={resolvedSession}
						terminalTarget={selectedTarget}
						theme={theme}
					/>
				) : (
					<AttemptPanelFallback attempt={attempt} latestBinding={latestBinding} />
				)}
			</div>
		</aside>
	);
}


/** Daemon-recorded recovery facts for an ended Attempt, shown verbatim. */
function EndedAttemptFacts({ attempt }: { attempt: AttemptRecord }) {
	const { t } = useTranslation();
	return (
		<div className="flex flex-col gap-1.5 border-b border-border px-3 py-2 text-2xs" data-testid="attempt-ended-facts">
			{attempt.presentation.endedUnclassified ? (
				<p className="text-warning">{t("outcome.run.panelEndedUnclassified")}</p>
			) : null}
			{attempt.receipts.length > 0 ? (
				<div>
					<h4 className="font-medium uppercase tracking-wide text-passive">{t("outcome.run.panelReceipts")}</h4>
					<ul className="mt-0.5 space-y-0.5">
						{attempt.receipts.map((receipt) => (
							<li className="flex items-center gap-1.5" key={receipt.id}>
								<Badge variant={receipt.resolution === "needs_attention" ? "warning" : "neutral"}>
									{t(`outcome.run.panelReceipt.${receipt.resolution}`)}
								</Badge>
								<time dateTime={receipt.createdAt}>{formatTimeCompact(receipt.createdAt)}</time>
							</li>
						))}
					</ul>
				</div>
			) : null}
			<p className="text-passive">{t("outcome.run.observationCount", { total: attempt.observations.length })}</p>
		</div>
	);
}

function AttemptPanelFallback({
	attempt,
	latestBinding,
}: {
	attempt: AttemptRecord;
	latestBinding?: { sessionId: string; harness: string; boundAt: string };
}) {
	const { t } = useTranslation();
	return (
		<div className="flex h-full flex-col gap-3 overflow-y-auto p-4 font-mono text-xs leading-relaxed text-terminal-dim">
			<p className="text-terminal">
				{latestBinding ? t("outcome.run.panelFallbackUnavailableTitle") : t("outcome.run.panelFallbackTitle")}
			</p>
			<p>{latestBinding ? t("outcome.run.panelFallbackUnavailableBody") : t("outcome.run.panelFallbackBody")}</p>
			<dl className="flex flex-col gap-1.5 text-terminal-foreground">
				<div className="flex gap-2">
					<dt className="text-terminal-dim">{t("outcome.run.panelFallbackNextAction")}</dt>
					<dd>{attempt.presentation.nextAction}</dd>
				</div>
				{latestBinding ? (
					<>
						<div className="flex gap-2">
							<dt className="text-terminal-dim">{t("outcome.run.panelFallbackAgent")}</dt>
							<dd>{agentLabel(latestBinding.harness)}</dd>
						</div>
						<div className="flex gap-2">
							<dt className="text-terminal-dim">{t("outcome.run.panelFallbackBoundAt")}</dt>
							<dd>{formatTimeCompact(latestBinding.boundAt)}</dd>
						</div>
					</>
				) : (
					<div className="flex gap-2">
						<dt className="text-terminal-dim">{t("outcome.run.panelFallbackAgent")}</dt>
						<dd>{t("outcome.run.panelFallbackNoSession")}</dd>
					</div>
				)}
			</dl>
		</div>
	);
}
