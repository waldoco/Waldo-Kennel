import { QrCode } from "lucide-react";
import { useTranslation } from "react-i18next";
import { useState } from "react";
import { QRCodeSVG } from "qrcode.react";
import { aoBridge } from "../../lib/bridge";
import { cn } from "../../lib/utils";
import { Button } from "../ui/button";

/** TestFlight beta for the Kennel iOS app. */
export const TESTFLIGHT_URL = "https://testflight.apple.com/join/t4U3fu2H";

/** Android internal-testing signup form. */
export const ANDROID_SIGNUP_URL = "https://forms.gle/pWLWoxTPXySAN4Ws8";

/** Deliberately smaller than the pairing QR so it never competes with it. */
const TESTFLIGHT_QR_SIZE = 140;

// ConnectMobileGetApp is step zero of pairing: the pairing QR below it is
// meaningless until the app is installed. It sits outside the modal's
// enable-collapse because installing the app has nothing to do with whether
// the LAN bridge is running. The QR (of the TestFlight URL itself) hides
// behind a disclosure so the widened modal keeps its height.
export function ConnectMobileGetApp() {
	const { t } = useTranslation();
	const [showQR, setShowQR] = useState(false);

	return (
		<div className="flex flex-col gap-2">
			<span className="wd-label px-4 text-wd-secondary">{t("mobile.getApp")}</span>

			<div className="settings-grouped-rows">
			{/* iOS — items-center so the action cluster sits on the row's optical centre. */}
			<div className="settings-row-bar">
				<div className="flex min-w-0 flex-col gap-0.5">
					<span className="wd-body text-wd-primary">{t("mobile.ios")}</span>
					<span className="wd-body-s text-wd-secondary">
						{t("mobile.iosHint")}
					</span>
				</div>
				<div className="flex shrink-0 items-center gap-1.5">
					<Button
						type="button"
						variant="secondary"
						aria-label={t("mobile.joinTestFlightAria")}
						onClick={() => void aoBridge.app.openExternal(TESTFLIGHT_URL)}
					>
						{t("mobile.joinBeta")}
					</Button>
					<button
						type="button"
						aria-label={showQR ? t("mobile.hideQR") : t("mobile.showQR")}
						aria-expanded={showQR}
						onClick={() => setShowQR((v) => !v)}
						className={cn(
							"inline-flex size-8 items-center justify-center rounded-full transition-colors duration-fast hover:bg-wd-hover",
							showQR ? "bg-wd-selected text-wd-primary" : "text-wd-secondary",
						)}
					>
						<QrCode className="size-4" aria-hidden="true" />
					</button>
				</div>
			</div>

			<div
				data-testid="testflight-qr"
				className={cn(
					"grid transition-[grid-template-rows] duration-slow ease-out",
					showQR ? "grid-rows-[1fr]" : "grid-rows-[0fr]",
				)}
				aria-hidden={!showQR}
			>
				<div className="overflow-hidden">
					<div
						className={cn(
							"flex flex-col items-center pb-4 pt-1 transition-opacity duration-slow ease-out",
							showQR ? "opacity-100" : "opacity-0",
						)}
					>
						<div className="rounded-lg bg-[var(--coat-0)] p-3">
							<QRCodeSVG value={TESTFLIGHT_URL} size={TESTFLIGHT_QR_SIZE} className="block" />
						</div>
						<p className="wd-body-s mt-2 text-wd-secondary">{t("mobile.qrHint")}</p>
					</div>
				</div>
			</div>

			{/* Android — internal testing signup until a Play Store beta is live. */}
			<div className="settings-row-bar">
				<div className="flex min-w-0 flex-col gap-0.5">
					<span className="wd-body text-wd-primary">{t("mobile.android")}</span>
					<span className="wd-body-s text-wd-secondary">
						{t("mobile.androidHint")}
					</span>
				</div>
				<Button
					type="button"
					variant="secondary"
					aria-label={t("mobile.androidSignupAria")}
					onClick={() => void aoBridge.app.openExternal(ANDROID_SIGNUP_URL)}
				>
					{t("mobile.joinWaitlist")}
				</Button>
			</div>
			</div>
		</div>
	);
}
