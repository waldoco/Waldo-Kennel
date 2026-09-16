import type { ElectronApplication } from "@playwright/test";
import type { Dialog } from "electron";

/**
 * Stubs the Electron main process's native folder picker to return a fixed
 * path, simulating the owner's OWN choice of folder — not a shortcut around
 * the product. Every action after this still goes through the real
 * "choose folder" button, the real IPC round trip, and the real
 * `POST /api/v1/projects` call (plan §0.6): Playwright cannot drive a native
 * macOS Open panel at all, so this is the standard, narrowly-scoped
 * substitution for what that panel would have returned.
 */
export async function stubFolderPicker(app: ElectronApplication, path: string): Promise<void> {
	await app.evaluate(({ dialog }: { dialog: Dialog }, folderPath: string) => {
		dialog.showOpenDialog = (async () => ({ canceled: false, filePaths: [folderPath] })) as typeof dialog.showOpenDialog;
	}, path);
}
