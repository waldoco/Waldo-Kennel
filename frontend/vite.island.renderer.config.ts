import path from "node:path";
import { fileURLToPath } from "node:url";
import react from "@vitejs/plugin-react";
import { defineConfig, normalizePath } from "vite";
import type { Plugin } from "vite";

const frontendRoot = path.dirname(fileURLToPath(import.meta.url));
const islandRoot = path.resolve(frontendRoot, "../packages/kennel-island");
const normalizeModuleId = (id: string): string =>
	normalizePath(id).replaceAll("\\", "/");
const normalizedIslandRoot = `${normalizeModuleId(islandRoot)}/`;

// Forge installs the desktop dependency graph in frontend/node_modules, while
// the Island source lives in a sibling package. Bare imports otherwise resolve
// relative to packages/kennel-island and fail in a clean desktop checkout.
const islandFrontendDependencyBoundary: Plugin = {
	name: "island-frontend-dependency-boundary",
	enforce: "pre",
	async resolveId(source, importer) {
		if (!importer || !normalizeModuleId(importer).startsWith(normalizedIslandRoot)) {
			return null;
		}
		const remap =
			source === "react" ||
			source.startsWith("react/") ||
			source === "react-dom" ||
			source.startsWith("react-dom/") ||
			source === "motion" ||
			source.startsWith("motion/") ||
			source === "@fontsource-variable/geist" ||
			source.startsWith("@fontsource-variable/geist/");
		if (!remap) {
			return null;
		}
		return this.resolve(
			source,
			path.join(frontendRoot, "src/renderer/main.tsx"),
			{ skipSelf: true },
		);
	},
};

// The Island remains a shared production renderer and a browser-only visual
// lab in packages/kennel-island. Forge owns only its output directory, keeping
// the detailed Island UI and its renderer tests in one canonical source tree.
export default defineConfig({
	root: islandRoot,
	publicDir: path.join(islandRoot, "public"),
	plugins: [islandFrontendDependencyBoundary, react()],
	build: {
		outDir: path.join(frontendRoot, ".vite/renderer/island_window"),
		emptyOutDir: true,
	},
});
