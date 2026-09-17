import type { BaseWindow, WebContents } from "electron";
import { parseAttemptReplacementProposal } from "./owner-command";

type SenderEvent = {
  sender: WebContents;
  senderFrame: WebContents["mainFrame"];
};
type DialogResult = { response: number };
type DaemonConnection = { port: number };

export type OwnerCommandHandlerDeps = {
  getWindow: () => BaseWindow | null;
  getShellWebContents: () => WebContents | null;
  showConfirmation: (
    window: BaseWindow,
    command: ReturnType<typeof parseAttemptReplacementProposal> & {},
  ) => Promise<DialogResult>;
  getDaemonConnection: () => DaemonConnection | null;
  ownerCommandToken: string;
  fetch: typeof globalThis.fetch;
};

function ownedShell(
  deps: OwnerCommandHandlerDeps,
  event: SenderEvent,
): WebContents {
  const shell = deps.getShellWebContents();
  if (
    !shell ||
    shell.isDestroyed() ||
    event.sender !== shell ||
    event.senderFrame !== shell.mainFrame
  ) {
    throw new Error(
      "Owner command must come from the live primary Kennel shell main frame",
    );
  }
  return shell;
}

export function createAttemptReplacementHandler(deps: OwnerCommandHandlerDeps) {
  return async (event: SenderEvent, input: unknown): Promise<unknown> => {
    const shell = ownedShell(deps, event);
    const window = deps.getWindow();
    if (!window || window.isDestroyed())
      throw new Error("Kennel window is unavailable");
    const command = parseAttemptReplacementProposal(input);
    if (!command) throw new Error("Replacement command is invalid");
    const confirmation = await deps.showConfirmation(window, command);
    if (confirmation.response !== 0) return { cancelled: true };
    // Approval belongs only to the exact shell/frame that requested it. A reload,
    // replacement, close, or navigation while the native prompt is open invalidates it.
    const liveShell = ownedShell(deps, event);
    if (
      liveShell !== shell ||
      deps.getWindow() !== window ||
      window.isDestroyed()
    ) {
      throw new Error("Kennel window changed while approval was open");
    }
    const daemon = deps.getDaemonConnection();
    if (!daemon) throw new Error("Kennel daemon is not ready");
    const response = await deps.fetch(
      `http://127.0.0.1:${daemon.port}/internal/owner-commands/attempt-replacement-decisions`,
      {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
          Authorization: `KennelOwner ${deps.ownerCommandToken}`,
        },
        body: JSON.stringify(command),
      },
    );
    const payload = await response.json();
    if (!response.ok)
      throw new Error(`Owner command rejected (${response.status})`);
    return payload;
  };
}
