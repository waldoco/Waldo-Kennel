import { useTranslation } from "react-i18next";
import type { TFunction } from "i18next";
import { RadioGroup } from "radix-ui";
import { Switch } from "../ui/switch";

export type SetupMode = "lan" | "tailscale";

// Maps a `securePairing.reason` from the daemon to the copy explaining it. An
// unknown reason (e.g. a newer daemon build) returns undefined and renders
// nothing rather than a raw translation key.
function reasonMessage(reason: string, t: TFunction): string | undefined {
	switch (reason) {
		case "no_cli":
		case "no_magicdns":
			return t("mobile.securePairing.noCli");
		case "no_certs":
			return t("mobile.securePairing.noCerts");
		case "serve_failed":
			return t("mobile.securePairing.serveFailed");
		case "port_mismatch":
			return t("mobile.securePairing.portMismatch");
		case "clear_failed":
			return t("mobile.securePairing.clearFailed");
		default:
			return undefined;
	}
}

interface ConnectMobileSetupProps {
	/** The selected connection method. Owned by the modal, which encodes the
	 *  matching address into the pairing QR. */
	mode: SetupMode;
	onModeChange: (mode: SetupMode) => void;
	/**
	 * False while the bridge is off; the steps are then collapsed, so their
	 * controls must leave the tab order (same pattern as the pairing block).
	 */
	enabled: boolean;
	/**
	 * True while any bridge mutation (enable/disable/regenerate/secure-pairing)
	 * is in flight. Disables the secure-pairing switch so a user can't fire a
	 * second toggle mid round-trip — the secure-pairing POST can take several
	 * seconds (up to three chained `tailscale` subprocess calls).
	 */
	busy?: boolean;
	/** Secure-pairing (Tailscale TLS) state, owned by the modal. */
	secure: { enabled: boolean; reason: string };
	onSecureChange: (on: boolean) => void;
}

// ConnectMobileSetup tells the user what to do with the pairing QR above it and
// which address that QR carries. The mode is owned by ConnectMobileModal so the
// QR can re-encode: LAN mode encodes the private IPv4 from AutopickLANIP, and
// Tailscale mode encodes the 100.64.0.0/10 address from AutopickTailscaleIP
// (backend/internal/mobilebridge/netiface.go), or the MagicDNS host over 443
// when secure pairing is active.
export function ConnectMobileSetup({ mode, onModeChange, enabled, busy = false, secure, onSecureChange }: ConnectMobileSetupProps) {
	const { t } = useTranslation();
	const reasonText = reasonMessage(secure.reason, t);

	// Margin-free on purpose: the modal owns the spacing around this block.
	return (
		<div className="flex w-full flex-col items-center">
			<RadioGroup.Root
				value={mode}
				onValueChange={(value) => onModeChange(value as SetupMode)}
				aria-label={t("mobile.connectionMethod")}
				className="settings-segment"
			>
				<RadioGroup.Item value="lan" tabIndex={enabled ? 0 : -1} className="settings-segment-item min-w-24">
					{t("mobile.lan")}
				</RadioGroup.Item>
				<RadioGroup.Item value="tailscale" tabIndex={enabled ? 0 : -1} className="settings-segment-item min-w-24">
					{t("mobile.tailscale")}
				</RadioGroup.Item>
			</RadioGroup.Root>

			{mode === "lan" ? (
				<div className="mt-4 w-full">
					<ol className="settings-mobile-steps">
						<li>{t("mobile.lan.step1")}</li>
						<li>{t("mobile.lan.step2")}</li>
						<li>{t("mobile.lan.step3")}</li>
					</ol>
				</div>
			) : (
				<div className="mt-4 w-full">
					<div className="settings-row-bar items-start">
						<div className="flex min-w-0 flex-col gap-0.5 pr-2">
							<span className="wd-body text-wd-primary">
								{t("mobile.securePairing")}
							</span>
							<span className="wd-body-s text-wd-secondary">
								{t("mobile.securePairing.hint")}
							</span>
						</div>
						<Switch
							checked={secure.enabled}
							onCheckedChange={onSecureChange}
							disabled={!enabled || busy}
							aria-label={t("mobile.securePairing")}
							tabIndex={enabled ? 0 : -1}
							className="mt-0.5"
						/>
					</div>

					<p className="wd-caption mt-2 px-4 text-wd-tertiary">
						{t("mobile.tailscale.iosHint")}
					</p>

					{secure.enabled && reasonText && (
						<p className="wd-body-s mt-3 px-4 text-caution">{reasonText}</p>
					)}

					<ol className="settings-mobile-steps mt-4">
						<li>{t("mobile.tailscale.step1")}</li>
						<li>
							{t("mobile.tailscale.step2Lead")}{" "}
							<code className="wd-cli rounded-xs bg-wd-inset px-1.5 py-0.5 text-wd-primary">tailscale ip -4</code>{" "}
							{t("mobile.tailscale.step2Trail")}
						</li>
						<li>{t("mobile.tailscale.step3")}</li>
					</ol>
				</div>
			)}
		</div>
	);
}
