import { describe, expect, it, vi } from "vitest";
import {
  createCodexDiscoveryHandler,
  createCodexPairingStateHandler,
} from "./provider-pairing-handler";
const shell = { isDestroyed: () => false, mainFrame: {} };
const event = { sender: shell, senderFrame: shell.mainFrame } as never;
describe("Codex provider pairing main handlers", () => {
  it("returns redacted discovery without executable paths", async () => {
    const handler = createCodexDiscoveryHandler({
      getShellWebContents: () => shell as never,
      getDaemonConnection: () => null,
      fetch,
      path: () => "/bin",
      resolve: async () => "/private/alice/bin/codex",
      version: async () => "codex-cli 1.2.3",
    });
    const result = await handler(event, { projectId: "p" });
    expect(result).toEqual(
      expect.objectContaining({
        state: "installed",
        version: "1.2.3",
        source: "path",
      }),
    );
    expect(JSON.stringify(result)).not.toContain("/private/");
  });
  it("does not misreport a broken installed Codex as absent", async () => {
    const handler = createCodexDiscoveryHandler({
      getShellWebContents: () => shell as never,
      getDaemonConnection: () => null,
      fetch,
      path: () => "/bin",
      resolve: async () => "/bin/codex",
      version: async () => {
        throw new Error("probe failed");
      },
    });
    await expect(handler(event, { projectId: "p" })).resolves.toEqual({
      state: "incompatible",
      message: expect.any(String),
    });
  });
  it("fails closed outside the primary frame", async () => {
    const handler = createCodexDiscoveryHandler({
      getShellWebContents: () => shell as never,
      getDaemonConnection: () => null,
      fetch,
      path: () => "",
    });
    await expect(
      handler(
        { sender: {} as never, senderFrame: {} as never },
        { projectId: "p" },
      ),
    ).rejects.toThrow(/primary/);
  });
  it("projects pending intent without secret fields", async () => {
    const handler = createCodexPairingStateHandler({
      getShellWebContents: () => shell as never,
      getDaemonConnection: () => ({ port: 9 }),
      path: () => "",
      fetch: vi.fn(
        async () =>
          new Response(
            JSON.stringify({
              data: {
                intents: [
                  { id: "i", status: "requested", secret: "NO", bearer: "NO" },
                ],
              },
            }),
            { status: 200 },
          ),
      ) as never,
    });
    expect(await handler(event, { projectId: "p" })).toEqual({
      state: "awaiting_confirmation",
    });
  });
});
