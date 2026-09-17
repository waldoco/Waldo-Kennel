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
  const [refreshing, setRefreshing] = useState(false);
  const bridge = window.kennel?.app;
  const refresh = async () => {
    if (!bridge?.discoverCodex || !bridge.getCodexPairing) {
      throw new Error("Provider pairing is unavailable in this build.");
    }
    const [found, current] = await Promise.all([
      bridge.discoverCodex({ projectId }),
      bridge.getCodexPairing({ projectId }),
    ]);
    return { found, current };
  };
  useEffect(() => {
    let current = true;
    setDiscovery(null);
    setPairing(null);
    void refresh()
      .then((next) => {
        if (!current) return;
        setDiscovery(next.found);
        setPairing(next.current);
      })
      .catch((error) => {
        if (!current) return;
        setDiscovery({
          state: "error",
          message:
            error instanceof Error
              ? error.message
              : "Provider discovery failed",
        });
        setPairing({
          state: "error",
          message: "Pairing state could not be loaded.",
        });
      });
    return () => {
      current = false;
    };
  }, [projectId]);
  const retryRefresh = async () => {
    setRefreshing(true);
    try {
      const next = await refresh();
      setDiscovery(next.found);
      setPairing(next.current);
    } catch (error) {
      setDiscovery({
        state: "error",
        message:
          error instanceof Error ? error.message : "Provider discovery failed",
      });
    } finally {
      setRefreshing(false);
    }
  };
  const pair = async () => {
    if (!bridge?.pairCodex || discovery?.state !== "installed") return;
    setBusy(true);
    setPairing({ state: "awaiting_confirmation" });
    try {
      const next = await bridge.pairCodex({
        projectId,
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
  const needsRefresh =
    discovery?.state === "error" || pairing?.state === "error";
  const pairLabel =
    pairing?.state === "connected" || pairing?.repair === "reconnect_adapter"
      ? "Reconnect"
      : pairing?.repair === "retry_pairing"
        ? "Retry pairing"
        : "Pair Codex";
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
        <div className="flex shrink-0 gap-2">
          {needsRefresh ? (
            <Button
              type="button"
              disabled={refreshing}
              onClick={() => void retryRefresh()}
              variant="ghost"
            >
              Refresh status
            </Button>
          ) : null}
          <Button
            type="button"
            aria-busy={busy}
            disabled={!canPair || busy || refreshing}
            onClick={() => void pair()}
          >
            {pairLabel}
          </Button>
        </div>
      </div>
    </SettingsSection>
  );
}
