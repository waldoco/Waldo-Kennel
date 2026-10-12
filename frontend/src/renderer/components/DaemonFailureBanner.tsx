import { AlertTriangle } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import type { DaemonStatus } from "../../shared/daemon-status";
import { daemonFailureHint, daemonFailureMessage, daemonFailureTitle } from "../lib/daemon-failure";
import { aoBridge } from "../lib/bridge";
import { cn } from "../lib/utils";
import { Button } from "./ui/button";

export function DaemonFailureBanner({ status }: { status: DaemonStatus }) {
	if (!status.code || status.state === "ready") return null;
	return <DaemonFailureContent status={status} />;
}

function DaemonFailureContent({ status }: { status: DaemonStatus }) {
	const { t } = useTranslation();
	const [detailsOpen, setDetailsOpen] = useState(false);
	const [copied, setCopied] = useState(false);
	const [restarting, setRestarting] = useState(false);
	const [restartError, setRestartError] = useState<string | null>(null);
	const copiedTimeout = useRef<ReturnType<typeof setTimeout> | null>(null);
	const details = status.details?.trim();
	const hint = daemonFailureHint(status, t);
	const title = daemonFailureTitle(status, t);
	const canRestart =
		status.code === "not_ready" ||
		status.code === "spawn_failed" ||
		status.code === "exited" ||
		status.code === "identity_mismatch" ||
		status.code === "daemon_unreachable";
	useEffect(() => {
		setCopied(false);
		return () => {
			if (copiedTimeout.current !== null) clearTimeout(copiedTimeout.current);
		};
	}, [details]);
	const copyDetails = async () => {
		const lines = [
			title,
			t("daemon.details.code", { code: status.code ?? "unknown" }),
			t("daemon.details.message", { message: daemonFailureMessage(status, t) }),
			details ? `\n${t("daemon.details.heading", { details })}` : "",
		];
		await aoBridge.clipboard.writeText(lines.filter(Boolean).join("\n"));
		setCopied(true);
		if (copiedTimeout.current !== null) clearTimeout(copiedTimeout.current);
		copiedTimeout.current = setTimeout(() => {
			setCopied(false);
			copiedTimeout.current = null;
		}, 2_000);
	};
	const restartDaemon = async () => {
		setRestarting(true);
		setRestartError(null);
		try {
			const nextStatus = await aoBridge.daemon.restart();
			if (nextStatus.state === "error" || nextStatus.state === "stopped") {
				setRestartError(daemonFailureMessage(nextStatus, t));
			}
		} catch (error) {
			setRestartError(error instanceof Error ? error.message : t("daemon.restartFailed"));
		} finally {
			setRestarting(false);
		}
	};
	// Waldo Banner: caution while the daemon may still come up on its own,
	// critical once it has failed. Title says what happened; the body says what
	// Kennel is doing and what still works; one action, named for the fix.
	const tone = status.code === "not_ready" || status.code === "port_unconfirmed" ? "caution" : "critical";
	const FOCUS = "focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-(--border-focus)";
	return (
		<section
			aria-live="assertive"
			className={cn(
				"pointer-events-auto fixed top-3 right-3 z-overlay flex w-daemon-failure-toast flex-col gap-1 rounded-md p-4 text-sm text-wd-primary shadow-md",
				tone === "critical" ? "bg-critical-bg" : "bg-caution-bg",
			)}
			data-tone={tone}
			role="alert"
		>
			<div className="flex items-start gap-2">
				<AlertTriangle
					className={cn("mt-0.5 size-icon-base shrink-0", tone === "critical" ? "text-critical" : "text-caution")}
					aria-hidden="true"
				/>
				<p className="min-w-0 flex-1 font-medium text-wd-primary">{title}</p>
				{status.code ? (
					<code className="shrink-0 rounded-xs bg-wd-inset px-1.5 py-0.5 font-mono text-xs text-wd-secondary">
						{status.code}
					</code>
				) : null}
			</div>
			<p className="wrap-break-word text-wd-secondary">{daemonFailureMessage(status, t)}</p>
			{hint ? <p className="text-wd-secondary">{hint}</p> : null}
			<p className="text-wd-secondary">{t("daemon.stillWorks")}</p>
			{restartError ? <p className="text-critical">{restartError}</p> : null}
			<div className="mt-2 flex flex-wrap items-center gap-2">
				{details ? (
					<>
						<button
							type="button"
							className={cn("rounded-full px-2.5 py-1 text-xs text-wd-secondary transition-colors duration-fast hover:bg-wd-hover hover:text-wd-primary", FOCUS)}
							onClick={() => setDetailsOpen((open) => !open)}
						>
							{detailsOpen ? t("daemon.hideDetails") : t("daemon.showDetails")}
						</button>
						<button
							type="button"
							className={cn("rounded-full px-2.5 py-1 text-xs text-wd-secondary transition-colors duration-fast hover:bg-wd-hover hover:text-wd-primary", FOCUS)}
							onClick={() => void copyDetails()}
						>
							{copied ? t("daemon.copied") : t("daemon.copyDetails")}
						</button>
					</>
				) : null}
				{canRestart ? (
					<Button
						type="button"
						size="sm"
						variant="primary"
						className="ml-auto disabled:cursor-wait"
						disabled={restarting}
						onClick={() => void restartDaemon()}
					>
						{restarting ? t("daemon.restarting") : t("daemon.restart")}
					</Button>
				) : null}
			</div>
			{details && detailsOpen ? (
				<pre className="mt-2 max-h-daemon-failure-details-max w-full overflow-auto rounded-sm bg-wd-inset px-2 py-1.5 font-mono text-caption leading-relaxed text-wd-secondary">
					{details}
				</pre>
			) : null}
		</section>
	);
}
