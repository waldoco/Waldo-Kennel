import type { BaseWindow, WebContents } from "electron";
import { describe, expect, it, vi } from "vitest";
import { createAttemptReplacementHandler, type OwnerCommandHandlerDeps } from "./owner-command-handler";

const command = { outcomeId: "out", predecessorAttemptId: "a", planRevisionId: "p", workUnitId: "w", runIntentGeneration: 1, contractRevisionNumber: 1, action: "replace", requestKey: "rk" };
function harness() {
	const frame = {} as never;
	const shell = { isDestroyed: vi.fn(() => false), mainFrame: frame } as unknown as WebContents;
	const window = { isDestroyed: vi.fn(() => false) } as unknown as BaseWindow;
	const response = { ok: true, status: 201, json: vi.fn(async () => ({ data: { created: true } })) } as never;
	const deps: OwnerCommandHandlerDeps = {
		getWindow: vi.fn(() => window), getShellWebContents: vi.fn(() => shell),
		showConfirmation: vi.fn(async () => ({ response: 0 })), getDaemonConnection: vi.fn(() => ({ port: 3031 })),
		ownerCommandToken: "secret", fetch: vi.fn(async () => response) as never,
	};
	return { deps, shell, frame, window, event: { sender: shell, senderFrame: frame }, handler: createAttemptReplacementHandler(deps) };
}
describe("owner command IPC authority", () => {
	it("sends after primary-shell confirmation", async () => { const h = harness(); await expect(h.handler(h.event, command)).resolves.toEqual({ data: { created: true } }); expect(h.deps.fetch).toHaveBeenCalledOnce(); });
	it("rejects a subframe", async () => { const h = harness(); await expect(h.handler({ ...h.event, senderFrame: {} as never }, command)).rejects.toThrow(/primary/); });
	it("rejects secondary shell", async () => { const h = harness(); await expect(h.handler({ ...h.event, sender: {} as never }, command)).rejects.toThrow(/primary/); });
	it("rejects absent or destroyed shell", async () => { const h = harness(); vi.mocked(h.deps.getShellWebContents).mockReturnValue(null); await expect(h.handler(h.event, command)).rejects.toThrow(/primary/); const j = harness(); vi.mocked(j.shell.isDestroyed).mockReturnValue(true); await expect(j.handler(j.event, command)).rejects.toThrow(/primary/); });
	it("cancels without HTTP", async () => { const h = harness(); vi.mocked(h.deps.showConfirmation).mockResolvedValue({ response: 1 }); await expect(h.handler(h.event, command)).resolves.toEqual({ cancelled: true }); expect(h.deps.fetch).not.toHaveBeenCalled(); });
	it("rejects shell replacement while dialog awaits", async () => { const h = harness(); vi.mocked(h.deps.showConfirmation).mockImplementation(async () => { vi.mocked(h.deps.getShellWebContents).mockReturnValue({ isDestroyed: () => false, mainFrame: {} } as never); return { response: 0 }; }); await expect(h.handler(h.event, command)).rejects.toThrow(/primary/); expect(h.deps.fetch).not.toHaveBeenCalled(); });
	it("rejects navigation/frame replacement while dialog awaits", async () => { const h = harness(); vi.mocked(h.deps.showConfirmation).mockImplementation(async () => { (h.shell as { mainFrame: unknown }).mainFrame = {}; return { response: 0 }; }); await expect(h.handler(h.event, command)).rejects.toThrow(/primary/); });
	it("rejects destroyed or replaced window after dialog", async () => { const h = harness(); vi.mocked(h.deps.showConfirmation).mockImplementation(async () => { vi.mocked(h.window.isDestroyed).mockReturnValue(true); return { response: 0 }; }); await expect(h.handler(h.event, command)).rejects.toThrow(); const j = harness(); vi.mocked(j.deps.showConfirmation).mockImplementation(async () => { vi.mocked(j.deps.getWindow).mockReturnValue({ isDestroyed: () => false } as never); return { response: 0 }; }); await expect(j.handler(j.event, command)).rejects.toThrow(/changed/); });
});
