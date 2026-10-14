import { StrictMode } from "react";
import { act, fireEvent, render, screen, waitFor, cleanup } from "@testing-library/react";
import { I18nextProvider } from "react-i18next";
import { afterEach, describe, expect, it, vi } from "vitest";
import { APP_LOCALES, createAppI18n, catalogFor } from "../../i18n";
import { WaldoConnectionSection, WaldoConnectionView } from "./WaldoConnectionSection";
import type { WaldoConnectionState } from "../../lib/waldo-connection";

const code = "A".repeat(43); // Synthetic zero bytes, never a real pairing code.
function view(node: React.ReactNode) { return render(<I18nextProvider i18n={createAppI18n()}>{node}</I18nextProvider>); }
afterEach(() => { cleanup(); vi.restoreAllMocks(); vi.unstubAllGlobals(); });
describe("inactive Waldo settings", () => {
 it("always shows unavailable with disabled secret entry, no preview controls or network", () => {
  const fetch = vi.fn(); const socket = vi.fn(); vi.stubGlobal("fetch",fetch); vi.stubGlobal("WebSocket",socket);
  view(<WaldoConnectionSection />);
  expect(screen.getByText("Pairing is not enabled on this build")).toBeInTheDocument();
  expect(screen.getByLabelText("Waldo-issued code")).toBeDisabled();
  expect(screen.getByLabelText("Device label")).toBeDisabled();
  expect(screen.getByRole("button", {name:"Connect"})).toBeDisabled();
  expect(screen.queryByTestId("waldo-preview-label")).not.toBeInTheDocument();
  expect(fetch).not.toHaveBeenCalled(); expect(socket).not.toHaveBeenCalled();
 });
 it.each<WaldoConnectionState>([{kind:"unpaired"},{kind:"pairing"},{kind:"recovery_required"},{kind:"paired",transport:"offline",deviceId:"synthetic-device",label:"Synthetic Mac"},{kind:"paired",transport:"online"},{kind:"error",reason:"failed"},{kind:"error",reason:"unavailable"}])("keeps state %j distinct and offers no recovery retry", state => {
  view(<WaldoConnectionView state={state} previewAction={vi.fn()} />);
  expect(screen.getByTestId("waldo-preview-label")).toBeInTheDocument();
  if(state.kind === "paired") { expect(screen.getByText(`Paired · ${state.transport === "offline" ? "Offline" : "Online"}`)).toBeInTheDocument(); expect(screen.queryByText("Not paired")).not.toBeInTheDocument(); }
  expect(screen.queryByRole("button", {name:/retry|reset|pair again/i})).not.toBeInTheDocument();
  expect(screen.getByLabelText("Waldo-issued code")).toHaveProperty("disabled",state.kind !== "unpaired");
 });
 it("blocks double submit, never persists/logs a code, and freshly clears the code after a local action", async () => {
  let finish!: (state: WaldoConnectionState) => void;
  const action = vi.fn(() => new Promise<WaldoConnectionState>(resolve => {finish=resolve;}));
  const store = vi.spyOn(Storage.prototype,"setItem"); const log = vi.spyOn(console,"log");
  const fetch=vi.fn(); const socket=vi.fn(); vi.stubGlobal("fetch",fetch);vi.stubGlobal("WebSocket",socket);
  view(<WaldoConnectionView state={{kind:"unpaired"}} previewAction={action} />);
  fireEvent.change(screen.getByLabelText("Waldo-issued code"),{target:{value:code}});
  fireEvent.change(screen.getByLabelText("Device label"),{target:{value:"機".repeat(40)}});
  const form=screen.getByRole("button",{name:"Connect"}).closest("form")!;
  fireEvent.submit(form);fireEvent.submit(form);
  expect(action).toHaveBeenCalledTimes(1); expect(screen.getByText("Pairing…")).toBeInTheDocument();
  await act(async () => finish({kind:"paired",transport:"offline"}));
  expect(screen.getByLabelText("Waldo-issued code")).toHaveValue("");
  expect(store).not.toHaveBeenCalled();expect(log).not.toHaveBeenCalled();expect(fetch).not.toHaveBeenCalled();expect(socket).not.toHaveBeenCalled();
 });
 it("rejects malformed code/label without normalization or a submit action", () => {
  const action=vi.fn();view(<WaldoConnectionView state={{kind:"unpaired"}} previewAction={action}/>);
  fireEvent.change(screen.getByLabelText("Waldo-issued code"),{target:{value:code+"="}});
  fireEvent.change(screen.getByLabelText("Device label"),{target:{value:"é".repeat(61)}});
  fireEvent.submit(screen.getByRole("button",{name:"Connect"}).closest("form")!);
  expect(screen.getAllByRole("alert")).toHaveLength(2);expect(action).not.toHaveBeenCalled();
  expect(screen.getByLabelText("Waldo-issued code")).toHaveValue(code+"=");
 });
 it("clears on close and ignores an action that settles after closing", async () => {
  let finish!: (state: WaldoConnectionState) => void;
  const state: WaldoConnectionState={kind:"unpaired"};const action=vi.fn(() => new Promise<WaldoConnectionState>(resolve=>{finish=resolve;}));
  const wrap=(open:boolean)=><I18nextProvider i18n={createAppI18n()}><WaldoConnectionView state={state} open={open} previewAction={action}/></I18nextProvider>;
  const rendered=render(wrap(true));fireEvent.change(screen.getByLabelText("Waldo-issued code"),{target:{value:code}});fireEvent.change(screen.getByLabelText("Device label"),{target:{value:"Synthetic Mac"}});
  fireEvent.submit(screen.getByRole("button",{name:"Connect"}).closest("form")!);rendered.rerender(wrap(false));
  await act(async()=>finish({kind:"paired",transport:"online"}));rendered.rerender(wrap(true));
  expect(screen.getByLabelText("Waldo-issued code")).toHaveValue("");expect(screen.getByText("Not paired")).toBeInTheDocument();
 });
 it("shows only a safe error when an injected action throws",async()=>{
  view(<WaldoConnectionView state={{kind:"unpaired"}} previewAction={async()=>{throw new Error(code);}}/>);
  fireEvent.change(screen.getByLabelText("Waldo-issued code"),{target:{value:code}});fireEvent.change(screen.getByLabelText("Device label"),{target:{value:"Synthetic Mac"}});
  fireEvent.submit(screen.getByRole("button",{name:"Connect"}).closest("form")!);
  await waitFor(()=>expect(screen.getByText(/Connection status is unknown/)).toBeInTheDocument());expect(screen.getByLabelText("Waldo-issued code")).toHaveValue("");expect(document.body.textContent).not.toContain(code);
 });
 it.each(APP_LOCALES)("has all new strings and localized unavailable copy in %s",locale=>{
  const catalog=catalogFor(locale);const keys=Object.keys(catalogFor("en")).filter(k=>k.startsWith("settings.waldo."));
  for(const key of keys)expect(catalog[key]).toBeTruthy();
  render(<I18nextProvider i18n={createAppI18n(locale)}><WaldoConnectionSection/></I18nextProvider>);
  expect(screen.getByText(catalog["settings.waldo.status.unavailable"])).toBeInTheDocument();
 });
});

