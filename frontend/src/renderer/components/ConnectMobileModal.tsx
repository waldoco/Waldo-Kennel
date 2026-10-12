import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { useEffect, useRef, useState } from "react";
import { Check, Copy, Info, Smartphone, X } from "lucide-react";
import { QRCodeSVG } from "qrcode.react";
import { apiClient, apiErrorMessage } from "../lib/api-client";
import { captureRendererEvent } from "../lib/telemetry";
import { cn } from "../lib/utils";
import { ConnectMobileGetApp } from "./settings/ConnectMobileGetApp";
import { ConnectMobileSetup, type SetupMode } from "./settings/ConnectMobileSetup";
import { MobileDevicesSection } from "./settings/MobileDevicesSection";
import {
	Dialog,
	DialogClose,
	DialogContent,
	DialogDescription,
	DialogHeader,
	DialogTitle,
	settingsDialogBodyClass,
	settingsDialogContentClass,
	settingsDialogHeaderClass,
} from "./ui/dialog";
import { Button } from "./ui/button";
import { Switch } from "./ui/switch";

export const mobileStatusQueryKey = ["mobile-status"] as const;

/** Matches `--size-settings-mobile-qr-code`; qrcode.react needs a px number. */
const QR_CODE_SIZE = 204;

interface MobileStatus {
	enabled: boolean;
	host: string;
	tailscaleHost: string;
	port: number;
	password: string;
	warning: string;
	securePairing: {
		enabled: boolean;
		available: boolean;
		active: boolean;
		host: string;
		port: number;
		reason: string;
	};
}

// pairingPayload is the QR code contents scanned by the mobile app to connect
// to the desktop's LAN bridge. It includes the password so a single scan
// autofills everything and connects with no typing. The bridge is a trusted-
// home-network tool over plaintext HTTP, so a QR that grants access is an
// acceptable trade-off; regenerating the password invalidates any old QR.
//
// `secure` is omitted unless true so every plaintext QR stays byte-identical
// to what older app builds already scan successfully.
export function pairingPayload(host: string, port: number, password: string, secure?: boolean): string {
	return JSON.stringify(secure ? { v: 1, host, port, password, secure: true } : { v: 1, host, port, password });
}

async function fetchMobileStatus(): Promise<MobileStatus> {
	const { data, error } = await apiClient.GET("/api/v1/mobile/status");
	if (error || !data) throw new Error(apiErrorMessage(error));
	return data;
}

interface ConnectMobileModalProps {
	open: boolean;
	onOpenChange: (open: boolean) => void;
}

