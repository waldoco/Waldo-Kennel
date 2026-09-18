import { createHash } from "node:crypto";
import { constants } from "node:fs";
import { access, realpath } from "node:fs/promises";
import path from "node:path";
import { execFile } from "node:child_process";
import { promisify } from "node:util";
import type { IpcMainInvokeEvent, WebContents } from "electron";
import type {
  CodexDiscoveryState,
  CodexPairingState,
} from "../shared/provider-pairing";

type Event = Pick<IpcMainInvokeEvent, "sender" | "senderFrame">;
type Daemon = { port: number };
type Deps = {
  getShellWebContents: () => WebContents | null;
  getDaemonConnection: () => Daemon | null;
  fetch: typeof globalThis.fetch;
  path: () => string;
  resolve?: (candidate: string) => Promise<string>;
  version?: (executable: string) => Promise<string>;
};
const run = promisify(execFile);
const safeId = (value: string) =>
  `codex-${createHash("sha256").update(value).digest("hex")}`;
const projectId = (input: unknown) =>
  typeof input === "object" &&
  input !== null &&
  typeof (input as { projectId?: unknown }).projectId === "string" &&
  /^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$/.test(
    (input as { projectId: string }).projectId,
  )
    ? (input as { projectId: string }).projectId
    : null;
const primary = (d: Deps, e: Event) => {
  const shell = d.getShellWebContents();
  if (
    !shell ||
    shell.isDestroyed() ||
    e.sender !== shell ||
    e.senderFrame !== shell.mainFrame
  )
    throw Error(
      "Provider pairing must come from the live primary Kennel shell main frame",
    );
};
const defaultResolve = async (candidate: string) => {
  const canonical = await realpath(candidate);
  await access(canonical, constants.X_OK);
  return canonical;
};
const defaultVersion = async (executable: string) =>
  (await run(executable, ["--version"], { timeout: 5000, env: {} })).stdout;
export function createCodexDiscoveryHandler(d: Deps) {
  return async (e: Event, input: unknown): Promise<CodexDiscoveryState> => {
    primary(d, e);
    if (!projectId(input)) throw Error("Project is invalid");
    const resolve = d.resolve ?? defaultResolve,
      version = d.version ?? defaultVersion;
    for (const directory of d.path().split(path.delimiter).filter(Boolean)) {
      let executable: string;
      try {
        executable = await resolve(
          path.join(
            directory,
            process.platform === "win32" ? "codex.exe" : "codex",
          ),
        );
      } catch {
        continue;
      }
      try {
        const output = (await version(executable)).trim();
        const match = output.match(
          /(?:codex(?:-cli)?\s+)?(\d+\.\d+\.\d+(?:[-+][\w.-]+)?)/i,
        );
        if (!match)
          return {
            state: "incompatible",
            message: "The installed Codex version could not be verified.",
          };
        return {
          state: "installed",
          installationId: safeId(executable),
          version: match[1],
          source: "path",
        };
      } catch {
        return {
          state: "incompatible",
          message: "The installed Codex version could not be checked.",
        };
      }
    }
    return {
      state: "not_found",
      message: "Codex was not found on this computer.",
    };
  };
}
const connectionBearers = new Map<
  string,
  { generation: number; bearer: string }
