import path from "node:path";
import { fileURLToPath } from "node:url";
import react from "@vitejs/plugin-react";
import { defineConfig } from "vite";

const frontendRoot = path.dirname(fileURLToPath(import.meta.url));
const islandRoot = path.resolve(frontendRoot, "../packages/kennel-island");

// Forge installs the desktop dependency graph in frontend/node_modules, while
// the Island source lives in a sibling package. Bare imports otherwise resolve
// relative to packages/kennel-island and fail in a clean desktop checkout.
const frontendDependencies = [
	"react",
	"react-dom",
	"motion",
	"@fontsource-variable/geist",
];

// The Island remains a shared production renderer and a browser-only visual
// lab in packages/kennel-island. Forge owns only its output directory, keeping
// the detailed Island UI and its renderer tests in one canonical source tree.
export default defineConfig({
	root: islandRoot,
	publicDir: path.join(islandRoot, "public"),
	resolve: {
		alias: frontendDependencies.map((dependency) => ({
			find: dependency,
			replacement: path.join(frontendRoot, "node_modules", dependency),
		})),
	},
	server: {
		fs: {
			allow: [islandRoot, path.join(frontendRoot, "node_modules")],
		},
	},
	plugins: [react()],
	build: {
		outDir: path.join(frontendRoot, ".vite/renderer/island_window"),
		emptyOutDir: true,
	},
});
