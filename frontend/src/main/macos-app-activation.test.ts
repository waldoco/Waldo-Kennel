import { describe, expect, it, vi } from "vitest";
import { promotePrimaryMacOSApp } from "./macos-app-activation";

describe("promotePrimaryMacOSApp", () => {
	it("promotes the macOS app to a regular Dock-visible application", () => {
		const setActivationPolicy = vi.fn();
		const show = vi.fn(async () => undefined);

		promotePrimaryMacOSApp("darwin", { setActivationPolicy, dock: { show } });

		expect(setActivationPolicy).toHaveBeenCalledOnce();
		expect(setActivationPolicy).toHaveBeenCalledWith("regular");
		expect(show).toHaveBeenCalledOnce();
	});

	it("does not change activation policy on other platforms", () => {
		const setActivationPolicy = vi.fn();
		const show = vi.fn(async () => undefined);

		promotePrimaryMacOSApp("linux", { setActivationPolicy, dock: { show } });

		expect(setActivationPolicy).not.toHaveBeenCalled();
		expect(show).not.toHaveBeenCalled();
	});
});
