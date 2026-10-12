import { Plus } from "lucide-react";
import { useTranslation } from "react-i18next";
import { useShell } from "../lib/shell-context";
import { CreateProjectFlow } from "./CreateProjectFlow";
import { TopbarButton } from "./TopbarButton";
import { WelcomePanel } from "./WelcomePanel";

// Board empty states: first-launch welcome (`BoardWelcome`) and project board
// with no worker sessions yet (`ProjectBoardEmpty`).
export function BoardWelcome() {
	const { createProject, initializeProjectRepository } = useShell();
	return (
		<WelcomePanel>
			<div
				className="flex h-full min-h-0 items-center justify-center overflow-y-auto px-6 py-8"
				data-testid="board-welcome"
			>
				<CreateProjectFlow
					embedded
					mode="choose"
					onCreateProject={createProject}
					onInitializeProject={initializeProjectRepository}
				/>
			</div>
		</WelcomePanel>
	);
}

// Project board with a registered project but no worker sessions yet: a clear
// starting point instead of four empty Kanban columns.
export function ProjectBoardEmpty({
	isProjectRestarting,
	isInstallingTmux,
	isSpawning,
	onNewTask,
	onInstallTmux,
	isExploring = false,
	tmuxInstallMessage,
	spawnError,
}: {
	isProjectRestarting: boolean;
	isInstallingTmux?: boolean;
	isSpawning: boolean;
	onNewTask: () => void;
	onInstallTmux?: () => void;
	isExploring?: boolean;
	tmuxInstallMessage?: string | null;
	spawnError?: string | null;
}) {
	const { t } = useTranslation();
	const agentLabel = t("shell.orchestrator");
	return (
		<div className="flex h-full min-h-0 items-center justify-center overflow-y-auto px-6">
			{/* EmptyState: one true line, then at most one next step. No card
			    chrome, glow or spinner - while the orchestrator explores, the
			    working state is a specific sentence. */}
			<div className="flex w-full max-w-preview-content flex-col items-start gap-2 py-10">
				{isExploring ? (
					<>
						<h2 className="wd-display-4 text-wd-primary">{t("board.exploring.title")}</h2>
						<p className="max-w-lg text-sm leading-5 text-wd-secondary" role="status">
							{t("board.exploring.body", { agent: agentLabel })}
						</p>
					</>
				) : (
					<>
						<h2 className="wd-display-4 text-wd-primary">{t("board.empty.title")}</h2>
						<p className="max-w-lg text-sm leading-5 text-wd-secondary">{t("board.empty.body")}</p>
					</>
				)}
				<div className="mt-3 flex items-center gap-2">
					<TopbarButton
						aria-label={t("shell.newTask")}
						className="outcome-primary-action"
						disabled={isProjectRestarting}
						onClick={onNewTask}
						variant="primary"
					>
						<Plus className="size-icon-md" aria-hidden="true" />
						{t("shell.newTask")}
					</TopbarButton>
				</div>
				{spawnError && (
					<div className="mt-3 flex flex-col items-start gap-2">
						<p className="text-sm leading-5 text-critical" role="status">
							{spawnError}
						</p>
						{onInstallTmux ? (
							<TopbarButton disabled={isSpawning || isInstallingTmux || isProjectRestarting} onClick={onInstallTmux}>
								{isInstallingTmux ? "Installing tmux…" : "Install tmux via Homebrew"}
							</TopbarButton>
						) : null}
						{tmuxInstallMessage ? (
							<p className="text-sm leading-5 text-wd-secondary" role="status">
								{tmuxInstallMessage}
							</p>
						) : null}
					</div>
				)}
			</div>
		</div>
	);
}
