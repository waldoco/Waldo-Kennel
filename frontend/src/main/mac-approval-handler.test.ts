import { describe, it, expect, vi } from "vitest";
import type { BaseWindow, WebContents } from "electron";
import { createMacApprovalRail, type MacApprovalDeps } from "./mac-approval-handler";
const request = { id:"command", device_id:"device", fingerprint:"a".repeat(64), kind:"approval", class:"shell_exec", project:"registered",folder:"/registered",command:"echo exact",reason:"run tests",risks:[],expires_at:Math.floor(Date.now()/1000)+900,state:"pending" };
function harness(response=1){
 const shell={isDestroyed:vi.fn(()=>false),mainFrame:{},on:vi.fn(),removeListener:vi.fn()};
 const window={isDestroyed:vi.fn(()=>false)};
 const deps:MacApprovalDeps={getWindow:()=>window as unknown as BaseWindow,getShellWebContents:()=>shell as unknown as WebContents,getDaemonConnection:()=>({port:3031}),ownerCommandToken:"local-test-owner-proof",fetch:vi.fn().mockResolvedValueOnce(new Response(JSON.stringify([request]),{status:200})).mockResolvedValue(new Response(null,{status:204})),showConfirmation:vi.fn(async()=>({response}))};
 const rail=createMacApprovalRail(deps);return{deps,rail,shell,window};
}
describe("native Mac approval rail",()=>{
 it("denies on cancel and keeps owner proof exclusively in the main request header",async()=>{const h=harness();await h.rail.pump();h.rail.stop();const calls=vi.mocked(h.deps.fetch).mock.calls;expect(calls).toHaveLength(2);const body=JSON.parse(String(calls[1][1]?.body));expect(body.decision).toBe("deny");expect(body).not.toHaveProperty("token");expect(h.deps.showConfirmation).toHaveBeenCalledWith(h.window,request)});
 it("approves only the exact dialog request once",async()=>{const h=harness(0);await h.rail.pump();h.rail.stop();expect(JSON.parse(String(vi.mocked(h.deps.fetch).mock.calls[1][1]?.body))).toEqual({id:request.id,device_id:request.device_id,fingerprint:request.fingerprint,decision:"approve"})});
 it("has no auto approval when the Mac window is closed",async()=>{const h=harness(0);h.deps.getWindow=()=>null;await h.rail.pump();h.rail.stop();expect(h.deps.fetch).not.toHaveBeenCalled();expect(h.deps.showConfirmation).not.toHaveBeenCalled()});
 it("invalidates a click after shell navigation",async()=>{const h=harness(0);h.deps.showConfirmation=vi.fn(async()=>{const listener=h.shell.on.mock.calls[0]?.[1] as (()=>void)|undefined;listener?.();return{response:0}});await h.rail.pump();h.rail.stop();expect(h.deps.fetch).toHaveBeenCalledTimes(1)});
 it("refuses expired requests",async()=>{const h=harness(0);vi.mocked(h.deps.fetch).mockReset().mockResolvedValue(new Response(JSON.stringify([{...request,expires_at:1}]),{status:200}));await h.rail.pump();h.rail.stop();expect(h.deps.showConfirmation).not.toHaveBeenCalled()});
});

it("retries the exact clicked decision after a transient failure",async()=>{
 const h=harness(0); const fetch=vi.mocked(h.deps.fetch);
 fetch.mockReset().mockResolvedValueOnce(new Response(JSON.stringify([request]),{status:200})).mockResolvedValueOnce(new Response(null,{status:503})).mockResolvedValueOnce(new Response(null,{status:204}));
 await h.rail.pump();h.rail.stop();expect(h.deps.showConfirmation).toHaveBeenCalledTimes(1);expect(fetch).toHaveBeenCalledTimes(3);expect(fetch.mock.calls[1][1]?.body).toEqual(fetch.mock.calls[2][1]?.body);
});
