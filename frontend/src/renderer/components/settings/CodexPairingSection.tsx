import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import type {
  CodexDiscoveryState,
  CodexPairingState,
} from "../../../shared/provider-pairing";
import { Button } from "../ui/button";
import { SettingsSection } from "./SettingsSection";

export function CodexPairingSection({ projectId }: { projectId: string }) {
  const { t } = useTranslation();
  const [discovery, setDiscovery] = useState<CodexDiscoveryState | null>(null);
  const [pairing, setPairing] = useState<CodexPairingState | null>(null);
  const [busy, setBusy] = useState(false);
  const [refreshing, setRefreshing] = useState(false);
  const operationEpoch = useRef(0);
  const bridge = window.kennel?.app;
  const statusMessage = (
    found: CodexDiscoveryState | null,
    current: CodexPairingState | null,
  ) => {
    if (current?.state === "connected")
      return t("settings.pairing.status.connected", {
        generation:
          current.generation ??
          t("settings.pairing.status.generationUnknown"),
      });
    if (current?.state === "pairing")
      return t("settings.pairing.status.proving");
    if (current?.state === "awaiting_confirmation")
      return t("settings.pairing.status.awaitingConfirmation");
    if (current?.state === "action_needed")
      return (
        current.message ??
        (current.reason
          ? t("settings.pairing.status.needsAttentionWithReason", {
              reason: current.reason,
            })
          : t("settings.pairing.status.needsAttention"))
      );
    if (current?.state === "error")
      return current.message ?? t("settings.pairing.status.failed");
    if (found?.state === "installed")
      return t("settings.pairing.status.installed", {
        version: found.version,
      });
    if (found) return found.message;
    return t("settings.pairing.status.checking");
  };
  const refresh = async () => {
    if (!bridge?.discoverCodex || !bridge.getCodexPairing) {
      throw new Error(t("settings.pairing.status.unavailable"));
    }
    const [found, current] = await Promise.all([
      bridge.discoverCodex({ projectId }),
      bridge.getCodexPairing({ projectId }),
    ]);
    return { found, current };
  };
  useEffect(() => {
    const epoch = ++operationEpoch.current;
    setDiscovery(null);
    setPairing(null);
    setBusy(false);
    setRefreshing(false);
    void refresh()
      .then((next) => {
        if (operationEpoch.current !== epoch) return;
        setDiscovery(next.found);
        setPairing(next.current);
      })
      .catch((error) => {
        if (operationEpoch.current !== epoch) return;
        setDiscovery({
          state: "error",
          message:
            error instanceof Error
              ? error.message
              : t("settings.pairing.status.discoveryFailed"),
        });
        setPairing({
          state: "error",
          message: t("settings.pairing.status.stateLoadFailed"),
        });
      });
    return () => {
      operationEpoch.current += 1;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [projectId]);
  const retryRefresh = async () => {
    const epoch = ++operationEpoch.current;
    setRefreshing(true);
    try {
      const next = await refresh();
      if (operationEpoch.current !== epoch) return;
      setDiscovery(next.found);
      setPairing(next.current);
    } catch (error) {
      if (operationEpoch.current !== epoch) return;
      setDiscovery({
        state: "error",
        message:
          error instanceof Error
            ? error.message
            : t("settings.pairing.status.discoveryFailed"),
      });
    } finally {
      if (operationEpoch.current === epoch) setRefreshing(false);
    }
  };
  const pair = async () => {
    if (!bridge?.pairCodex || discovery?.state !== "installed") return;
    const epoch = ++operationEpoch.current;
    setBusy(true);
    setPairing({ state: "awaiting_confirmation" });
    try {
      const next = await bridge.pairCodex({
        projectId,
        requestKey: crypto.randomUUID(),
      });
      if (operationEpoch.current !== epoch) return;
      setPairing(next);
    } catch (error) {
      if (operationEpoch.current !== epoch) return;
      setPairing({
        state: "error",
        message:
          error instanceof Error
            ? error.message
            : t("settings.pairing.status.failed"),
      });
    } finally {
      if (operationEpoch.current === epoch) setBusy(false);
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
      ? t("settings.pairing.action.reconnect")
      : pairing?.repair === "retry_pairing"
        ? t("settings.pairing.action.retry")
        : t("settings.pairing.action.pair");
  return (
    <SettingsSection title={t("settings.pairing.title")} grouped>
      <div
        data-testid="codex-pairing"
        className="flex items-center justify-between gap-4 p-4"
      >
        <div>
          <p className="text-sm font-medium">{t("settings.pairing.name")}</p>
          <p role="status" className="mt-1 text-xs text-muted-foreground">
            {statusMessage(discovery, pairing)}
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
              {t("settings.pairing.action.refresh")}
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