function fakeLive(statusResult: import("../../../main/waldo-bridge-handler").WaldoBridgeStatusResult = {ok:true,state:"unpaired",ready:true}) {
 const bridge = {status:vi.fn(async()=>statusResult),pair:vi.fn<NonNullable<NonNullable<Window["kennel"]>["waldoBridge"]>["pair"]>().mockResolvedValue({ok:true})};
 vi.stubGlobal("kennel",{waldoBridge:bridge});return bridge;
}
function enter() {
 fireEvent.change(screen.getByLabelText("Waldo-issued code"),{target:{value:code}});
 fireEvent.change(screen.getByLabelText("Device label"),{target:{value:"Synthetic Mac"}});
 return screen.getByRole("button",{name:"Connect"}).closest("form")!;
}
async function opened() { await waitFor(()=>expect(screen.getByLabelText("Waldo-issued code")).toBeEnabled()); }

describe("production Waldo adapter",()=>{
 it("shows unavailable and makes zero bridge calls when waldoBridge is absent",()=>{
  vi.stubGlobal("kennel",{});const fetch=vi.fn();vi.stubGlobal("fetch",fetch);view(<WaldoConnectionSection/>);
  expect(screen.getByLabelText("Waldo-issued code")).toBeDisabled();expect(fetch).not.toHaveBeenCalled();
 });
 it("makes exactly one status call when opened and none when closed, with no timers",async()=>{
  vi.useFakeTimers();try {
   const bridge=fakeLive();const wrap=(open:boolean)=><I18nextProvider i18n={createAppI18n()}><WaldoConnectionSection open={open}/></I18nextProvider>;
   const r=render(wrap(false));expect(bridge.status).not.toHaveBeenCalled();
   await act(async()=>r.rerender(wrap(true)));expect(bridge.status).toHaveBeenCalledTimes(1);
   await act(async()=>vi.advanceTimersByTime(3600000));expect(bridge.status).toHaveBeenCalledTimes(1);
   r.rerender(wrap(false));expect(bridge.status).toHaveBeenCalledTimes(1);expect(vi.getTimerCount()).toBe(0);
  } finally {vi.useRealTimers();}
 });
 it("pair sends exactly {code,label,capabilities:[machine_state_query,notify_local]} and nothing else",async()=>{
  const bridge=fakeLive();bridge.status.mockResolvedValueOnce({ok:true,state:"unpaired",ready:true}).mockResolvedValue({ok:true,state:"online",ready:true});
  view(<WaldoConnectionSection/>);await opened();fireEvent.submit(enter());
  await waitFor(()=>expect(bridge.pair).toHaveBeenCalledTimes(1));expect(bridge.pair.mock.calls[0]).toStrictEqual([{code,label:"Synthetic Mac",sessionConsent:false,capabilities:["machine_state_query","notify_local","shell_exec"]}]);
  await waitFor(()=>expect(screen.getByText("Paired · Online")).toBeInTheDocument());expect(bridge.status).toHaveBeenCalledTimes(2);expect(screen.queryByTestId("waldo-preview-label")).not.toBeInTheDocument();
 });
 it("refreshes status once after a successful pair and does not assume paired from the pair reply",async()=>{
  const bridge=fakeLive();bridge.status.mockResolvedValueOnce({ok:true,state:"unpaired",ready:true}).mockResolvedValue({ok:true,state:"pairing",ready:true});
  view(<WaldoConnectionSection/>);await opened();fireEvent.submit(enter());await waitFor(()=>expect(bridge.status).toHaveBeenCalledTimes(2));
  expect(screen.getByText("Pairing…")).toBeInTheDocument();expect(screen.queryByText(/Paired ·/)).not.toBeInTheDocument();
 });
 it("409 shows recovery_required with no retry, reset or pair-again control and sends no second request",async()=>{
  const bridge=fakeLive();bridge.pair.mockResolvedValue({ok:false,reason:"recovery_required"});view(<WaldoConnectionSection/>);await opened();fireEvent.submit(enter());
  await waitFor(()=>expect(screen.getByText("Recovery required")).toBeInTheDocument());expect(screen.queryByRole("button",{name:/retry|reset|pair.again/i})).not.toBeInTheDocument();
  fireEvent.submit(screen.getByRole("button",{name:"Connect"}).closest("form")!);expect(bridge.pair).toHaveBeenCalledTimes(1);expect(bridge.status).toHaveBeenCalledTimes(1);
 });
 it.each(["success","recovery_required","unavailable","failed","exception"])("clears the code after %s; never stores/logs it",async reason=>{
  const bridge=fakeLive();const store=vi.spyOn(Storage.prototype,"setItem");const spies=[vi.spyOn(console,"log"),vi.spyOn(console,"warn"),vi.spyOn(console,"error")];
  if(reason==="exception")bridge.pair.mockRejectedValue(new Error(code));else if(reason!=="success")bridge.pair.mockResolvedValue({ok:false,reason:reason as "failed"|"unavailable"|"recovery_required"});
  view(<WaldoConnectionSection/>);await opened();fireEvent.submit(enter());await waitFor(()=>expect(screen.getByLabelText("Waldo-issued code")).toHaveValue(""));
  expect(store).not.toHaveBeenCalled();for(const spy of spies)expect(JSON.stringify(spy.mock.calls)).not.toContain(code);
  expect(JSON.stringify({...localStorage,...sessionStorage})).not.toContain(code);expect(document.body.textContent).not.toContain(code);expect(document.body.innerHTML).not.toContain(code);
  if(reason==="exception" || reason==="failed")expect(screen.getByText("Could not complete pairing. Check the connection status before trying again.")).toBeInTheDocument();
 });
 it("blocks double submit with a single pair call",async()=>{
  const bridge=fakeLive();let finish!:(v:{ok:true})=>void;bridge.pair.mockImplementation(()=>new Promise(resolve=>{finish=resolve;}));view(<WaldoConnectionSection/>);await opened();const form=enter();fireEvent.submit(form);fireEvent.submit(form);expect(bridge.pair).toHaveBeenCalledTimes(1);await act(async()=>finish({ok:true}));
 });
 it("ignores a pair result that settles after the section is closed",async()=>{
  const bridge=fakeLive();let finish!:(v:{ok:true})=>void;bridge.pair.mockImplementation(()=>new Promise(resolve=>{finish=resolve;}));
  const wrap=(open:boolean)=><I18nextProvider i18n={createAppI18n()}><WaldoConnectionSection open={open}/></I18nextProvider>;
  const r=render(wrap(true));await opened();fireEvent.submit(enter());r.rerender(wrap(false));expect(screen.getByLabelText("Waldo-issued code")).toHaveValue("");await act(async()=>finish({ok:true}));expect(bridge.status).toHaveBeenCalledTimes(1);expect(screen.queryByText(/Paired ·/)).not.toBeInTheDocument();
 });
 it.each(["online","offline","unpaired"])("shows the unknown / delivery_unknown copy when paired and not otherwise: %s",async state=>{
  fakeLive({ok:true,state,ready:true});view(<WaldoConnectionSection/>);await waitFor(()=>expect(screen.getByText(state === "unpaired" ? "Not paired" : `Paired · ${state === "online" ? "Online":"Offline"}`)).toBeInTheDocument());expect(screen.queryByText(/answers machine-state queries as unknown/)!==null).toBe(state!=="unpaired");
 });
 it.each([{ok:false,reason:"unavailable"},{ok:true,state:"unpaired",ready:false},{ok:true,state:"future",ready:true}] as const)("no request and no pair call while the state is unavailable or ready is false: %j",async result=>{
  const bridge=fakeLive(result);view(<WaldoConnectionSection/>);await act(async()=>{});expect(screen.getByLabelText("Waldo-issued code")).toBeDisabled();fireEvent.submit(screen.getByRole("button",{name:"Connect"}).closest("form")!);expect(bridge.pair).not.toHaveBeenCalled();expect(bridge.status).toHaveBeenCalledTimes(1);
 });
});

 it("makes one status call through StrictMode effect replay",async()=>{
  const bridge=fakeLive();view(<StrictMode><WaldoConnectionSection/></StrictMode>);await opened();expect(bridge.status).toHaveBeenCalledTimes(1);
 });

