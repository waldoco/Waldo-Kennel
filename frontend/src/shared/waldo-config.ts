// The Waldo web origin Kennel pairs with. Staging until a production origin exists.
// KENNEL_WALDO_ORIGIN overrides it; KENNEL_WALDO_BRIDGE_ENABLED=0 turns pairing off.
export const DEFAULT_WALDO_ORIGIN = "https://waldo-runtime-staging.piyushfulper3210.workers.dev";
// The Waldo console page that issues pairing codes (signed-out owners return here).
export const WALDO_PAIRING_PATH = "/console/devices";

export type WaldoBridgeConfig = { enabled: false } | { enabled: true; origin: string; ownerId?: string };

// Mirrors the daemon's origin rule: one bare https origin, no path, query or credentials.
function bareHttpsOrigin(value: string): boolean {
	try {
		const url = new URL(value);
		return value.startsWith("https://") && url.protocol === "https:" && !!url.hostname && !url.username && !url.password && url.origin === value;
	} catch {
		return false;
	}
}

// Pairing is on by default on macOS only; elsewhere it needs KENNEL_WALDO_BRIDGE_ENABLED=1.
// The owner is optional: without it the daemon learns it from the first pairing code.
export function waldoBridgeConfig(env: Record<string, string | undefined>, platform: string): WaldoBridgeConfig {
	const flag = env.KENNEL_WALDO_BRIDGE_ENABLED;
	if (flag !== "1" && !(flag === undefined && platform === "darwin")) return { enabled: false };
	const origin = env.KENNEL_WALDO_ORIGIN || DEFAULT_WALDO_ORIGIN;
	if (!bareHttpsOrigin(origin)) return { enabled: false };
	const ownerId = env.KENNEL_WALDO_OWNER_ID;
	return ownerId ? { enabled: true, origin, ownerId } : { enabled: true, origin };
}

// The daemon reads these exact keys; a disabled config forwards an explicit off.
export function waldoBridgeDaemonEnv(config: WaldoBridgeConfig): Record<string, string> {
	if (!config.enabled) return { KENNEL_WALDO_BRIDGE_ENABLED: "0" };
	return { KENNEL_WALDO_BRIDGE_ENABLED: "1", KENNEL_WALDO_ORIGIN: config.origin, ...(config.ownerId ? { KENNEL_WALDO_OWNER_ID: config.ownerId } : {}) };
}

export function waldoPairingUrl(config: WaldoBridgeConfig): string | null {
	return config.enabled ? config.origin + WALDO_PAIRING_PATH : null;
}
