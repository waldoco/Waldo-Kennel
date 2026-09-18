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
	Loader2,
	RefreshCw,
	ShieldCheck,
	Sparkles,
	Target,
	TerminalSquare,
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
					className="fixed left-1/2 top-1/2 z-overlay flex w-[min(720px,calc(100vw-32px))] -translate-x-1/2 -translate-y-1/2 flex-col overflow-hidden rounded-panel hairline border-border bg-card shadow-[var(--shadow-import-modal)] outline-none data-[state=open]:animate-modal-in"
					data-testid="onboarding-tour"
				>
					<header className="flex shrink-0 items-center justify-between gap-3 border-b border-border px-5 py-3.5">
						<Dialog.Title className="flex min-w-0 items-baseline gap-2">
							<span className="truncate text-sm font-medium text-foreground">
								{TITLES[step]}
							</span>
							<span className="shrink-0 text-2xs text-passive">
								{t("onboarding.stepCount", { current: stepIndex + 1, total: STEPS.length })}
							</span>
						</Dialog.Title>
						<div className="flex items-center gap-3">
							<StepPager current={stepIndex} total={STEPS.length} />
							<Dialog.Close
								aria-label={t("onboarding.close")}
								className="grid size-control-chip place-items-center rounded-md text-passive transition-colors hover:bg-muted hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/60"
							>
								<X aria-hidden="true" className="size-icon-md" />
							</Dialog.Close>
						</div>
					</header>
					<p
						aria-live="polite"
						className="sr-only"
					>{`Step ${stepIndex + 1} of ${STEPS.length}: ${TITLES[step]}`}</p>
					<div className="board-scrollbar h-[390px] shrink-0 overflow-y-auto px-5 py-5.5">
						{step === "welcome" ? <WelcomeStep /> : null}
						{step === "system" ? <SystemStep /> : null}
						{step === "codex" ? <CodexStep /> : null}
						{step === "journey" ? <JourneyStep /> : null}
						{step === "project" ? <ProjectStep /> : null}
					</div>
					<footer className="flex shrink-0 items-center justify-between gap-3 border-t border-border px-5 py-3.5">
						<Button
							className="gap-1.5"
							disabled={stepIndex === 0}
							onClick={() => setStepIndex((n) => Math.max(0, n - 1))}
							size="sm"
							variant="ghost"
						>
							<ArrowLeft aria-hidden="true" className="size-icon-sm" /> {t("onboarding.back")}
						</Button>
						<Button onClick={closeOnboarding} size="sm" variant="ghost">
							{t("onboarding.skip")}
						</Button>
						<Button
							className="gap-1.5"
							onClick={() =>
								isLast ? closeOnboarding() : setStepIndex((n) => n + 1)
							}
							size="sm"
							variant="primary"
						>
							{isLast ? (
								<Check aria-hidden="true" className="size-icon-sm" />
							) : null}
							{stepIndex === 0
								? "Set up Kennel"
								: isLast
									? "Finish"
									: "Continue"}
							{isLast ? null : (
								<ArrowRight aria-hidden="true" className="size-icon-sm" />
							)}
						</Button>
					</footer>
				</Dialog.Content>
			</Dialog.Portal>
		</Dialog.Root>
	);
}

function StepPager({ current, total }: { current: number; total: number }) {
	const { t } = useTranslation();
	return (
		<div
			aria-label={t("onboarding.progressAria", { current: current + 1, total })}
			aria-valuemax={total}
			aria-valuemin={1}
			aria-valuenow={current + 1}
			className="flex items-center gap-1"
			role="progressbar"
		>
			{Array.from({ length: total }, (_, i) => (
				<span
					aria-hidden="true"
					className={cn(
						"h-1.5 rounded-full transition-[width,background-color] duration-200",
						i === current
							? "w-5 bg-foreground"
							: i < current
								? "w-1.5 bg-muted-foreground"
								: "w-1.5 bg-border-strong",
					)}
					key={i}
				/>
			))}
		</div>
	);
}

function Heading({
	body,
	icon,
	title,
}: {
	body: string;
	icon: ReactNode;
	title: string;
}) {
	return (
		<div className="flex flex-col gap-2.5">
			<h2
				className="flex items-center gap-2 text-brand font-medium leading-snug text-foreground outline-none"
				data-onboarding-step-heading
				tabIndex={-1}
			>
				{icon}
				{title}
			</h2>
			<p className="max-w-[62ch] text-xs leading-body text-foreground/60">
				{body}
			</p>
		</div>
	);
}

