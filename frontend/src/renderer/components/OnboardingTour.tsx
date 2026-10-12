import { useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import * as Dialog from "@radix-ui/react-dialog";
import {
	ArrowLeft,
	ArrowRight,
	Check,
	CheckCircle2,
	Circle,
	FileCheck2,
	FolderPlus,
	GitBranch,
	RefreshCw,
	ShieldCheck,
	X,
	XCircle,
} from "lucide-react";
import { useTranslation } from "react-i18next";
import { cn } from "../lib/utils";
import { useAgentsQuery } from "../hooks/useAgentsQuery";
import { useSettings, useUpdateReasoning } from "../hooks/useSettings";
import { aoBridge } from "../lib/bridge";
import { useUiStore } from "../stores/ui-store";
import { AgentAvatar } from "./AgentAvatar";
import { Button } from "./ui/button";

/**
 * First-run path for the Outcome product. It only claims readiness returned by
 * a real daemon or native bridge. In particular, tmux has no read endpoint yet,
 * so this surface calls that out instead of manufacturing a green check.
 */
const STEPS = ["welcome", "system", "codex", "journey", "project"] as const;
type Step = (typeof STEPS)[number];

const TITLES: Record<Step, string> = {
	welcome: "Welcome",
	system: "System check",
	codex: "Connect Codex",
	journey: "How work moves",
	project: "Your first Project",
};

export function OnboardingTour({ daemonReady }: { daemonReady: boolean }) {
	const isOpen = useUiStore((state) => state.isOnboardingOpen);
	const hasCompleted = useUiStore((state) => state.hasCompletedOnboarding);
	const openOnboarding = useUiStore((state) => state.openOnboarding);
	const closeOnboarding = useUiStore((state) => state.closeOnboarding);
	const [stepIndex, setStepIndex] = useState(0);
	const contentRef = useRef<HTMLDivElement>(null);
	const { t } = useTranslation();

	useEffect(() => {
		if (!hasCompleted && daemonReady) openOnboarding();
	}, [daemonReady, hasCompleted, openOnboarding]);
	useEffect(() => {
		if (isOpen) setStepIndex(0);
	}, [isOpen]);
	useEffect(() => {
		if (!isOpen) return;
		contentRef.current
			?.querySelector<HTMLElement>("[data-onboarding-step-heading]")
			?.focus();
	}, [isOpen, stepIndex]);

	const step = STEPS[stepIndex];
	const isLast = stepIndex === STEPS.length - 1;
	return (
		<Dialog.Root
			open={isOpen}
			onOpenChange={(next) => !next && closeOnboarding()}
		>
			<Dialog.Portal>
				<Dialog.Overlay className="dialog-overlay data-[state=open]:animate-overlay-in" />
				<Dialog.Content
					aria-describedby={undefined}
					ref={contentRef}
					onOpenAutoFocus={(event) => {
						event.preventDefault();
						contentRef.current
							?.querySelector<HTMLElement>("[data-onboarding-step-heading]")
							?.focus();
					}}
					className="fixed left-1/2 top-1/2 z-overlay flex w-[min(720px,calc(100vw-32px))] -translate-x-1/2 -translate-y-1/2 flex-col overflow-hidden rounded-xl border-[length:var(--stroke-hairline)] border-wd-hairline bg-wd-raised shadow-[var(--shadow-overlay)] outline-none data-[state=open]:animate-modal-in"
					data-testid="onboarding-tour"
				>
					<header className="flex shrink-0 flex-col gap-4 px-8 pb-2 pt-6">
						<div className="flex items-center justify-between gap-3">
							<Dialog.Title className="flex min-w-0 items-baseline gap-1.5">
								<span className="wd-label truncate text-wd-primary">{TITLES[step]}</span>
								<span className="wd-label shrink-0 text-wd-tertiary">
									{t("onboarding.stepCount", { current: stepIndex + 1, total: STEPS.length })}
								</span>
							</Dialog.Title>
							<Dialog.Close
								aria-label={t("onboarding.close")}
								className="settings-close-button"
							>
								<X aria-hidden="true" className="size-icon-md" />
							</Dialog.Close>
						</div>
						<StepPager current={stepIndex} total={STEPS.length} />
					</header>
					<p
						aria-live="polite"
						className="sr-only"
					>{`Step ${stepIndex + 1} of ${STEPS.length}: ${TITLES[step]}`}</p>
					<div className="board-scrollbar h-[440px] shrink-0 overflow-y-auto px-8 py-6">
						{step === "welcome" ? <WelcomeStep /> : null}
						{step === "system" ? <SystemStep /> : null}
						{step === "codex" ? <CodexStep /> : null}
						{step === "journey" ? <JourneyStep /> : null}
						{step === "project" ? <ProjectStep /> : null}
					</div>
					<footer className="flex shrink-0 items-center justify-between gap-3 px-8 pb-6 pt-4">
						<Button
							className="gap-1.5"
							disabled={stepIndex === 0}
							onClick={() => setStepIndex((n) => Math.max(0, n - 1))}
							variant="ghost"
						>
							<ArrowLeft aria-hidden="true" className="size-icon-sm" /> {t("onboarding.back")}
						</Button>
						<div className="flex items-center gap-2">
							{isLast ? null : (
								<Button onClick={closeOnboarding} variant="ghost">
									{t("onboarding.skip")}
								</Button>
							)}
							{/* The last step's one primary is its own "Choose a Project
							    folder"; finishing there is the quiet way out. */}
							<Button
								className="gap-1.5"
								onClick={() =>
									isLast ? closeOnboarding() : setStepIndex((n) => n + 1)
								}
								variant={isLast ? "secondary" : "primary"}
							>
								{stepIndex === 0
									? "Set up Kennel"
									: isLast
										? "Finish"
										: "Continue"}
								{isLast ? null : (
									<ArrowRight aria-hidden="true" className="size-icon-sm" />
								)}
							</Button>
						</div>
					</footer>
				</Dialog.Content>
			</Dialog.Portal>
		</Dialog.Root>
	);
}

function StepPager({ current, total }: { current: number; total: number }) {
	const { t } = useTranslation();
	return (
		<ol
			aria-label={t("onboarding.progressAria", { current: current + 1, total })}
			aria-valuemax={total}
			aria-valuemin={1}
			aria-valuenow={current + 1}
			className="grid gap-2"
			role="progressbar"
			style={{ gridTemplateColumns: `repeat(${total}, minmax(0, 1fr))` }}
		>
			{STEPS.slice(0, total).map((name, i) => (
				<li aria-hidden="true" className="flex min-w-0 flex-col gap-1.5" key={name}>
					<span
						className={cn(
							"h-1 rounded-full transition-colors duration-base ease-out",
							i <= current ? "bg-wd-inverse" : "bg-wd-inset",
						)}
					/>
					<span
						className={cn(
							"wd-caption truncate",
							i === current ? "text-wd-primary" : "text-wd-tertiary",
						)}
					>
						{TITLES[name]}
					</span>
				</li>
			))}
		</ol>
	);
}

function Heading({
	body,
	title,
}: {
	body: string;
	icon?: ReactNode;
	title: string;
}) {
	return (
		<div className="flex flex-col gap-3">
			<h2
				className="wd-display-2 font-display font-normal text-wd-primary outline-none"
				data-onboarding-step-heading
				tabIndex={-1}
			>
				{title}
			</h2>
			<p className="wd-body-l max-w-[58ch] text-wd-secondary">
				{body}
			</p>
		</div>
	);
}

function WelcomeStep() {
	const { t } = useTranslation();
	return (
		<div className="flex flex-col gap-6">
			<Heading
				title={t("onboarding.tour.welcome.heading")}
				body={t("onboarding.tour.welcome.body")}
			/>
			<div className="settings-grouped-rows">
				<WelcomeCard
					icon={<ShieldCheck />}
					title={t("onboarding.tour.welcome.governedTitle")}
					body="Authority stays inside the Contract you approve."
				/>
				<WelcomeCard
					icon={<GitBranch />}
					title={t("onboarding.tour.welcome.traceableTitle")}
					body="Every WorkUnit keeps its plan and evidence lineage."
				/>
				<WelcomeCard
					icon={<FileCheck2 />}
					title={t("onboarding.tour.welcome.acceptTitle")}
					body="Agent completion never replaces your decision."
				/>
			</div>
			<p className="wd-caption text-wd-tertiary">
				{t("onboarding.tour.welcome.timeHint")}
			</p>
		</div>
	);
}
function WelcomeCard({
	body,
	icon,
	title,
}: {
	body: string;
	icon: ReactNode;
	title: string;
}) {
	return (
		<div className="settings-row-bar justify-start">
			<span
				aria-hidden="true"
				className="settings-row-icon grid size-7 shrink-0 place-items-center rounded-sm bg-wd-inset text-wd-secondary [&_svg]:size-4"
			>
				{icon}
			</span>
			<div className="flex min-w-0 flex-col gap-0.5">
				<p className="wd-body text-wd-primary">{title}</p>
				<p className="wd-body-s text-wd-secondary">{body}</p>
			</div>
		</div>
	);
}

function SystemStep() {
	const { t } = useTranslation();
	const [installState, setInstallState] = useState<
		"idle" | "installing" | "installed" | "failed" | "cancelled"
	>("idle");
	const [message, setMessage] = useState<string | null>(null);
	const install = async () => {
		setInstallState("installing");
		setMessage(null);
		try {
			const result = await aoBridge.app.installTmux();
			setInstallState(result.status);
			setMessage(result.message ?? null);
		} catch (error) {
			setInstallState("failed");
			setMessage(
				error instanceof Error ? error.message : "tmux setup could not start.",
			);
		}
	};
	return (
		<div className="flex flex-col gap-5">
			<Heading
				title={t("onboarding.tour.system.title")}
				body="Kennel runs coding work on your machine. The daemon is ready. macOS and Linux sessions also need tmux; Windows uses its native terminal runtime."
			/>
			<StatusRow
				state="ready"
				title={t("onboarding.tour.system.daemonTitle")}
				body="Running and responding on this Mac."
			/>
			<StatusRow
				state={
					installState === "installed"
						? "unknown"
						: installState === "failed"
							? "error"
							: "unknown"
				}
				title={t("onboarding.tour.system.runtimeTitle")}
				body={
					installState === "installed"
						? "Homebrew finished installing tmux. Kennel will verify the session runtime when the first session starts."
						: installState === "failed"
							? (message ?? "tmux could not be installed.")
							: "Kennel does not yet expose a setup probe. It will check tmux before the first session starts."
				}
				action={
					installState !== "installed" ? (
						<Button
							disabled={installState === "installing"}
							onClick={() => void install()}
							size="sm"
							variant="secondary"
						>
							{installState === "installing"
								? t("onboarding.tour.system.installing")
								: "Install tmux with Homebrew"}
						</Button>
					) : undefined
				}
			/>
			{installState === "cancelled" ? (
				<p className="wd-body-s text-wd-secondary" role="status">
					{t("onboarding.tour.system.cancelled")}
				</p>
			) : null}
			<p className="wd-body-s rounded-md bg-wd-inset px-4 py-3 text-wd-secondary">
				{t("onboarding.tour.system.prereqNote")}
			</p>
		</div>
	);
}

function StatusRow({
	action,
	body,
	state,
	title,
}: {
	action?: ReactNode;
	body: string;
	state: "ready" | "unknown" | "error";
	title: string;
}) {
	const Icon =
		state === "ready" ? CheckCircle2 : state === "error" ? XCircle : Circle;
	return (
		<div
			className={cn(
				"rounded-lg border-[length:var(--stroke-hairline)] border-wd-hairline bg-wd-surface p-4",
				action
					? "grid grid-cols-[auto_minmax(0,1fr)] items-start gap-x-3 gap-y-3 sm:grid-cols-[auto_minmax(0,1fr)_auto] sm:items-center"
					: "flex items-center gap-3",
			)}
		>
			<span
				aria-hidden="true"
				className={cn(
					"grid size-7 shrink-0 place-items-center rounded-sm",
					state === "ready"
						? "bg-good-bg text-good"
						: state === "error"
							? "bg-critical-bg text-critical"
							: "bg-wd-inset text-wd-secondary",
				)}
			>
				<Icon className="size-4" />
			</span>
			<div className="min-w-0 flex-1">
				<p className="wd-body text-wd-primary">{title}</p>
				<p className="wd-body-s mt-0.5 text-wd-secondary">{body}</p>
			</div>
			{action ? (
				<div className="col-start-2 sm:col-start-3 sm:row-start-1">
					{action}
				</div>
			) : null}
		</div>
	);
}

function CodexStep() {
	const { t } = useTranslation();
	const agents = useAgentsQuery();
	const { settings } = useSettings();
	const { update, saving } = useUpdateReasoning();
	const setDefaultAgentId = useUiStore((state) => state.setDefaultAgentId);
	const defaultAgentId = useUiStore((state) => state.defaultAgentId);
	const [notice, setNotice] = useState<string | null>(null);
	const codex = useMemo(
		() => agents.data?.installed?.find((agent) => agent.id === "codex"),
		[agents.data],
	);
	const authorized =
		agents.data?.authorized?.some((agent) => agent.id === "codex") ?? false;
	const connect = async () => {
		setNotice(null);
		try {
			const result = await update({
				provider: "codex",
				model: settings?.reasoning.model ?? "",
				effort: settings?.reasoning.effort ?? "",
			});
			setDefaultAgentId("codex");
			setNotice(
				result?.verified === true
					? "Codex is selected for planning, and Kennel verified a model call."
					: "Codex is selected for planning; Kennel has not verified a model call yet.",
			);
		} catch {
			setNotice(
				"Codex was selected, but the setting could not be saved. Try again or open Settings.",
			);
		}
	};
	return (
		<div className="flex flex-col gap-5">
			<Heading
				title={t("onboarding.tour.codex.title")}
				body="Codex can reason about Contracts and Plans, then carry out approved WorkUnits inside the Project you choose."
			/>
			{agents.isPending ? (
				<div className="wd-body-s rounded-lg border-[length:var(--stroke-hairline)] border-wd-hairline bg-wd-surface p-4 text-wd-secondary" role="status">
					{t("onboarding.tour.codex.looking")}
				</div>
			) : agents.isError ? (
				<StatusRow
					state="error"
					title={t("onboarding.tour.codex.checkFailedTitle")}
					body="The daemon could not read the provider inventory."
					action={
						<Button
							onClick={() => void agents.refetch?.()}
							size="sm"
							variant="secondary"
						>
							<RefreshCw className="mr-1.5 size-icon-sm" />
							{t("onboarding.tour.codex.tryAgain")}
						</Button>
					}
				/>
			) : !codex ? (
				<StatusRow
					state="unknown"
					title={t("onboarding.tour.codex.notFoundTitle")}
					body="Install and sign in to Codex, then ask Kennel to check again. No provider is selected automatically."
					action={
						<Button
							onClick={() => void agents.refetch?.()}
							size="sm"
							variant="secondary"
						>
							<RefreshCw className="mr-1.5 size-icon-sm" />
							{t("onboarding.tour.codex.checkAgain")}
						</Button>
					}
				/>
			) : (
				<div className="flex items-center gap-3 rounded-lg border-[length:var(--stroke-hairline)] border-wd-hairline bg-wd-surface p-4">
					<AgentAvatar provider="codex" />
					<div className="min-w-0 flex-1">
						<p className="wd-body text-wd-primary">{codex.label || "Codex"}</p>
						<p className="wd-body-s mt-0.5 text-wd-secondary">
							{authorized
								? "Installed · sign-in detected"
								: "Installed · sign-in not confirmed"}
						</p>
					</div>
					{defaultAgentId === "codex" &&
					settings?.reasoning.provider === "codex" ? (
						<span className="wd-label flex items-center gap-1 rounded-xs bg-good-bg px-2 py-1 text-good">
							<Check className="size-icon-sm" />
							{t("onboarding.tour.codex.selected")}
						</span>
					) : (
						<Button
							disabled={saving}
							onClick={() => void connect()}
							size="sm"
							variant="secondary"
						>
							{saving ? "Connecting…" : "Use Codex"}
						</Button>
					)}
				</div>
			)}
			{notice ? (
				<p
					className="wd-body-s rounded-md bg-wd-inset px-4 py-3 text-wd-secondary"
					role="status"
				>
					{notice}
				</p>
			) : null}
			<p className="wd-caption text-wd-tertiary">
				{t("onboarding.tour.codex.authorityNote")}
			</p>
		</div>
	);
}

function JourneyStep() {
	const { t } = useTranslation();
	const stages = [
		["1", "Project", "Choose the repository or workspace that holds the work."],
		["2", "Outcome", "Describe the result you want, not a list of tasks."],
		["3", "Contract", "Review success criteria, boundaries, and authority."],
		["4", "Planning", "Discuss the approach and approve the exact WorkUnits."],
		[
			"5",
			"Mission Control",
			"Watch progress, answer Needs You, inspect proof, and accept.",
		],
	] as const;
	return (
		<div className="flex flex-col gap-5">
			<Heading
				title={t("onboarding.tour.journey.title")}
				body="Kennel keeps each decision in its own place, so setup never turns into silent execution."
			/>
			<ol className="relative flex flex-col gap-1 before:absolute before:bottom-5 before:left-[23px] before:top-5 before:w-px before:bg-(--border-hairline)">
				{stages.map(([n, title, body]) => (
					<li
						className="relative flex gap-3 rounded-md px-3 py-2.5"
						key={n}
					>
						<span className="wd-data z-10 grid size-6 shrink-0 place-items-center rounded-full bg-wd-inset text-wd-primary">
							{n}
						</span>
						<div>
							<p className="wd-body text-wd-primary">{title}</p>
							<p className="wd-body-s mt-0.5 text-wd-secondary">
								{body}
							</p>
						</div>
					</li>
				))}
			</ol>
		</div>
	);
}

function ProjectStep() {
	const { t } = useTranslation();
	const requestCreateProject = useUiStore(
		(state) => state.requestCreateProject,
	);
	const closeOnboarding = useUiStore((state) => state.closeOnboarding);
	return (
		<div className="flex flex-col items-start gap-5">
			<Heading
				title={t("onboarding.tour.project.title")}
				body="Choose one repository or a workspace of repositories. Registering it records context and provider preferences; it does not start agent work."
			/>
			<div className="w-full rounded-lg border-[length:var(--stroke-hairline)] border-wd-hairline bg-wd-surface p-4 text-left">
				<p className="wd-label text-wd-primary">{t("onboarding.tour.project.guideIntro")}</p>
				<ul className="wd-body-s mt-2 grid grid-cols-2 gap-x-4 gap-y-1.5 text-wd-secondary">
					<li>{t("onboarding.tour.project.guideReview")}</li>
					<li>{t("onboarding.tour.project.guidePairing")}</li>
					<li>{t("onboarding.tour.project.guideOutcome")}</li>
					<li>{t("onboarding.tour.project.guideApprove")}</li>
				</ul>
			</div>
			<Button
				className="gap-1.5"
				onClick={() => {
					closeOnboarding();
					requestCreateProject();
				}}
				variant="primary"
			>
				<FolderPlus className="size-icon-sm" aria-hidden="true" />
				{t("onboarding.tour.project.chooseFolder")}
			</Button>
			<p className="wd-caption text-wd-tertiary">
				{t("onboarding.tour.project.reopenHint")}
			</p>
		</div>
	);
}
