import type { BaseWindow, WebContents } from "electron";
import { parseHarnessAuthorityProposal } from "./harness-authority-command";
type Event = { sender: WebContents; senderFrame: WebContents["mainFrame"] };
type Deps = {
  getWindow: () => BaseWindow | null;
  getShellWebContents: () => WebContents | null;
  showConfirmation: (
    w: BaseWindow,
    c: NonNullable<ReturnType<typeof parseHarnessAuthorityProposal>>,
  ) => Promise<{ response: number }>;
  getDaemonConnection: () => { port: number } | null;
  ownerCommandToken: string;
  fetch: typeof globalThis.fetch;
};
export function createHarnessAuthorityHandler(d: Deps) {
  return async (e: Event, input: unknown) => {
    const shell = d.getShellWebContents();
    if (
      !shell ||
      shell.isDestroyed() ||
      e.sender !== shell ||
      e.senderFrame !== shell.mainFrame
    )
      throw Error(
        "Harness authority command must come from the live primary Kennel shell main frame",
      );
    const w = d.getWindow();
    if (!w || w.isDestroyed()) throw Error("Kennel window is unavailable");
    const c = parseHarnessAuthorityProposal(input);
    if (!c) throw Error("Harness authority command is invalid");
    if ((await d.showConfirmation(w, c)).response !== 0)
      return { cancelled: true };
    if (
      d.getShellWebContents() !== shell ||
      shell.isDestroyed() ||
      e.senderFrame !== shell.mainFrame ||
      d.getWindow() !== w ||
      w.isDestroyed()
    )
      throw Error("Kennel window changed while approval was open");
    const daemon = d.getDaemonConnection();
    if (!daemon) throw Error("Kennel daemon is not ready");
    const path =
      c.action === "revoke"
        ? `harness-connections/${encodeURIComponent(c.connectionId)}/revoke`
        : `harness-pairing-intents/${encodeURIComponent(c.intentId)}/${c.action}`;
    const { action, ...body } = c;
    const response = await d.fetch(
      `http://127.0.0.1:${daemon.port}/internal/owner-commands/${path}`,
      {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
          Authorization: `KennelOwner ${d.ownerCommandToken}`,
        },
        body: JSON.stringify(body),
      },
    );
    const payload = await response.json();
    if (!response.ok)
      throw Error(`Harness authority command rejected (${response.status})`);
    return payload;
  };
}
