import { useTranslation } from "react-i18next";
import waldoMark from "../../../assets/waldo-mark.svg";

// Waldo's working state is calm: one static, specific line, no spinner, dots
// or breathing mark. The status region's accessible name carries the state.
export function DaemonStartupLoader() {
	const { t } = useTranslation();

	return (
		<div
			aria-busy="true"
			aria-label={t("startup.aria", { brand: "Kennel" })}
			aria-live="polite"
			className="ao-startup-screen flex h-full w-full items-center justify-center bg-wd-canvas text-wd-primary"
			data-testid="daemon-startup-loader"
			role="status"
		>
			<div className="ao-startup-content flex -translate-y-[3vh] flex-col items-center gap-2 text-center">
				<div className="grid h-24 w-28 place-items-center" aria-hidden="true">
					<img
						alt=""
						className="ao-startup-logo h-20 w-22 object-contain"
						data-brand="waldo"
						data-testid="waldo-brand-mark"
						src={waldoMark}
					/>
				</div>
				<p className="wd-display-4 mt-3 text-wd-primary">Kennel</p>
				<p className="min-h-5 text-sm text-wd-secondary">
					<span aria-hidden="true" className="ao-startup-status">
						{t("startup.startingDaemon")}
					</span>
				</p>
			</div>
		</div>
	);
}