function WelcomeStep() {
	const { t } = useTranslation();
	return (
		<div className="flex flex-col items-center gap-6 text-center">
			<div className="grid size-14 place-items-center rounded-2xl hairline border-border bg-popover text-foreground shadow-sm">
				<Sparkles aria-hidden="true" className="size-6" />
			</div>
			<div className="flex flex-col gap-2">
				<h2
					className="text-heading-sm font-medium text-foreground outline-none"
					data-onboarding-step-heading
					tabIndex={-1}
				>
					{t("onboarding.tour.welcome.heading")}
				</h2>
				<p className="mx-auto max-w-[54ch] text-xs leading-body text-foreground/60">
					{t("onboarding.tour.welcome.body")}
				</p>
			</div>
			<div className="grid w-full grid-cols-3 gap-2 text-left">
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
			<p className="text-2xs text-passive">
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
		<div className="rounded-lg hairline border-border bg-popover/60 p-3.5">
			<span className="mb-2 block text-muted-foreground [&_svg]:size-icon-md">
				{icon}
			</span>
			<p className="text-xs font-medium">{title}</p>
			<p className="mt-1 text-2xs leading-body text-passive">{body}</p>
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
				icon={
					<TerminalSquare
						aria-hidden="true"
						className="size-icon-base text-muted-foreground"
					/>
				}
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
							variant="outline"
						>
							{installState === "installing" ? (
								<>
									<Loader2 className="mr-1.5 size-icon-sm animate-spin" />
									{t("onboarding.tour.system.installing")}
								</>
							) : (
								"Install tmux with Homebrew"
							)}
						</Button>
					) : undefined
				}
			/>
			{installState === "cancelled" ? (
				<p className="text-2xs text-passive" role="status">
					{t("onboarding.tour.system.cancelled")}
				</p>
			) : null}
			<p className="rounded-md bg-muted/45 px-3 py-2.5 text-2xs leading-body text-passive">
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
				"rounded-lg hairline border-border bg-popover/55 p-3.5",
				action
					? "grid grid-cols-[auto_minmax(0,1fr)] items-start gap-x-3 gap-y-3 sm:grid-cols-[auto_minmax(0,1fr)_auto] sm:items-center"
					: "flex items-center gap-3",
			)}
		>
			<Icon
				className={cn(
					"size-icon-base shrink-0",
					state === "ready"
						? "text-status-ready"
						: state === "error"
							? "text-error"
							: "text-passive",
				)}
			/>
			<div className="min-w-0 flex-1">
				<p className="text-xs font-medium">{title}</p>
				<p className="mt-0.5 text-2xs leading-body text-passive">{body}</p>
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
				icon={
					<Sparkles
						aria-hidden="true"
						className="size-icon-base text-muted-foreground"
					/>
				}
				title={t("onboarding.tour.codex.title")}
				body="Codex can reason about Contracts and Plans, then carry out approved WorkUnits inside the Project you choose."
			/>
			{agents.isPending ? (
				<div className="flex items-center gap-2 rounded-lg hairline border-border p-4 text-xs text-passive">
					<Loader2 className="size-icon-sm animate-spin" />
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
							variant="outline"
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
							variant="outline"
						>
							<RefreshCw className="mr-1.5 size-icon-sm" />
							{t("onboarding.tour.codex.checkAgain")}
						</Button>
					}
				/>
			) : (
				<div className="flex items-center gap-3 rounded-lg hairline border-border bg-popover/55 p-4">
					<AgentAvatar provider="codex" />
					<div className="min-w-0 flex-1">
						<p className="text-xs font-medium">{codex.label || "Codex"}</p>
						<p className="mt-0.5 text-2xs text-passive">
							{authorized
								? "Installed · sign-in detected"
								: "Installed · sign-in not confirmed"}
						</p>
					</div>
					{defaultAgentId === "codex" &&
					settings?.reasoning.provider === "codex" ? (
						<span className="flex items-center gap-1 text-2xs text-status-ready">
							<Check className="size-icon-sm" />
							{t("onboarding.tour.codex.selected")}
						</span>
					) : (
						<Button
							disabled={saving}
							onClick={() => void connect()}
							size="sm"
							variant="primary"
						>
							{saving ? "Connecting…" : "Use Codex"}
						</Button>
					)}
				</div>
			)}
			{notice ? (
				<p
					className="rounded-md bg-muted/45 px-3 py-2.5 text-2xs leading-body text-passive"
					role="status"
				>
					{notice}
				</p>
			) : null}
			<p className="text-2xs leading-body text-passive">
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
				icon={
					<GitBranch
						aria-hidden="true"
						className="size-icon-base text-muted-foreground"
					/>
				}
				title={t("onboarding.tour.journey.title")}
				body="Kennel keeps each decision in its own place, so setup never turns into silent execution."
			/>
			<ol className="relative flex flex-col gap-1 before:absolute before:bottom-5 before:left-[17px] before:top-5 before:w-px before:bg-border">
				{stages.map(([n, title, body]) => (
					<li
						className="relative flex gap-3 rounded-lg p-2.5 transition-colors hover:bg-popover/60"
						key={n}
					>
						<span className="z-10 grid size-6 shrink-0 place-items-center rounded-full hairline border-border-strong bg-card text-2xs font-medium">
							{n}
						</span>
						<div>
							<p className="text-xs font-medium">{title}</p>
							<p className="mt-0.5 text-2xs leading-body text-passive">
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
		<div className="flex flex-col items-center gap-5 text-center">
			<div className="grid size-12 place-items-center rounded-xl hairline border-border bg-popover">
				<Target className="size-5" />
			</div>
			<Heading
				icon={null}
				title={t("onboarding.tour.project.title")}
				body="Choose one repository or a workspace of repositories. Registering it records context and provider preferences; it does not start agent work."
			/>
			<div className="w-full rounded-lg hairline border-border bg-popover/55 p-4 text-left">
				<p className="text-xs font-medium">{t("onboarding.tour.project.guideIntro")}</p>
				<ul className="mt-2 grid grid-cols-2 gap-x-4 gap-y-1.5 text-2xs text-passive">
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
				size="sm"
				variant="primary"
			>
				<FolderPlus className="size-icon-sm" />
				{t("onboarding.tour.project.chooseFolder")}
			</Button>
			<p className="text-2xs text-passive">
				{t("onboarding.tour.project.reopenHint")}
			</p>
		</div>
	);
}
