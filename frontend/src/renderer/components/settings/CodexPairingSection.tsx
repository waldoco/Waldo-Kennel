import { useEffect, useState } from "react";
import type {
  CodexDiscoveryState,
  CodexPairingState,
} from "../../../shared/provider-pairing";
import { Button } from "../ui/button";
import { SettingsSection } from "./SettingsSection";

function message(
  discovery: CodexDiscoveryState | null,
  pairing: CodexPairingState | null,
) {
  if (pairing?.state === "connected")
    return `Connected · generation ${pairing.generation ?? "unknown"}`;
  if (pairing?.state === "pairing")
    return "Proving the approved Codex connection…";
  if (pairing?.state === "awaiting_confirmation")
    return "Waiting for desktop confirmation";
  if (pairing?.state === "action_needed")
    return (
      pairing.message ??
      `Connection needs attention${pairing.reason ? `: ${pairing.reason}` : ""}`
    );
  if (pairing?.state === "error")
    return (
      pairing.message ??
      "Pairing failed. Your previous connection was not changed."
    );
  if (discovery?.state === "installed")
    return `Codex ${discovery.version} is installed`;
  if (discovery) return discovery.message;
  return "Checking this project for an installed Codex provider…";
}

export function CodexPairingSection({ projectId }: { projectId: string }) {
  const [discovery, setDiscovery] = useState<CodexDiscoveryState | null>(null);
  const [pairing, setPairing] = useState<CodexPairingState | null>(null);
  const [busy, setBusy] = useState(false);
  const bridge = window.kennel?.app;
  const refresh = async () => {
    if (!bridge?.discoverCodex || !bridge.getCodexPairing) {
      setDiscovery({
        state: "error",
        message: "Provider pairing is unavailable in this build.",
      });
      return;
    }
    const [found, current] = await Promise.all([
      bridge.discoverCodex({ projectId }),
      bridge.getCodexPairing({ projectId }),
    ]);
    setDiscovery(found);
    setPairing(current);
  };
  useEffect(() => {
    void refresh().catch((error) =>
      setDiscovery({
        state: "error",
        message:
          error instanceof Error ? error.message : "Provider discovery failed",
      }),
    );
  }, [projectId]);
  const pair = async () => {
    if (!bridge?.pairCodex || discovery?.state !== "installed") return;
    setBusy(true);
    setPairing({ state: "awaiting_confirmation" });
    try {
      const next = await bridge.pairCodex({
        projectId,
        installationId: discovery.installationId,
        requestKey: crypto.randomUUID(),
      });
      setPairing(next);
    } catch (error) {
      setPairing({
        state: "error",
        message:
          error instanceof Error
            ? error.message
            : "Pairing failed. Your previous connection was not changed.",
      });
    } finally {
      setBusy(false);
    }
  };
  const canPair =
    discovery?.state === "installed" &&
    pairing?.state !== "pairing" &&
    pairing?.state !== "awaiting_confirmation";
  return (
    <SettingsSection title="Codex provider" grouped>
      <div
        data-testid="codex-pairing"
        className="flex items-center justify-between gap-4 p-4"
      >
        <div>
          <p className="text-sm font-medium">Codex</p>
          <p role="status" className="mt-1 text-xs text-muted-foreground">
            {message(discovery, pairing)}
          </p>
        </div>
        <Button
          type="button"
          disabled={!canPair || busy}
          onClick={() => void pair()}
        >
          {pairing?.state === "connected" ? "Reconnect" : "Pair Codex"}
        </Button>
      </div>
    </SettingsSection>
  );
}