>();
export function readCodexConnectionBearer(
  connectionId: string,
  generation: number,
): string | null {
  const current = connectionBearers.get(connectionId);
  return current?.generation === generation ? current.bearer : null;
}
export function clearCodexConnectionBearer(connectionId: string): void {
  connectionBearers.delete(connectionId);
}
export function resetCodexConnectionBearersForTest(): void {
  connectionBearers.clear();
}
export function createCodexPairingStateHandler(d: Deps) {
  return async (e: Event, input: unknown): Promise<CodexPairingState> => {
    primary(d, e);
    const id = projectId(input);
    if (!id) throw Error("Project is invalid");
    const daemon = d.getDaemonConnection();
    if (!daemon)
      return {
        state: "action_needed",
        reason: "daemon_unavailable",
        repair: "start_daemon",
        message: "Start the Kennel daemon to pair Codex.",
      };
    const intentsResponse = await d.fetch(
      `http://127.0.0.1:${daemon.port}/api/v1/harness-pairing-intents?projectId=${encodeURIComponent(id)}&limit=20`,
    );
    if (!intentsResponse.ok)
      return {
        state: "error",
        message: "Codex pairing state could not be loaded.",
      };
    const body = (await intentsResponse.json()) as {
      data?: {
        intents?: Array<{
          id: string;
          status: string;
          proofState?: string;
          missionId?: string;
          connectionId?: string;
          expectedGeneration?: number;
        }>;
      };
    };
    const intent = body.data?.intents?.[0];
    if (!intent) return { state: "unpaired" };
    if (intent.connectionId) {
      const connectionResponse = await d.fetch(
        `http://127.0.0.1:${daemon.port}/api/v1/harness-connections/${encodeURIComponent(intent.connectionId)}`,
      );
      if (connectionResponse.ok) {
        const connectionBody = (await connectionResponse.json()) as {
          data?: { connection?: { generation?: number; state?: string } };
        };
        const connection = connectionBody.data?.connection;
        if (
          connection &&
          (connection.state !== "connected" ||
            connection.generation !== intent.expectedGeneration)
        )
          clearCodexConnectionBearer(intent.connectionId);
      }
    }
    if (intent.status === "requested")
      return { state: "awaiting_confirmation" };
    if (intent.status === "approved")
      return {
        state: "action_needed",
        reason: "pairing_activation_incomplete",
        repair: "retry_pairing",
        message:
          "Pairing was approved but did not complete. Retry to keep the previous connection unchanged.",
      };
    if (
      (intent.status === "activating" ||
        intent.status === "challenge_active") &&
      intent.proofState !== "succeeded"
    )
      return { state: "pairing" };
    if (
      intent.status === "activation_failed" ||
      intent.status === "denied" ||
      intent.status === "superseded" ||
      intent.status === "expired" ||
      intent.proofState === "failed" ||
      intent.proofState === "expired" ||
      intent.proofState === "superseded"
    )
      return {
        state: "error",
        message:
          "Pairing did not complete. Your previous connection was not changed.",
      };
    if (intent.connectionId && intent.proofState === "succeeded") {
      const generation = intent.expectedGeneration ?? 0;
      if (!readCodexConnectionBearer(intent.connectionId, generation))
        return {
          state: "action_needed",
          reason: "connection_bearer_unavailable",
          repair: "reconnect_adapter",
          message: "Reconnect Codex for this app session.",
        };
      return {
        state: "connected",
        connectionId: intent.connectionId,
        generation,
      };
    }
    return { state: "unpaired" };
  };
}

