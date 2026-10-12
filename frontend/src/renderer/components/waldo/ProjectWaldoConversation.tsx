import { AlertTriangle, ArrowUp, ArrowUpRight, CheckCircle2, ChevronDown, Clock, Link2, Link2Off } from "lucide-react";
import { useCallback, useEffect, useMemo, useState, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import type { components } from "../../../api/schema";
import {
  apiClient,
  apiErrorCode,
  apiErrorMessage,
  apiErrorRequestId,
} from "../../lib/api-client";
import { cn } from "../../lib/utils";
import { Button } from "../ui/button";

type Snapshot = components["schemas"]["WaldoConversationSnapshotResponse"];
type ContextAttachment = components["schemas"]["WaldoContextAttachmentResponse"];

type ProjectWaldoConversationProps = {
  daemonReady: boolean;
  onOpenHome?: () => void;
  outcomeId?: string;
  outcomeTitle?: string;
  projectId: string;
  projectName: string;
};

function snapshotKey(projectId: string) {
  return `kennel.waldo.snapshot.${projectId}`;
}

function draftKey(projectId: string, outcomeId?: string) {
  return `kennel.waldo.draft.${projectId}.${outcomeId ?? "project"}`;
}

function requestKey(prefix: string) {
  const id = globalThis.crypto?.randomUUID?.() ?? `${Date.now()}-${Math.random().toString(16).slice(2)}`;
  return `${prefix}-${id}`;
}

function readSnapshot(projectId: string): Snapshot | null {
  try {
    const raw = localStorage.getItem(snapshotKey(projectId));
    return raw ? (JSON.parse(raw) as Snapshot) : null;
  } catch {
    return null;
  }
}

function writeSnapshot(projectId: string, snapshot: Snapshot) {
  try {
    localStorage.setItem(snapshotKey(projectId), JSON.stringify(snapshot));
  } catch {
    // SQLite remains canonical. A failed renderer cache only removes offline read-back.
  }
}

type ContinuationBindingStatus = "unchanged" | "changed" | "unavailable";

function continuationBindingStatus(previous: unknown, replacement: unknown, fields: string[]): ContinuationBindingStatus {
  if (!previous || typeof previous !== "object" || !replacement || typeof replacement !== "object") return "unavailable";
  const before = previous as Record<string, unknown>;
  const after = replacement as Record<string, unknown>;
  if (fields.some((field) => before[field] === undefined || before[field] === null || before[field] === "" || after[field] === undefined || after[field] === null || after[field] === "")) return "unavailable";
  return fields.every((field) => before[field] === after[field]) ? "unchanged" : "changed";
}

export function ProjectWaldoConversation({ daemonReady, onOpenHome, outcomeId, outcomeTitle, projectId, projectName }: ProjectWaldoConversationProps) {
  const { t, i18n } = useTranslation();
  const [snapshot, setSnapshot] = useState<Snapshot | null>(() => readSnapshot(projectId));
  const [draft, setDraft] = useState(() => localStorage.getItem(draftKey(projectId, outcomeId)) ?? "");
  const [loading, setLoading] = useState(daemonReady);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [requestId, setRequestId] = useState("");
  const [notice, setNotice] = useState("");
  const [expandedReceiptIds, setExpandedReceiptIds] = useState<Set<string>>(() => new Set());

  const acceptSnapshot = useCallback((next: Snapshot) => {
    setSnapshot(next);
    writeSnapshot(projectId, next);
  }, [projectId]);

  const load = useCallback(async () => {
    if (!daemonReady) {
      setLoading(false);
      return;
    }
    setLoading(true);
    setError("");
    setRequestId("");
    const read = await apiClient.GET("/api/v1/projects/{id}/waldo-conversation", {
      params: { path: { id: projectId } },
    });
    if (read.data) {
      acceptSnapshot(read.data.waldoConversation);
      setLoading(false);
      return;
    }
    if (apiErrorCode(read.error) !== "WALDO_CONVERSATION_NOT_FOUND") {
      setError(apiErrorMessage(read.error, t("waldo.project.error.read")));
      setRequestId(apiErrorRequestId(read.error) ?? "");
      setLoading(false);
      return;
    }
    const opened = await apiClient.POST("/api/v1/projects/{id}/waldo-conversation", {
      params: { path: { id: projectId } },
    });
    if (opened.data) acceptSnapshot(opened.data.waldoConversation);
    else {
      setError(apiErrorMessage(opened.error, t("waldo.project.error.open")));
      setRequestId(apiErrorRequestId(opened.error) ?? "");
    }
    setLoading(false);
  }, [acceptSnapshot, daemonReady, projectId, t]);

  useEffect(() => {
    setSnapshot(readSnapshot(projectId));
    setDraft(localStorage.getItem(draftKey(projectId, outcomeId)) ?? "");
    setNotice("");
    void load();
  }, [load, outcomeId, projectId]);

  useEffect(() => {
    try {
      localStorage.setItem(draftKey(projectId, outcomeId), draft);
    } catch {
      // Draft persistence is best-effort renderer state, never canonical truth.
    }
  }, [draft, outcomeId, projectId]);

  const activeEpisode = useMemo(
    () => [...(snapshot?.episodes ?? [])].reverse().find((episode) => episode.state === "active"),
    [snapshot?.episodes],
  );
  const activeAttachments = useMemo(
    () => (snapshot?.contextAttachments ?? []).filter((attachment) => attachment.active),
    [snapshot?.contextAttachments],
  );
  const selectedAttachment = activeAttachments.find(
    (attachment) => attachment.ref.kind === (outcomeId ? "outcome" : "project") && attachment.ref.objectId === (outcomeId ?? projectId),
  );
  const selectedContextLabel = outcomeId ? `Outcome · ${outcomeTitle || outcomeId}` : null;

  async function ensureActiveEpisode(current: Snapshot): Promise<{ episodeId: string; snapshot: Snapshot } | null> {
    const existing = [...current.episodes].reverse().find((episode) => episode.state === "active");
    if (existing) return { episodeId: existing.id, snapshot: current };
    const opened = await apiClient.POST("/api/v1/projects/{id}/waldo-conversation/episodes", {
      params: { path: { id: projectId } },
      body: { expectedRevision: current.conversation.revision, requestKey: requestKey("waldo-episode") },
    });
    if (!opened.data) {
      setError(apiErrorMessage(opened.error, t("waldo.project.error.episode")));
      setRequestId(apiErrorRequestId(opened.error) ?? "");
      return null;
    }
    acceptSnapshot(opened.data.waldoConversation);
    const episode = [...opened.data.waldoConversation.episodes].reverse().find((item) => item.state === "active");
    return episode ? { episodeId: episode.id, snapshot: opened.data.waldoConversation } : null;
  }

  async function sendTurn() {
    const message = draft.trim();
    if (!daemonReady || !snapshot || !message || busy) return;
    setBusy(true);
    setError("");
    setNotice("");
    const episode = await ensureActiveEpisode(snapshot);
    if (!episode) {
      setBusy(false);
      return;
    }
    const response = await apiClient.POST("/api/v1/projects/{id}/waldo-conversation/turns", {
      params: { path: { id: projectId } },
      body: {
        expectedRevision: episode.snapshot.conversation.revision,
        episodeId: episode.episodeId,
        role: "user",
        message,
        contextAttachmentIds: episode.snapshot.contextAttachments.filter((attachment) => attachment.active).map((attachment) => attachment.id),
        requestKey: requestKey("waldo-turn"),
      },
    });
    if (response.data) {
      acceptSnapshot(response.data.waldoConversation);
      setDraft("");
      setNotice(t("waldo.project.saved"));
    } else {
      setError(apiErrorMessage(response.error, t("waldo.project.error.save")));
      setRequestId(apiErrorRequestId(response.error) ?? "");
      if (apiErrorCode(response.error) === "WALDO_CONVERSATION_REVISION_CONFLICT") void load();
    }
    setBusy(false);
  }

  async function attachSelectedContext() {
    if (!daemonReady || !snapshot || busy) return;
    setBusy(true);
    setError("");
    const response = await apiClient.POST("/api/v1/projects/{id}/waldo-conversation/context", {
      params: { path: { id: projectId } },
      body: {
        expectedRevision: snapshot.conversation.revision,
        ref: { kind: outcomeId ? "outcome" : "project", objectId: outcomeId ?? projectId, provenance: { kind: "user", sourceId: "waldo-rail" } },
        requestKey: requestKey("waldo-context"),
      },
    });
    if (response.data) acceptSnapshot(response.data.waldoConversation);
    else {
      setError(apiErrorMessage(response.error, t("waldo.project.error.attach")));
      setRequestId(apiErrorRequestId(response.error) ?? "");
    }
    setBusy(false);
  }

  async function detachContext(attachment: ContextAttachment) {
    if (!daemonReady || !snapshot || busy) return;
    setBusy(true);
    setError("");
    const response = await apiClient.POST("/api/v1/projects/{id}/waldo-conversation/context/{attachmentId}/detach", {
      params: { path: { id: projectId, attachmentId: attachment.id } },
      body: {
        expectedRevision: snapshot.conversation.revision,
        reason: "Detached from Waldo rail",
        requestKey: requestKey("waldo-context-detach"),
      },
    });
    if (response.data) acceptSnapshot(response.data.waldoConversation);
    else {
      setError(apiErrorMessage(response.error, t("waldo.project.error.detach")));
      setRequestId(apiErrorRequestId(response.error) ?? "");
    }
    setBusy(false);
  }

  function toggleReceiptDetails(receiptId: string) {
    setExpandedReceiptIds((current) => {
      const next = new Set(current);
      if (next.has(receiptId)) next.delete(receiptId);
      else next.add(receiptId);
      return next;
    });
  }

  const timeFormat = useMemo(() => new Intl.DateTimeFormat(i18n.language, { hour: "2-digit", minute: "2-digit" }), [i18n.language]);
  const formatTime = (value: string) => {
    const date = new Date(value);
    return Number.isNaN(date.getTime()) ? "" : timeFormat.format(date);
  };

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <div className="flex flex-col gap-2 hairline-b border-wd-hairline px-4 py-3">
        <div className="flex flex-wrap items-center gap-2">
          <span className="inline-flex h-7 items-center rounded-full bg-wd-inset px-3 wd-label text-wd-primary">{projectName}</span>
          {selectedContextLabel ? <span className="inline-flex h-7 items-center rounded-full hairline border-wd-hairline px-3 wd-caption text-wd-primary">{selectedContextLabel}</span> : null}
          {selectedAttachment ? (
            <>
              <span className="inline-flex h-7 items-center gap-1.5 rounded-full hairline border-wd-hairline px-3 wd-caption text-wd-secondary">
                <Link2 aria-hidden="true" className="size-3.5" /> {outcomeId ? t("waldo.project.outcomeContextAttached") : t("waldo.project.contextAttached")}
              </span>
              <Button aria-label={outcomeId ? t("waldo.project.detachOutcomeAria") : t("waldo.project.detachAria")} className="h-7" disabled={!daemonReady || busy} onClick={() => void detachContext(selectedAttachment)} size="sm" type="button" variant="ghost">
                {t("waldo.project.detach")}
              </Button>
            </>
          ) : (
            <Button className="h-7" disabled={!daemonReady || !snapshot || busy} onClick={() => void attachSelectedContext()} size="sm" type="button" variant="outline">
              <Link2 aria-hidden="true" className="size-3.5" /> {outcomeId ? t("waldo.project.attachOutcome") : t("waldo.project.attach")}
            </Button>
          )}
        </div>
        {!daemonReady ? (
          <p className="inline-flex items-center gap-1.5 wd-caption text-caution">
            <Clock aria-hidden="true" className="size-3.5 shrink-0" /> {t("waldo.project.offline")}
          </p>
        ) : null}
        {onOpenHome ? (
          <Button aria-label={t("waldo.project.homeAria")} className="-ml-3 self-start text-wd-secondary hover:text-wd-primary" onClick={onOpenHome} size="sm" type="button" variant="ghost">
            {t("waldo.project.home")}
            <ArrowUpRight aria-hidden="true" className="size-3.5" />
          </Button>
        ) : null}
      </div>

      <div className="min-h-0 flex-1 overflow-y-auto px-4 py-5">
        {loading ? <p className="wd-note text-wd-secondary">{t("waldo.project.loading")}</p> : null}
        {!loading && !snapshot ? <p className="wd-body-s text-wd-secondary">{t("waldo.project.noState")}</p> : null}
        {snapshot ? (
          <div aria-label={t("waldo.project.logAria")} className="flex flex-col gap-5" role="log">
            {snapshot.turns.length === 0 ? <p className="wd-body-s text-wd-secondary">{t("waldo.project.empty")}</p> : null}
            {snapshot.turns.map((turn) => {
              const time = formatTime(turn.createdAt);
              return turn.role === "user" ? (
                <article className="flex justify-end" key={turn.id}>
                  <div className="max-w-[80%] whitespace-pre-wrap rounded-lg rounded-br-xs bg-wd-inset px-4 py-3 wd-body text-wd-primary">
                    <span className="sr-only">{t("waldo.project.turn", { sequence: turn.sequence, role: turn.role })}</span>
                    {turn.message}
                  </div>
                </article>
              ) : (
                <article className="flex flex-col gap-2" key={turn.id}>
                  <span className="sr-only">{t("waldo.project.turn", { sequence: turn.sequence, role: turn.role })}</span>
                  <p className="flex items-baseline gap-2">
                    <span className="wd-caption text-wd-secondary">{t("waldo.rail.identity")}</span>
                    {time ? <time className="wd-data text-wd-tertiary" dateTime={turn.createdAt}>{time}</time> : null}
                  </p>
                  <p className="whitespace-pre-wrap wd-body-l text-wd-primary">{turn.message}</p>
                </article>
              );
            })}
          </div>
        ) : null}

        {(snapshot?.continuationReceipts.length ?? 0) > 0 ? (
          <section aria-label={t("waldo.project.receiptsAria")} className="mt-6 flex flex-col gap-3">
            {snapshot?.continuationReceipts.map((receipt) => {
              const expanded = expandedReceiptIds.has(receipt.id);
              const detailsId = `waldo-continuation-details-${receipt.id}`;
              const bindingStatus = {
                scope: continuationBindingStatus(receipt.previousBindings, receipt.replacementBindings, ["projectId", "outcomeId", "contractRevisionId", "planRevisionId", "workUnitId", "attemptId", "role"]),
                provider: continuationBindingStatus(receipt.previousBindings, receipt.replacementBindings, ["provider", "model", "profile"]),
                workspace: continuationBindingStatus(receipt.previousBindings, receipt.replacementBindings, ["workspaceOwner"]),
                authority: continuationBindingStatus(receipt.previousBindings, receipt.replacementBindings, ["authorityDigest", "effectPolicyDigest"]),
                budget: continuationBindingStatus(receipt.previousBindings, receipt.replacementBindings, ["budgetDigest"]),
              };
              const statusLabel = (status: ContinuationBindingStatus) => {
                if (status === "unchanged") return t("waldo.project.continuationUnchanged");
                if (status === "changed") return t("waldo.project.continuationChanged");
                return t("waldo.project.continuationUnavailable");
              };
              const receiptTime = formatTime(receipt.createdAt);
              return (
                <article className="flex flex-col gap-2 rounded-lg hairline border-wd-hairline bg-wd-surface p-4" key={receipt.id}>
                  <div className="flex min-h-6 items-center gap-2">
                    {receipt.action === "automatic" ? (
                      <span className="inline-flex items-center gap-1.5 wd-label text-wd-primary">
                        <CheckCircle2 aria-hidden="true" className="size-4 shrink-0 text-good" />
                        {t("waldo.project.continuedSafely")}
                      </span>
                    ) : (
                      <NeedsYouBadge>{t("waldo.project.needsYou")}</NeedsYouBadge>
                    )}
                    {receiptTime ? <time className="ml-auto wd-data text-wd-tertiary" dateTime={receipt.createdAt}>{receiptTime}</time> : null}
                  </div>
                  <p className="wd-body-s text-wd-primary">{receipt.action === "automatic" ? receipt.reasonDetail : receipt.needsUserReason || receipt.reasonDetail}</p>
                  {receipt.action === "automatic" ? <p className="wd-caption text-wd-secondary">{t("waldo.project.continuationSummary", { scope: statusLabel(bindingStatus.scope), provider: statusLabel(bindingStatus.provider), workspace: statusLabel(bindingStatus.workspace), authority: statusLabel(bindingStatus.authority), budget: statusLabel(bindingStatus.budget) })}</p> : null}
                  <p className="wd-caption text-wd-tertiary">{receipt.oldSessionFenced ? t("waldo.project.predecessorFenced") : t("waldo.project.fencingUnconfirmed")} · {receipt.replacementIdentityConfirmed ? t("waldo.project.replacementConfirmed") : t("waldo.project.noReplacement")}</p>
                  <Button
                    aria-controls={detailsId}
                    aria-expanded={expanded}
                    aria-label={expanded ? t("waldo.project.hideReceiptDetails") : t("waldo.project.showReceiptDetails")}
                    className="-ml-3 self-start text-wd-secondary hover:text-wd-primary"
                    onClick={() => toggleReceiptDetails(receipt.id)}
                    size="sm"
                    type="button"
                    variant="ghost"
                  >
                    <ChevronDown aria-hidden="true" className={cn("size-3.5 transition-transform duration-fast ease-out motion-reduce:transition-none", expanded && "rotate-180")} />
                    {expanded ? t("waldo.project.hideReceiptDetails") : t("waldo.project.showReceiptDetails")}
                  </Button>
                  {expanded ? (
                    <div aria-label={t("waldo.project.receiptDetailsAria")} className="rounded-md bg-wd-inset p-3" id={detailsId} role="region">
                      <dl className="flex flex-col gap-2.5">
                        <div className="flex flex-col gap-0.5">
                          <dt className="wd-caption text-wd-secondary">{t("waldo.project.oldSession")}</dt>
                          <dd className="break-all wd-data text-wd-primary">{receipt.fromAgentSessionRef}</dd>
                        </div>
                        {receipt.toAgentSessionRef ? (
                          <div className="flex flex-col gap-0.5">
                            <dt className="wd-caption text-wd-secondary">{t("waldo.project.newSession")}</dt>
                            <dd className="break-all wd-data text-wd-primary">{receipt.toAgentSessionRef}</dd>
                          </div>
                        ) : null}
                        <div className="flex flex-col gap-0.5">
                          <dt className="wd-caption text-wd-secondary">{t("waldo.project.triggerEvidence")}</dt>
                          <dd className="break-all wd-data text-wd-primary">{receipt.triggerEvidence.kind} · {receipt.triggerEvidence.reference}</dd>
                        </div>
                        <div className="flex flex-col gap-0.5">
                          <dt className="wd-caption text-wd-secondary">{t("waldo.project.contextCheckpoint")}</dt>
                          <dd className="break-all wd-data text-wd-primary">{receipt.contextDigest || t("waldo.project.continuationUnavailable")}</dd>
                        </div>
                        {(receipt.contextRefs?.length ?? 0) > 0 ? (
                          <div className="flex flex-col gap-0.5">
                            <dt className="wd-caption text-wd-secondary">{t("waldo.project.contextReferences")}</dt>
                            <dd className="flex flex-col gap-1 wd-data text-wd-primary">
                              {receipt.contextRefs.map((ref, index) => (
                                <div className="break-all" key={`${ref.kind}-${ref.objectId}-${ref.revision ?? index}`}>
                                  <div>{ref.kind} · {ref.objectId}{ref.revision ? ` · ${t("waldo.project.revision", { revision: ref.revision })}` : ""}</div>
                                  <div className="text-wd-secondary">{ref.provenance.kind} · {ref.provenance.sourceId}</div>
                                </div>
                              ))}
                            </dd>
                          </div>
                        ) : null}
                        {receipt.fenceReceiptRef ? (
                          <div className="flex flex-col gap-0.5">
                            <dt className="wd-caption text-wd-secondary">{t("waldo.project.fenceReceipt")}</dt>
                            <dd className="break-all wd-data text-wd-primary">{receipt.fenceReceiptRef}</dd>
                          </div>
                        ) : null}
                        {receipt.reconciliationRef ? (
                          <div className="flex flex-col gap-0.5">
                            <dt className="wd-caption text-wd-secondary">{t("waldo.project.reconciliation")}</dt>
                            <dd className="break-all wd-data text-wd-primary">{receipt.reconciliationRef}</dd>
                          </div>
                        ) : null}
                      </dl>
                    </div>
                  ) : null}
                </article>
              );
            })}
          </section>
        ) : null}

        {error ? (
          <div className="mt-4 flex items-start gap-2 rounded-md bg-critical-bg p-3 wd-body-s text-critical" role="alert">
            <AlertTriangle aria-hidden="true" className="mt-0.5 size-4 shrink-0" />
            <span className="min-w-0">{error}{requestId ? ` · Request ${requestId}` : ""}</span>
          </div>
        ) : null}
        {notice ? <p className="mt-3 wd-note text-wd-secondary" role="status">{notice}</p> : null}
      </div>

      <div className="flex flex-col gap-2 hairline-t border-wd-hairline px-3 pb-3 pt-2">
        <label className="sr-only" htmlFor={`waldo-message-${projectId}`}>{t("waldo.project.composerLabel")}</label>
        <div className="flex items-end gap-1">
          <div className="min-h-(--size-field) min-w-0 flex-1 rounded-[calc(var(--size-field)/2)] hairline border-wd-hairline bg-wd-inset px-4 py-3 outline-2 outline-offset-2 outline-transparent transition-[outline-color] duration-fast ease-out focus-within:outline-(--border-focus) motion-reduce:transition-none">
            <textarea id={`waldo-message-${projectId}`} aria-label={t("waldo.project.composerLabel")} className="block max-h-[6lh] min-h-6 w-full resize-none bg-transparent wd-body text-wd-primary field-sizing-content placeholder:text-wd-tertiary focus-visible:outline-none" onChange={(event) => setDraft(event.target.value)} onKeyDown={(event) => {
              if ((event.metaKey || event.ctrlKey) && event.key === "Enter") {
                event.preventDefault();
                void sendTurn();
              }
            }} placeholder={daemonReady ? t("waldo.project.placeholder") : t("waldo.project.placeholderOffline")} rows={1} value={draft} />
          </div>
          <Button aria-label={t("waldo.project.sendAria")} className="mb-0.5 size-11 rounded-full" disabled={!daemonReady || !snapshot || !draft.trim() || busy} onClick={() => void sendTurn()} size="icon" title={t("waldo.project.send")} type="button" variant="primary">
            <ArrowUp aria-hidden="true" className="size-5" />
          </Button>
        </div>
        <p className="px-4 wd-caption text-wd-tertiary">{activeEpisode ? t("waldo.project.episode", { ordinal: activeEpisode.ordinal }) : t("waldo.project.episodeOnSend")} · {t("waldo.project.shortcut")}</p>
        {!daemonReady && draft ? <p className="inline-flex items-center gap-1.5 px-4 wd-caption text-wd-tertiary"><Link2Off aria-hidden="true" className="size-3.5 shrink-0" /> {t("waldo.project.draftLocal")}</p> : null}
      </div>
    </div>
  );
}

/** The Needs you badge: the Spot dot plus the words (Waldo StateBadge). */
export function NeedsYouBadge({ children }: { children: ReactNode }) {
  return (
    <span className="inline-flex h-5 shrink-0 items-center gap-1 whitespace-nowrap rounded-xs bg-spot-subtle px-1.5 text-xs font-medium text-spot-text">
      <span aria-hidden="true" className="size-1.5 rounded-full bg-spot" />
      {children}
    </span>
  );
}
