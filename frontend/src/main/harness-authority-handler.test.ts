import { describe, expect, it, vi } from "vitest";
import { createHarnessAuthorityHandler } from "./harness-authority-handler";
const digest = "a".repeat(64),
  command = {
    action: "approve" as const,
    intentId: "i",
    digest,
    requestKey: "k",
  };
function h() {
  const frame = {} as never,
    shell = { isDestroyed: vi.fn(() => false), mainFrame: frame } as never,
    w = { isDestroyed: vi.fn(() => false) } as never;
  const deps = {
    getWindow: vi.fn(() => w),
    getShellWebContents: vi.fn(() => shell),
    showConfirmation: vi.fn(async () => ({ response: 0 })),
    getDaemonConnection: vi.fn(() => ({ port: 3031 })),
    ownerCommandToken: "secret",
    fetch: vi.fn(async () => ({
      ok: true,
      status: 200,
      json: async () => ({ data: { changed: true } }),
    })) as never,
  };
  return {
    deps,
    shell,
    frame,
    w,
    event: { sender: shell, senderFrame: frame },
    handler: createHarnessAuthorityHandler(deps),
  };
}
describe("harness authority IPC", () => {
  it("confirms then sends exact authenticated command", async () => {
    const x = h();
    await expect(x.handler(x.event, command)).resolves.toEqual({
      data: { changed: true },
    });
    expect(x.deps.fetch).toHaveBeenCalledWith(
      expect.stringContaining("/harness-pairing-intents/i/approve"),
      expect.objectContaining({
        headers: expect.objectContaining({
          Authorization: "KennelOwner secret",
        }),
      }),
    );
  });
  it("cancels without HTTP", async () => {
    const x = h();
    x.deps.showConfirmation.mockResolvedValue({ response: 1 });
    await expect(x.handler(x.event, command)).resolves.toEqual({
      cancelled: true,
    });
    expect(x.deps.fetch).not.toHaveBeenCalled();
  });
  it("rejects subframes and changed windows", async () => {
    const x = h();
    await expect(
      x.handler({ ...x.event, senderFrame: {} as never }, command),
    ).rejects.toThrow(/primary/);
    const y = h();
    y.deps.showConfirmation.mockImplementation(async () => {
      y.deps.getWindow.mockReturnValue({ isDestroyed: () => false } as never);
      return { response: 0 };
    });
    await expect(y.handler(y.event, command)).rejects.toThrow(/changed/);
  });
});
