import type { WaldoBridgePairResult, WaldoBridgeStatusResult } from "../../main/waldo-bridge-handler";

// Pure view models and IPC result mapping.
export type WaldoConnectionState =
 | { kind: "unavailable" | "unpaired" | "pairing" | "recovery_required" }
 | { kind: "paired"; transport: "offline" | "online"; deviceId?: string; label?: string }
 | { kind: "error"; reason: "failed" | "unavailable" };

export function validWaldoCode(value: string): boolean {
 // Waldo shows wp1.<owner id>.<code>; a bare code is accepted for a configured owner.
 const scoped = /^(?:wp1\.[A-Za-z0-9_-]{1,128}\.)?([A-Za-z0-9_-]{43})$/.exec(value);
 if (!scoped) return false;
 const code = scoped[1]!;
 // 32 bytes encode as 43 characters; only 4 of the final character's 6 bits
 // carry data. Round-trip equality rejects nonzero final padding bits.
 try {
  const bytes = atob(code.replace(/-/g, "+").replace(/_/g, "/") + "=");
  return bytes.length === 32 && btoa(bytes).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "") === code;
 } catch { return false; }
}
export function validWaldoLabel(label: string): boolean {
 for (let i = 0; i < label.length; i++) {
  const c = label.charCodeAt(i);
  if (c >= 0xd800 && c <= 0xdbff) {
   const next = label.charCodeAt(++i);
   if (!(next >= 0xdc00 && next <= 0xdfff)) return false;
  } else if (c >= 0xdc00 && c <= 0xdfff) return false;
 }
 const size = new TextEncoder().encode(label).length;
 return size >= 1 && size <= 120;
}

export function statusToState(result: WaldoBridgeStatusResult): WaldoConnectionState {
 if (!result.ok || !result.ready) return {kind:"unavailable"};
 switch(result.state) {
  case "unpaired": case "pairing": case "recovery_required": return {kind:result.state};
  case "offline": case "online": return {kind:"paired",transport:result.state,...(result.deviceId !== undefined ? {deviceId:result.deviceId}:{}),...(result.label !== undefined ? {label:result.label}:{})};
  default: return {kind:"unavailable"};
 }
}
export function pairResultToState(result: WaldoBridgePairResult): WaldoConnectionState {
 if (result.ok) return {kind:"unavailable"}; // Success requires a fresh status.
 return result.reason === "recovery_required" ? {kind:"recovery_required"} : {kind:"error",reason:result.reason};
}
