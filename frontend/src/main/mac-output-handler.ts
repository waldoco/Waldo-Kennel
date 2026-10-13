import type { IpcMainInvokeEvent, WebContents } from "electron";

export type MacActionSummary = { id: string; device_id: string; command?: string; action?: string; project: string; state: string };
export type MacHistoryResult = { ok: true; requests: MacActionSummary[] } | { ok: false };
export type MacOutputResult = { ok: true; output: string } | { ok: false };
type Event = Pick<IpcMainInvokeEvent, "sender" | "senderFrame">;
type Deps = { getShellWebContents: () => WebContents | null; getDaemonConnection: () => { port: number } | null; ownerCommandToken: string; fetch: typeof globalThis.fetch };
export function createMacOutputHandlers(deps: Deps) {
 const owned = (event: Event) => { const shell = deps.getShellWebContents(); return !!shell && !shell.isDestroyed() && event.sender === shell && event.senderFrame === shell.mainFrame; };
 const read = async (event: Event, path: string) => {
  const daemon = deps.getDaemonConnection();
  if (!owned(event) || !daemon || !deps.ownerCommandToken) return null;
  const response = await deps.fetch(`http://127.0.0.1:${daemon.port}/internal/owner-commands/mac-actions/${path}`, { headers: { Authorization: `KennelOwner ${deps.ownerCommandToken}` } });
  return owned(event) && deps.getDaemonConnection()?.port === daemon.port && response.ok ? response : null;
 };
 return {
  history: async (event: Event): Promise<MacHistoryResult> => {
   try {
    const response = await read(event, "history"); if (!response) return { ok: false };
    const raw: unknown = await response.json();
    if (!Array.isArray(raw) || raw.length > 20) return { ok: false };
    const requests: MacActionSummary[] = [];
    for (const row of raw) {
     if (!row || typeof row !== "object" || !["id","device_id","project","state"].every(key => typeof row[key] === "string") || ["command","action"].some(key => row[key] !== undefined && typeof row[key] !== "string")) return { ok: false };
     requests.push({id:row.id,device_id:row.device_id,project:row.project,state:row.state,...(row.command ? {command:row.command}: {}),...(row.action ? {action:row.action}: {})});
    }
    return owned(event) ? {ok:true,requests} : {ok:false};
   } catch { return {ok:false}; }
  },
  output: async (event: Event, input: unknown): Promise<MacOutputResult> => {
   if (!input || typeof input !== "object" || Array.isArray(input)) return {ok:false};
   const q = input as Record<string, unknown>;
   if (Object.keys(q).sort().join(",") !== "deviceId,id" || ![q.deviceId,q.id].every(v => typeof v === "string" && /^[A-Za-z0-9_-]{1,128}$/.test(v))) return {ok:false};
   try {
    const response = await read(event, `${encodeURIComponent(String(q.deviceId))}/${encodeURIComponent(String(q.id))}/output`);
    if (!response) return {ok:false};
    const output = await response.text();
    return owned(event) && Buffer.byteLength(output,"utf8") <= 3 * 1024 * 1024 ? {ok:true,output} : {ok:false};
   } catch { return {ok:false}; }
  },
 };
}
