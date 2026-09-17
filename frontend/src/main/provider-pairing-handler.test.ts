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

describe("Codex pairing orchestration", () => {
  it("confirms exact intent and keeps challenge secret and bearer out of IPC", async () => {
    const { createCodexPairingHandler } =
      await import("./provider-pairing-handler");
    const intent = {
      id: "intent",
      digest: "d".repeat(64),
      kind: "rotate" as const,
      connectionId: "hc",
      installationId: "install",
      adapterDigest: "a".repeat(64),
      harnessIdentity: "codex",
      providerVersion: "1",
      protocolFingerprint: "p".repeat(64),
      missionId: "rsp-1",
      capabilityClasses: ["turn"],
      expectedGeneration: 2,
    };
    const calls: object[] = [];
    const fetchMock = vi.fn(
      async (url: string) =>
        new Response(
          JSON.stringify(
            url.includes("codex-pairing-proposals")
              ? { data: { intent } }
              : { data: {} },
          ),
          { status: url.includes("codex-pairing-proposals") ? 201 : 200 },
        ),
    );
    const exchange = vi.fn(
      async (_address: string, frame: Record<string, unknown>) => {
        calls.push(frame);
        return frame.type === "request_challenge"
          ? { ok: true, challenge_id: "challenge", secret: "PRIVATE_SECRET" }
          : { ok: true, bearer: "PRIVATE_BEARER" };
      },
    );
    const window = { isDestroyed: () => false };
    const handler = createCodexPairingHandler({
      getShellWebContents: () => shell as never,
      getWindow: () => window as never,
      showConfirmation: async () => ({ response: 0 }),
      getDaemonConnection: () => ({ port: 9 }),
      getPairingAddress: () => "/tmp/pair.sock",
      fetch: fetchMock as never,
      path: () => "",
      ownerCommandToken: "owner",
      appRunId: "run-new",
      exchange,
    });
    const result = await handler(event, {
      projectId: "project-1",
      installationId: "renderer-ignored",
      requestKey: "request",
    });
    expect(result).toEqual({
      state: "connected",
      connectionId: "hc",
      generation: 2,
    });
    expect(JSON.stringify(result)).not.toMatch(/PRIVATE_/);
    expect(calls[1]).toEqual(
      expect.objectContaining({
        secret: "PRIVATE_SECRET",
        app_run_id: "run-new",
        expected_generation: 2,
      }),
    );
    expect(fetchMock).toHaveBeenCalledWith(
      expect.stringContaining("/approve"),
      expect.anything(),
    );
  });
});
