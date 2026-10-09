import type { BaseWindow, WebContents } from "electron";
import { createIslandEventStream } from "./island/event-stream";

export type MacApproval = { id: string; device_id: string; fingerprint: string; kind: "approval"; class: string; project: string; folder: string; session_id?: string; command?: string; prompt?: string; reason: string; action?: string; risks: string[]; expires_at: number; state: "pending" };
export type MacApprovalDeps = {
 getWindow: () => BaseWindow | null;
 getShellWebContents: () => WebContents | null;
 getDaemonConnection: () => { port: number } | null;
 ownerCommandToken: string;
 fetch: typeof globalThis.fetch;
 showConfirmation: (window: BaseWindow, request: MacApproval) => Promise<{ response: number }>;
};
function approval(value: unknown): value is MacApproval {
 if (!value || typeof value !== "object" || Array.isArray(value)) return false;
 const q = value as Record<string, unknown>;
 return ["id", "device_id", "fingerprint", "class", "project", "folder", "reason"].every(k => typeof q[k] === "string") && /^[a-f0-9]{64}$/.test(String(q.fingerprint)) && q.kind === "approval" && q.state === "pending" && typeof q.expires_at === "number" && Number.isSafeInteger(q.expires_at) && Array.isArray(q.risks) && q.risks.every(v => typeof v === "string") && ["command", "prompt", "action", "session_id"].every(k => q[k] === undefined || typeof q[k] === "string");
}
// Main alone reads custody and posts decisions. No renderer decision IPC or
// chat acknowledgement can grant permission. The durable event stream wakes
// this reader; a rebind also recovers approvals posted while the app was closed.
export function createMacApprovalRail(deps: MacApprovalDeps) {
 let running = false, stopped = false;
 const shown = new Set<string>();
 const pump = async () => {
  if (running || stopped) return;
  const daemon = deps.getDaemonConnection();
  const window = deps.getWindow(), shell = deps.getShellWebContents();
  if (!daemon || !window || window.isDestroyed() || !shell || shell.isDestroyed() || !deps.ownerCommandToken) return;
  running = true;
  try {
   const base = `http://127.0.0.1:${daemon.port}/internal/owner-commands/mac-actions`;
   const response = await deps.fetch(`${base}/pending`, { headers: { Authorization: `KennelOwner ${deps.ownerCommandToken}` } });
   if (!response.ok) return;
   const rows: unknown = await response.json();
   if (!Array.isArray(rows) || rows.length > 20) return;
   for (const row of rows) {
    if (!approval(row) || row.expires_at <= Math.floor(Date.now()/1000)) continue;
    const key = `${row.device_id}:${row.id}:${row.fingerprint}`;
    if (shown.has(key)) continue;
    if (deps.getWindow() !== window || deps.getShellWebContents() !== shell || window.isDestroyed() || shell.isDestroyed()) break;
    const frame = shell.mainFrame;
    let navigated = false;
    const invalidate = () => { navigated = true; };
    shell.on("did-start-navigation", invalidate);
    let confirmation;
    try { confirmation = await deps.showConfirmation(window, row); }
    finally { shell.removeListener("did-start-navigation", invalidate); }
    if (stopped || navigated || shell.isDestroyed() || window.isDestroyed() || deps.getWindow() !== window || deps.getShellWebContents() !== shell || shell.mainFrame !== frame || deps.getDaemonConnection()?.port !== daemon.port) continue;
    // The daemon makes the final atomic deadline and fingerprint check.
    const decision = { device_id: row.device_id, id: row.id, fingerprint: row.fingerprint, decision: confirmation.response === 0 ? "approve" : "deny" };
    // Retry only this exact native decision. A lost response must not cause a
    // second execution or permanently hide a still-pending approval.
    for (let attempt=0; attempt<3; attempt++) {
     if (stopped || navigated || row.expires_at <= Math.floor(Date.now()/1000) || window.isDestroyed() || shell.isDestroyed() || deps.getWindow() !== window || deps.getShellWebContents() !== shell || shell.mainFrame !== frame || deps.getDaemonConnection()?.port !== daemon.port) break;
     try {
      const posted = await deps.fetch(`${base}/decision`, { method: "POST", headers: { "Content-Type": "application/json", Authorization: `KennelOwner ${deps.ownerCommandToken}` }, body: JSON.stringify(decision) });
      if (posted.ok || posted.status === 409) { shown.add(key); break; }
     } catch { /* Exact decision retry stays bound to the live Mac window. */ }
     if (attempt < 2) await new Promise(resolve => setTimeout(resolve, 250*(attempt+1)));
    }
   }
  } catch { /* Transport/dialog failure leaves durable approval pending. */ }
  finally { running = false; }
 };
 const events = createIslandEventStream({ fetch: deps.fetch, onInvalidate: () => { void pump(); } });
 return { pump, rebind(port: number | null) { events.rebind(port); void pump(); }, stop() { stopped = true; events.stop(); } };
}
