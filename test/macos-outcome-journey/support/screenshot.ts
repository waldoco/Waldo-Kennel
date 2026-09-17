import { execFile } from "node:child_process";
import { createHash } from "node:crypto";
import { mkdir, readFile } from "node:fs/promises";
import { dirname, join } from "node:path";
import { promisify } from "node:util";
import type { ElectronApplication, Page } from "@playwright/test";

const execFileAsync = promisify(execFile);

export interface CapturedArtifact {
	path: string;
	sha256: string;
}

/**
 * Captures one named checkpoint two ways (plan §7.2):
 *  - the renderer's own web contents (page.screenshot), taken at every step;
 *  - the real macOS window frame via `screencapture -R <bounds>`, taken only
 *    at the checkpoints the plan names as needing full native-window proof
 *    (clean entry, approve-before-commit, the execution boundary) — this is
 *    what actually proves the pixels came from the packaged app's own window,
 *    not just its DOM.
 *
 * `screencapture -R` takes a screen-relative region rather than a window ID:
 * Electron's BrowserWindow.getBounds() is trivially readable over
 * app.evaluate(), while resolving a CGWindowID from Node is not, so bounds+
 * region is the simpler and equally valid proof.
 */
export async function captureCheckpoint(
	page: Page,
	evidenceDir: string,
	name: string,
	{ app, native = false }: { app?: ElectronApplication; native?: boolean } = {},
): Promise<CapturedArtifact[]> {
	const artifacts: CapturedArtifact[] = [];
	const rendererPath = join(evidenceDir, "screenshots", `${name}.png`);
	await mkdir(dirname(rendererPath), { recursive: true });
	await page.screenshot({ path: rendererPath });
	artifacts.push({ path: rendererPath, sha256: await hashFile(rendererPath) });

	if (native && app) {
		const bounds = await app.evaluate(({ BrowserWindow }: typeof import("electron")) => {
			const win = BrowserWindow.getAllWindows()[0];
			return win ? win.getBounds() : null;
		});
		if (bounds) {
			const nativePath = join(evidenceDir, "screenshots", `${name}-native.png`);
			const region = `${bounds.x},${bounds.y},${bounds.width},${bounds.height}`;
			await execFileAsync("screencapture", ["-x", "-R", region, nativePath]);
			artifacts.push({ path: nativePath, sha256: await hashFile(nativePath) });
		}
	}
	return artifacts;
}

async function hashFile(path: string): Promise<string> {
	return createHash("sha256").update(await readFile(path)).digest("hex");
}
