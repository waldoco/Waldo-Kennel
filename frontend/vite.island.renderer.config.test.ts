import path from "node:path";
import { fileURLToPath } from "node:url";
import type { Plugin, UserConfig } from "vite";
import { describe, expect, it, vi } from "vitest";
import islandConfig from "./vite.island.renderer.config";

const frontendRoot = path.dirname(fileURLToPath(import.meta.url));
const islandImporter = path.resolve(
	frontendRoot,
	"../packages/kennel-island/src/main.tsx",
);

function islandDependencyBoundary(): Plugin {
	const plugins = (islandConfig as UserConfig).plugins ?? [];
	const plugin = plugins
		.flatMap((candidate) => (Array.isArray(candidate) ? candidate : [candidate]))
		.find(
			(candidate): candidate is Plugin =>
				typeof candidate === "object" &&
				candidate !== null &&
				"name" in candidate &&
				candidate.name === "island-frontend-dependency-boundary",
		);

	expect(plugin).toBeDefined();
	return plugin as Plugin;
}

describe("Island renderer dependency resolution", () => {
	it("resolves Island runtime dependencies from the frontend install", async () => {
		const plugin = islandDependencyBoundary();
		const resolve = vi.fn(async (source: string, importer: string | undefined) => ({
			id: `${importer}:${source}`,
		}));
		const sources = [
			"react",
			"react-dom/client",
			"motion/react",
			"@fontsource-variable/geist",
		];
		const hook = typeof plugin.resolveId === "object"
			? plugin.resolveId.handler
			: plugin.resolveId;
		const importers = [islandImporter, islandImporter.replaceAll("/", "\\")];

		expect(hook).toBeTypeOf("function");
		for (const importer of importers) {
			for (const source of sources) {
				await expect(
					hook!.call({ resolve } as never, source, importer, {} as never),
				).resolves.toEqual({
					id: `${path.join(frontendRoot, "src/renderer/main.tsx")}:${source}`,
				});
			}
		}

		expect(resolve).toHaveBeenCalledTimes(sources.length * importers.length);
		for (const source of sources) {
			expect(resolve).toHaveBeenCalledWith(
				source,
				path.join(frontendRoot, "src/renderer/main.tsx"),
				{ skipSelf: true },
			);
		}
	});
});
