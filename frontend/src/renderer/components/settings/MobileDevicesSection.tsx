import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Smartphone, X } from "lucide-react";
import { apiClient, apiErrorCode, apiErrorMessage } from "../../lib/api-client";
import { Button } from "../ui/button";
import { Switch } from "../ui/switch";

export const mobileDevicesQueryKey = ["mobile-devices"] as const;

/**
 * Error code the daemon returns from all three roster routes (list/mute/remove)
 * when the on-disk device registry (~/.kennel/data/mobile/push-devices.json) failed
 * to load — e.g. it's corrupt. This is distinct from "you have no devices": an
 * unreadable registry must be surfaced explicitly, never rendered as the empty
 * state.
 */
const DEVICE_REGISTRY_UNAVAILABLE_CODE = "DEVICE_REGISTRY_UNAVAILABLE";

interface MobileDevice {
	installId: string;
	deviceName?: string;
	platform?: string;
	muted: boolean;
	live: boolean;
	notificationsEnabled: boolean;
	lastSeenAt: string;
}

class MobileDevicesQueryError extends Error {
	code?: string;

	constructor(message: string, code?: string) {
		super(message);
		this.code = code;
	}
}

// Relative time is rendered here, on the desktop, and must stay here: Hermes has
// no Intl.RelativeTimeFormat, so reusing this in the mobile app crashes at runtime
// and vitest would not catch it.
//
// The formatter locale is threaded through as a parameter (i18next's resolved
// language) rather than left to default: `new Intl.RelativeTimeFormat(undefined, ...)`
// resolves to the OS locale, which can disagree with the app's own language
// setting (e.g. app running in `ja` on an en-US machine) — exactly the mismatch
// the multi-locale translation work above is meant to prevent.
function lastSeenLabel(iso: string, language: string): string {
	const relative = new Intl.RelativeTimeFormat(language, { numeric: "auto" });
	const seconds = Math.round((Date.parse(iso) - Date.now()) / 1000);
	const units: [Intl.RelativeTimeFormatUnit, number][] = [
		["second", 60],
		["minute", 60],
		["hour", 24],
		["day", 30],
		["month", 12],
	];
	let value = seconds;
	for (const [unit, size] of units) {
		if (Math.abs(value) < size) return relative.format(Math.round(value), unit);
		value /= size;
	}
	return relative.format(Math.round(value), "year");
}

async function fetchDevices(): Promise<MobileDevice[]> {
	const { data, error } = await apiClient.GET("/api/v1/mobile/devices");
	if (error || !data) throw new MobileDevicesQueryError(apiErrorMessage(error), apiErrorCode(error));
	return data.devices as MobileDevice[];
}

