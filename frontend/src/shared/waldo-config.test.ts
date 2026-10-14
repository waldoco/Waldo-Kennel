import { describe, expect, it } from "vitest";
import { DEFAULT_WALDO_ORIGIN, waldoBridgeConfig, waldoBridgeDaemonEnv, waldoPairingUrl } from "./waldo-config";

describe("Connect to Waldo boot configuration", () => {
	it("is on by default on macOS with the built-in origin and no owner", () => {
		const config = waldoBridgeConfig({}, "darwin");
		expect(config).toStrictEqual({ enabled: true, origin: DEFAULT_WALDO_ORIGIN });
		expect(waldoBridgeDaemonEnv(config)).toStrictEqual({ KENNEL_WALDO_BRIDGE_ENABLED: "1", KENNEL_WALDO_ORIGIN: DEFAULT_WALDO_ORIGIN });
		expect(waldoPairingUrl(config)).toBe(`${DEFAULT_WALDO_ORIGIN}/console/devices`);
	});
	it("stays off elsewhere unless explicitly enabled, and can be turned off on macOS", () => {
		expect(waldoBridgeConfig({}, "linux")).toStrictEqual({ enabled: false });
		expect(waldoBridgeConfig({}, "win32")).toStrictEqual({ enabled: false });
		expect(waldoBridgeConfig({ KENNEL_WALDO_BRIDGE_ENABLED: "1" }, "linux").enabled).toBe(true);
		expect(waldoBridgeConfig({ KENNEL_WALDO_BRIDGE_ENABLED: "0" }, "darwin")).toStrictEqual({ enabled: false });
		expect(waldoBridgeDaemonEnv({ enabled: false })).toStrictEqual({ KENNEL_WALDO_BRIDGE_ENABLED: "0" });
		expect(waldoPairingUrl({ enabled: false })).toBeNull();
	});
	it("honours an explicit origin and owner, and refuses an origin the daemon would refuse", () => {
		expect(waldoBridgeConfig({ KENNEL_WALDO_ORIGIN: "https://localhost:8544", KENNEL_WALDO_OWNER_ID: "owner-1" }, "darwin"))
			.toStrictEqual({ enabled: true, origin: "https://localhost:8544", ownerId: "owner-1" });
		for (const origin of ["http://waldo.test", "https://waldo.test/", "https://waldo.test/console", "https://user@waldo.test", "https://waldo.test?x=1", "not a url"])
			expect(waldoBridgeConfig({ KENNEL_WALDO_ORIGIN: origin }, "darwin")).toStrictEqual({ enabled: false });
	});
});