type PairingIntent = {
  id: string;
  digest: string;
  kind: "pair" | "rotate";
  connectionId: string;
  installationId: string;
  adapterDigest: string;
  harnessIdentity: string;
  providerVersion: string;
  protocolFingerprint: string;
  missionId: string;
  capabilityClasses: string[];
  expectedGeneration: number;
};
type PairingDeps = Deps & {
  getWindow: () => import("electron").BaseWindow | null;
  showConfirmation: (
    window: import("electron").BaseWindow,
    intent: PairingIntent,
  ) => Promise<{ response: number }>;
  ownerCommandToken: string;
  appRunId: string;
  getPairingAddress: () => string | null;
  exchange: (
    address: string,
    frame: Record<string, unknown>,
  ) => Promise<Record<string, unknown>>;
};
function ownerHeaders(d: PairingDeps) {
  return {
    "Content-Type": "application/json",
    Authorization: `KennelOwner ${d.ownerCommandToken}`,
  };
}
async function ownerPost(
  d: PairingDeps,
  daemon: Daemon,
  path: string,
  body: object,
) {
  const response = await d.fetch(
    `http://127.0.0.1:${daemon.port}/internal/owner-commands/${path}`,
    { method: "POST", headers: ownerHeaders(d), body: JSON.stringify(body) },
  );
  if (!response.ok)
    throw Error(`Codex pairing command rejected (${response.status})`);
  return response.json() as Promise<Record<string, unknown>>;
}
export function createCodexPairingHandler(d: PairingDeps) {
  return async (e: Event, input: unknown): Promise<CodexPairingState> => {
    primary(d, e);
    const id = projectId(input);
    const requestKey =
      typeof input === "object" &&
      input !== null &&
      typeof (input as { requestKey?: unknown }).requestKey === "string"
        ? (input as { requestKey: string }).requestKey
        : "";
    if (!id || !requestKey) throw Error("Codex pairing proposal is invalid");
    const daemon = d.getDaemonConnection(),
      window = d.getWindow();
    if (!daemon || !window || window.isDestroyed())
      throw Error("Kennel is not ready to pair Codex");
    const proposal = await ownerPost(d, daemon, "codex-pairing-proposals", {
      projectId: id,
      requestKey,
    });
    const intent = (proposal.data as { intent?: PairingIntent } | undefined)
      ?.intent;
    if (!intent || intent.harnessIdentity !== "codex")
      throw Error("Codex pairing proposal response is invalid");
    if ((await d.showConfirmation(window, intent)).response !== 0) {
      await ownerPost(
        d,
        daemon,
        `harness-pairing-intents/${encodeURIComponent(intent.id)}/deny`,
        { digest: intent.digest, requestKey: `${requestKey}:deny` },
      );
      return { state: "unpaired" };
    }
    if (
      d.getShellWebContents() !== e.sender ||
      e.sender.isDestroyed() ||
      e.senderFrame !== e.sender.mainFrame ||
      d.getWindow() !== window ||
      window.isDestroyed()
    )
      throw Error("Kennel window changed while approval was open");
    await ownerPost(
      d,
      daemon,
      `harness-pairing-intents/${encodeURIComponent(intent.id)}/approve`,
      { digest: intent.digest, requestKey: `${requestKey}:approve` },
    );
    const address = d.getPairingAddress();
    if (!address) throw Error("Codex pairing transport is unavailable");
    const issued = await d.exchange(address, {
      type: "request_challenge",
      intent_id: intent.id,
      intent_digest: intent.digest,
    });
    if (
      issued.ok !== true ||
      typeof issued.challenge_id !== "string" ||
      typeof issued.secret !== "string"
    )
      throw Error("Codex pairing challenge failed");
    const proved = await d.exchange(address, {
      type: "prove",
      challenge_id: issued.challenge_id,
      secret: issued.secret,
      connection_id: intent.connectionId,
      installation_id: intent.installationId,
      adapter_digest: intent.adapterDigest,
      harness_identity: intent.harnessIdentity,
      provider_version: intent.providerVersion,
      protocol_fingerprint: intent.protocolFingerprint,
      mission_id: intent.missionId,
      app_run_id: d.appRunId,
      capability_classes: intent.capabilityClasses,
      expected_generation: intent.expectedGeneration,
    });
    if (
      proved.ok !== true ||
      typeof proved.bearer !== "string" ||
      !proved.bearer
    )
      throw Error(
        "Codex pairing proof failed. Your previous connection was not changed.",
      );
    // The transport bearer stays in main custody. Product command ingress can
    // consume it in this process; it is never returned over IPC.
    connectionBearers.set(intent.connectionId, {
      generation: intent.expectedGeneration,
      bearer: proved.bearer,
    });
    return {
      state: "connected",
      connectionId: intent.connectionId,
      generation: intent.expectedGeneration,
    };
  };
}