// MobileDevicesSection lists every paired phone with whether its app is open right
// now, a per-device mute switch, and a remove action. Live status comes from the
// daemon's presence tracker, which is fed by each phone's own REST poll.
//
// The caller (ConnectMobileModal) must only mount this while the bridge is
// enabled — it is not gated on that itself, and its Switch/remove buttons issue
// real PATCH/DELETE calls the moment they're interacted with.
export function MobileDevicesSection() {
	const { t, i18n } = useTranslation();
	const queryClient = useQueryClient();
	const [confirmingRemoval, setConfirmingRemoval] = useState<string | null>(null);

	const query = useQuery({
		queryKey: mobileDevicesQueryKey,
		queryFn: fetchDevices,
		refetchInterval: 3000,
	});

	const invalidate = () => {
		void queryClient.invalidateQueries({ queryKey: mobileDevicesQueryKey });
	};

	const mute = useMutation({
		mutationFn: async ({ installId, muted }: { installId: string; muted: boolean }) => {
			const { error } = await apiClient.PATCH("/api/v1/mobile/devices/{installId}", {
				params: { path: { installId } },
				body: { muted },
			});
			if (error) throw new Error(apiErrorMessage(error));
		},
		onSuccess: invalidate,
	});

	const remove = useMutation({
		mutationFn: async (installId: string) => {
			const { error } = await apiClient.DELETE("/api/v1/mobile/devices/{installId}", {
				params: { path: { installId } },
			});
			if (error) throw new Error(apiErrorMessage(error));
		},
		onSuccess: () => {
			setConfirmingRemoval(null);
			invalidate();
		},
	});

	const devices = query.data ?? [];
	// Stable client-side order: the daemon sorts live-first then by LastSeenAt
	// descending, and LastSeenAt advances on every phone poll — with 2+ live
	// devices that ordering can flip on any 3s refetch, jumping rows under the
	// cursor mid-interaction (e.g. right as someone reaches for "Confirm remove").
	// installId never changes for a paired device, so sorting on it keeps row
	// order fixed across polls regardless of what the server returns.
	const sortedDevices = [...devices].sort((a, b) => a.installId.localeCompare(b.installId));
	const queryError = query.error as MobileDevicesQueryError | null;
	const registryUnavailable = queryError?.code === DEVICE_REGISTRY_UNAVAILABLE_CODE;
	// A transient poll failure (daemon restart mid-refetch, a one-off 500) should
	// not blank a list we already successfully loaded — that flickers the whole
	// section red and back every time. Only replace the section outright when
	// there is no retained data to show. DEVICE_REGISTRY_UNAVAILABLE always takes
	// over the section regardless of stale data — that state is distinct enough
	// (an unreadable on-disk registry) that showing a stale list next to it would
	// be misleading.
	const hasData = query.data !== undefined;
	const mutationError =
		(mute.error instanceof Error && mute.error.message) ||
		(remove.error instanceof Error && remove.error.message) ||
		null;

	return (
		<section className="flex flex-col gap-2">
			<h3 className="wd-label px-4 text-wd-secondary">{t("mobile.devices.title")}</h3>

			{query.isLoading ? (
				<p className="wd-body-s rounded-lg bg-wd-surface px-4 py-3 text-wd-secondary">{t("mobile.devices.loading")}</p>
			) : registryUnavailable ? (
				<p className="wd-body-s rounded-lg bg-critical-bg px-4 py-3 text-critical">{t("mobile.devices.registryUnavailable")}</p>
			) : queryError && !hasData ? (
				<p className="wd-body-s rounded-lg bg-critical-bg px-4 py-3 text-critical">{queryError.message}</p>
			) : devices.length === 0 ? (
				<p className="wd-body-s rounded-lg border-[length:var(--stroke-hairline)] border-wd-hairline bg-wd-surface px-4 py-3 text-wd-secondary">
					{t("mobile.devices.empty")}
				</p>
			) : (
				<>
					{queryError && <p className="wd-body-s px-4 text-critical">{queryError.message}</p>}
					<ul className="settings-grouped-rows">
						{sortedDevices.map((device) => {
							const name = device.deviceName || t("mobile.devices.unnamed");
							const confirming = confirmingRemoval === device.installId;
							return (
								<li key={device.installId} className="flex flex-col">
									<div className="settings-row-bar">
										<div className="flex min-w-0 items-center gap-3">
											<span
												className="settings-row-icon grid size-7 shrink-0 place-items-center rounded-sm bg-wd-inset text-wd-secondary"
												aria-hidden="true"
											>
												<Smartphone className="size-4" />
											</span>
											<div className="flex min-w-0 flex-col gap-0.5">
												<div className="wd-body truncate text-wd-primary">{name}</div>
												<div className="wd-body-s flex items-center gap-1.5 text-wd-secondary">
													{device.live ? (
														<>
															<span className="size-1.5 shrink-0 rounded-full bg-wd-accent" aria-hidden />
															<span>{t("mobile.devices.live")}</span>
														</>
													) : (
														<span className="wd-data">{lastSeenLabel(device.lastSeenAt, i18n.language)}</span>
													)}
												</div>
												{!device.notificationsEnabled && (
													<div className="wd-body-s text-wd-secondary">
														{t("mobile.devices.notificationsNotEnabled")}
													</div>
												)}
											</div>
										</div>

										<div className="flex shrink-0 items-center gap-2">
											<Switch
												checked={device.notificationsEnabled && !device.muted}
												disabled={mute.isPending || !device.notificationsEnabled}
												aria-label={t("mobile.devices.notificationsFor", { name })}
												onCheckedChange={(next) =>
													mute.mutate({ installId: device.installId, muted: !next })
												}
											/>
											{confirming ? null : (
												<button
													type="button"
													aria-label={t("mobile.devices.removeAria", { name })}
													className="grid size-8 place-items-center rounded-full text-wd-secondary transition-colors duration-fast hover:bg-wd-hover hover:text-wd-primary"
													onClick={() => setConfirmingRemoval(device.installId)}
												>
													<X className="size-4" />
												</button>
											)}
										</div>
									</div>
									{confirming && (
										<div className="flex flex-col gap-3 px-4 pb-4">
											<p className="wd-body-s text-wd-secondary">
												{t("mobile.devices.removeConsequence", { name })}
											</p>
											<div className="flex items-center justify-end gap-2">
												<Button type="button" variant="ghost" size="sm" onClick={() => setConfirmingRemoval(null)}>
													{t("mobile.devices.keep")}
												</Button>
												<Button
													type="button"
													variant="critical"
													size="sm"
													disabled={remove.isPending}
													onClick={() => remove.mutate(device.installId)}
												>
													{t("mobile.devices.confirmRemove")}
												</Button>
											</div>
										</div>
									)}
								</li>
							);
						})}
					</ul>
				</>
			)}

			{mutationError && <p className="wd-body-s px-4 text-critical">{mutationError}</p>}
			<p className="wd-caption px-4 text-wd-tertiary">{t("mobile.devices.description")}</p>
		</section>
	);
}
