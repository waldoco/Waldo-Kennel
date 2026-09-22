export interface MacOSApplicationActivation {
	setActivationPolicy(policy: "regular"): void;
	dock?: { show(): Promise<void> };
}

/** Promote the primary macOS app out of accessory/UIElement mode before creating its BaseWindow. */
export function promotePrimaryMacOSApp(platform: NodeJS.Platform, application: MacOSApplicationActivation): void {
	if (platform !== "darwin") return;
	application.setActivationPolicy("regular");
	void application.dock?.show();
}