describe("one-click pairing", () => {
 it("opens Waldo from Settings, reports a browser that did not open, and accepts the pasted owner-scoped code", async () => {
  const openPairing = vi.fn(async () => false);
  const action = vi.fn(async () => ({kind:"unpaired"}) as WaldoConnectionState);
  view(<WaldoConnectionView state={{kind:"unpaired"}} action={action} openPairing={openPairing} defaultLabel="Synthetic Mac" />);
  expect(screen.getByLabelText("Device label")).toHaveValue("Synthetic Mac");
  fireEvent.click(screen.getByRole("button", {name:"Get a code from Waldo"}));
  await waitFor(() => expect(screen.getByRole("alert")).toHaveTextContent("Your browser didn’t open."));
  expect(openPairing).toHaveBeenCalledTimes(1);
  fireEvent.change(screen.getByLabelText("Waldo-issued code"),{target:{value:`  wp1.owner-1.${code}\n`}});
  fireEvent.submit(screen.getByRole("button",{name:"Connect"}).closest("form")!);
  await waitFor(() => expect(action).toHaveBeenCalledWith({code:`wp1.owner-1.${code}`,label:"Synthetic Mac",sessionConsent:false}));
 });
 it("keeps a typed label and offers no Waldo button once paired", () => {
  const { rerender } = view(<WaldoConnectionView state={{kind:"unpaired"}} action={vi.fn()} openPairing={vi.fn()} />);
  fireEvent.change(screen.getByLabelText("Device label"),{target:{value:"Typed"}});
  rerender(<I18nextProvider i18n={createAppI18n()}><WaldoConnectionView state={{kind:"unpaired"}} action={vi.fn()} openPairing={vi.fn()} defaultLabel="Synthetic Mac" /></I18nextProvider>);
  expect(screen.getByLabelText("Device label")).toHaveValue("Typed");
  rerender(<I18nextProvider i18n={createAppI18n()}><WaldoConnectionView state={{kind:"paired",transport:"online"}} action={vi.fn()} openPairing={vi.fn()} /></I18nextProvider>);
  expect(screen.getByRole("button", {name:"Get a code from Waldo"})).toBeDisabled();
 });
});