// ConnectMobileModal lets a user pair the mobile app with this desktop over
// the LAN bridge. A single "Allow mobile pairing" toggle sits at the top; flipping it
// on starts the bridge and reveals the pairing details below the toggle row —
// a QR code (host/port/password), the plaintext address + password with a copy
// affordance, and a Regenerate action. Flipping it off tears the bridge down.
export function ConnectMobileModal({ open, onOpenChange }: ConnectMobileModalProps) {
	const { t } = useTranslation();
	const queryClient = useQueryClient();
	const [copied, setCopied] = useState(false);
	const copiedTimeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null);
	const [mode, setMode] = useState<SetupMode>("lan");

	useEffect(() => {
		return () => {
			if (copiedTimeoutRef.current) clearTimeout(copiedTimeoutRef.current);
		};
	}, []);

	const query = useQuery({
		queryKey: mobileStatusQueryKey,
		queryFn: fetchMobileStatus,
		enabled: open,
	});

	// Reported once per open, and only after the status query resolves, so
	// bridge_enabled reflects the real state rather than the `false` default that
	// every open would otherwise report. Reset on close so reopening counts again.
	const reportedOpen = useRef(false);
	const initialEnabled = query.data?.enabled;
	useEffect(() => {
		if (!open) {
			reportedOpen.current = false;
			setMode("lan");
			return;
		}
		if (initialEnabled === undefined || reportedOpen.current) return;
		reportedOpen.current = true;
		void captureRendererEvent("kennel.renderer.mobile_connect_opened", { bridge_enabled: initialEnabled });
	}, [open, initialEnabled]);

	const invalidate = () => {
		void queryClient.invalidateQueries({ queryKey: mobileStatusQueryKey });
	};

	const enable = useMutation({
		mutationFn: async () => {
			const { data, error } = await apiClient.POST("/api/v1/mobile/enable");
			if (error) throw new Error(apiErrorMessage(error));
			return data;
		},
		onSuccess: invalidate,
	});

	const disable = useMutation({
		mutationFn: async () => {
			const { data, error } = await apiClient.POST("/api/v1/mobile/disable");
			if (error) throw new Error(apiErrorMessage(error));
			return data;
		},
		onSuccess: invalidate,
	});

	const regenerate = useMutation({
		mutationFn: async () => {
			const { data, error } = await apiClient.POST("/api/v1/mobile/regenerate");
			if (error) throw new Error(apiErrorMessage(error));
			return data;
		},
		onSuccess: invalidate,
	});

	const setSecure = useMutation({
		mutationFn: async (secureEnabled: boolean) => {
			const { data, error } = await apiClient.POST("/api/v1/mobile/secure-pairing", { body: { enabled: secureEnabled } });
			if (error) throw new Error(apiErrorMessage(error));
			return data;
		},
		onSuccess: invalidate,
	});

	const status = query.data;
	const enabled = status?.enabled ?? false;
	// The QR encodes whichever address matches the selected tab. Either can be
	// empty — no LAN interface, or Tailscale not running — and an empty host
	// would otherwise produce a QR the phone rejects outright.
	const secureActive = mode === "tailscale" && (status?.securePairing?.active ?? false);
	const activeHost = secureActive
		? status!.securePairing.host
		: mode === "tailscale"
			? (status?.tailscaleHost ?? "")
			: (status?.host ?? "");
	const activePort = secureActive ? status!.securePairing.port : (status?.port ?? 0);
	// Blocked = the user asked for secure pairing but it isn't usable yet. Show
	// setup steps instead of a QR that cannot connect.
	const secureBlocked = mode === "tailscale" && (status?.securePairing?.enabled ?? false) && !secureActive;
	const busy = enable.isPending || disable.isPending || regenerate.isPending || setSecure.isPending;

	const clearActionErrors = () => {
		enable.reset();
		disable.reset();
		regenerate.reset();
		setSecure.reset();
	};

	const copyPassword = async () => {
		if (!status?.password) return;
		try {
			await navigator.clipboard.writeText(status.password);
			setCopied(true);
			if (copiedTimeoutRef.current) clearTimeout(copiedTimeoutRef.current);
			copiedTimeoutRef.current = setTimeout(() => setCopied(false), 1500);
		} catch {
			// Clipboard can reject (permissions / non-secure context).
		}
	};

	const onToggle = (next: boolean) => {
		if (busy) return;
		clearActionErrors();
		// Enabling is the step that starts the bridge and reveals the QR, so this
		// paired with kennel.mobile.device_connected (emitted by the daemon when a
		// phone actually authenticates) is what shows how many people who set this
		// up ever finish the scan.
		const report = (outcome: "succeeded" | "failed") => {
			void captureRendererEvent("kennel.renderer.mobile_bridge_toggled", { enabled: next, outcome });
		};
		const mutation = next ? enable : disable;
		mutation.mutate(undefined, { onSuccess: () => report("succeeded"), onError: () => report("failed") });
	};

	const actionError =
		(enable.error instanceof Error && enable.error.message) ||
		(disable.error instanceof Error && disable.error.message) ||
		(regenerate.error instanceof Error && regenerate.error.message) ||
		(setSecure.error instanceof Error && setSecure.error.message) ||
		null;

	return (
		<Dialog open={open} onOpenChange={onOpenChange}>
			<DialogContent
				showCloseButton={false}
				className={cn(settingsDialogContentClass, "w-[min(var(--size-settings-mobile-dialog),calc(100vw-var(--space-8)))]")}
			>
				<DialogClose asChild>
					<button
						type="button"
						className="settings-dialog-close-button settings-close-button"
						aria-label={t("mobile.close")}
					>
						<X className="size-5" aria-hidden="true" />
					</button>
				</DialogClose>
				{/* The get-app QR and setup steps can push this past a short window,
				    so the body scrolls rather than clipping under the screen edges. */}
				<DialogHeader className={cn(settingsDialogHeaderClass, "items-start gap-1 border-b-0 p-6 pb-2 pr-14 text-left")}>
					<DialogTitle className="settings-dialog-title text-left">{t("mobile.title")}</DialogTitle>
					<DialogDescription className="wd-body-s max-w-(--size-settings-mobile-desc) text-left text-wd-secondary">
						{t("mobile.description")}
					</DialogDescription>
				</DialogHeader>
				<div className={cn(settingsDialogBodyClass, "max-h-[80vh] gap-0 p-6 pt-4 scrollbar-none")}>
					<ConnectMobileGetApp />

					{query.isLoading ? (
						<p className="wd-body-s mt-6 text-center text-wd-secondary">{t("mobile.checkingStatus")}</p>
					) : query.isError ? (
						<p className="wd-body-s mt-6 text-center text-critical">
							{query.error instanceof Error ? query.error.message : t("mobile.loadFailed")}
						</p>
					) : status ? (
						<div className="mt-4 flex flex-col">
							{/* Toggle row — always visible. Flipping it starts/stops the bridge. */}
							<div className="settings-row-bar items-start">
								<div className="flex min-w-0 items-start gap-3 pr-2">
									<span
										className="settings-row-icon grid size-7 shrink-0 place-items-center rounded-sm bg-wd-inset text-wd-secondary"
										aria-hidden="true"
									>
										<Smartphone className="size-4" />
									</span>
									<div className="flex min-w-0 flex-col gap-0.5">
										<span className="wd-body text-wd-primary">{t("mobile.enable")}</span>
										<span className="wd-body-s text-wd-secondary">{t("mobile.enableHint")}</span>
									</div>
								</div>
								<div className="flex shrink-0 items-center gap-2 pt-0.5">
									<Switch
										checked={enabled}
										onCheckedChange={onToggle}
										disabled={busy}
										aria-label={t("mobile.enable")}
									/>
								</div>
							</div>

							{/* What pairing grants, stated before the switch is flipped. */}
							<div className="wd-caption mt-2 flex flex-col gap-1 px-4 text-wd-tertiary">
								<p>{t("mobile.scope.can")}</p>
								<p>{t("mobile.scope.cannot")}</p>
							</div>

							{actionError && <p role="alert" className="wd-body-s mt-3 px-4 text-critical">{actionError}</p>}

							{/* Pairing details — expand/collapse with the enable toggle. */}
							<div
								className={cn(
									"grid transition-[grid-template-rows] duration-slow ease-out",
									enabled ? "grid-rows-[1fr]" : "grid-rows-[0fr]",
								)}
								aria-hidden={!enabled}
							>
					<div className="overflow-hidden">
									<div
										className={cn(
											"mt-4 flex flex-col items-center transition-opacity duration-slow ease-out",
											enabled ? "opacity-100" : "opacity-0",
										)}
									>
										{/* Steps sit above the QR so the LAN/Tailscale choice is on screen
										    the moment the bridge turns on, with no scrolling. */}
										<ConnectMobileSetup
											mode={mode}
											onModeChange={setMode}
											enabled={enabled}
											busy={busy}
											secure={{
												enabled: status.securePairing?.enabled ?? false,
												reason: status.securePairing?.reason ?? "",
											}}
											onSecureChange={(on) => {
												clearActionErrors();
												setSecure.mutate(on);
											}}
										/>

										<div className="mt-6 flex w-(--size-settings-mobile-qr) flex-col items-center">
											{activeHost && !secureBlocked ? (
												<>
													{/* The code must stay dark-on-white in both themes for cameras. */}
													<div className="rounded-lg bg-[var(--coat-0)] p-3">
														<QRCodeSVG
															value={pairingPayload(activeHost, activePort, status.password, secureActive)}
															data-qr-value={pairingPayload(activeHost, activePort, status.password, secureActive)}
															size={QR_CODE_SIZE}
															className="block size-(--size-settings-mobile-qr-code)"
														/>
													</div>
													<p className="wd-body-s mt-3 text-wd-secondary">{t("mobile.scanToPair")}</p>
												</>
											) : (
												<div className="flex size-(--size-settings-mobile-qr-code) items-center justify-center rounded-lg bg-wd-inset p-4">
													<p className="wd-body-s text-center text-wd-secondary">
														{mode === "tailscale" ? t("mobile.noTailscaleHost") : t("mobile.noPairingHost")}
													</p>
												</div>
											)}
										</div>

										{status.warning && !secureActive && (
											<p className="wd-body-s mt-6 flex w-full max-w-(--size-settings-mobile-warning) items-start gap-2 text-caution">
												<Info className="mt-0.5 size-3.5 shrink-0" aria-hidden="true" />
												<span>{status.warning}</span>
											</p>
										)}

										<div className="mt-6 flex w-full flex-col gap-2 rounded-md bg-wd-inset px-4 py-3">
											<div className="wd-body-s flex items-center gap-6" data-testid="mobile-pairing-address">
												<span className="w-(--size-settings-mobile-label) shrink-0 text-wd-secondary">{t("mobile.address")}</span>
												<span className="wd-data text-wd-primary">
													{activeHost ? `${activeHost}:${activePort}` : "—"}
												</span>
											</div>
											<div className="wd-body-s flex items-center gap-6">
												<span className="w-(--size-settings-mobile-label) shrink-0 text-wd-secondary">{t("mobile.password")}</span>
												<div className="flex min-w-0 items-center gap-2">
													<span className="wd-data text-wd-primary">{status.password}</span>
													<button
														type="button"
														aria-label={copied ? t("mobile.passwordCopied") : t("mobile.copyPassword")}
														tabIndex={enabled ? 0 : -1}
														className="inline-flex size-7 shrink-0 items-center justify-center rounded-full text-wd-secondary transition-colors duration-fast hover:bg-wd-hover hover:text-wd-primary"
														onClick={() => void copyPassword()}
													>
														{copied ? (
															<Check className="size-4" aria-hidden="true" />
														) : (
															<Copy className="size-4" aria-hidden="true" />
														)}
													</button>
												</div>
											</div>
										</div>

										<Button
											type="button"
											variant="secondary"
											className="mt-4"
											onClick={() => {
												clearActionErrors();
												regenerate.mutate();
											}}
											disabled={busy || !enabled}
											tabIndex={enabled ? 0 : -1}
										>
											{t("mobile.regenerate")}
										</Button>

										{/* Not just visually hidden with the rest of this wrapper: the roster's
										    Switch/remove controls issue real PATCH/DELETE calls, so they must be
										    absent from the DOM (not merely aria-hidden) while the bridge is off —
										    otherwise they stay keyboard-focusable behind the collapse animation. */}
										{enabled && (
											<div className="mt-8 w-full">
												<MobileDevicesSection />
											</div>
										)}
									</div>
								</div>
							</div>
						</div>
					) : null}
				</div>
			</DialogContent>
		</Dialog>
	);
}
