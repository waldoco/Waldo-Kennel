import {it,expect,vi} from "vitest";
import type { WebContents } from "electron";
import {createMacOutputHandlers} from "./mac-output-handler";
function harness() {
 const frame={};const shell={mainFrame:frame,isDestroyed:()=>false} as unknown as WebContents;
 const fetch=vi.fn<typeof globalThis.fetch>();const h=createMacOutputHandlers({getShellWebContents:()=>shell,getDaemonConnection:()=>({port:3031}),ownerCommandToken:"test-local-proof",fetch});
 return {h,fetch,event:{sender:shell,senderFrame:shell.mainFrame}};
}
it("shows full local capture through owned-shell IPC without exposing owner proof",async()=>{
 const {h,fetch,event}=harness();const output="local output".repeat(800);
 fetch.mockResolvedValue(new Response(output));expect(await h.output(event,{deviceId:"dev",id:"cmd"})).toEqual({ok:true,output});
 expect(fetch.mock.calls[0][1]?.headers).toEqual({Authorization:"KennelOwner test-local-proof"});
});
it("rejects foreign frames and path injection before reading custody",async()=>{
 const {h,fetch,event}=harness();
 expect(await h.output({...event,senderFrame:{} as typeof event.senderFrame},{deviceId:"dev",id:"cmd"})).toEqual({ok:false});
 expect(await h.output(event,{deviceId:"dev",id:"../decision"})).toEqual({ok:false});expect(fetch).not.toHaveBeenCalled();
});
it("returns only bounded history summaries",async()=>{
 const {h,fetch,event}=harness();fetch.mockResolvedValue(new Response(JSON.stringify([{device_id:"dev",id:"cmd",project:"p",state:"finished",command:"echo hi",fingerprint:"hidden",local_output:"hidden"}])));
 expect(await h.history(event)).toEqual({ok:true,requests:[{device_id:"dev",id:"cmd",project:"p",state:"finished",command:"echo hi"}]});
});
