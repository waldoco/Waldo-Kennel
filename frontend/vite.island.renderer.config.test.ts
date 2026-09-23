import path from "node:path";
import { fileURLToPath } from "node:url";
import { normalizePath, resolveConfig, type UserConfig } from "vite";
import { describe, expect, it } from "vitest";
import islandConfig from "./vite.island.renderer.config";

const frontendRoot = path.dirname(fileURLToPath(import.meta.url));
const frontendModules = `${normalizePath(path.join(frontendRoot, "node_modules"))}/`;

describe("Island renderer dependency resolution", () => {
	it("resolves dependency-optimizer entries from the frontend install", async () => {
		const config = await resolveConfig(
			{ ...(islandConfig as UserConfig), configFile: false },
			"serve",
		);
		const resolve = config.createResolver();
		const sources = [
			"react",
			"react/jsx-dev-runtime",
			"react-dom/client",
			"motion/react",
			"@fontsource-variable/geist",
		];

		for (const source of sources) {
			const resolved = await resolve(source);
			expect(resolved, source).toBeTruthy();
			expect(normalizePath(resolved!).startsWith(frontendModules), source).toBe(true);
		}
	});

	it("allows aliased frontend dependency assets through the dev server", async () => {
		const config = await resolveConfig(
			{ ...(islandConfig as UserConfig), configFile: false },
			"serve",
		);
		const allowed = config.server.fs.allow.map((entry) => normalizePath(entry));

		expect(allowed).toContain(normalizePath(path.join(frontendRoot, "node_modules")));
	});
});
