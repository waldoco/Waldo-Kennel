import { Activity, Bell, Circle, CircleCheck, CircleX, Clock } from "lucide-react";
import { useEffect, useId, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "../ui/button";
import { Input } from "../ui/input";
import { Switch } from "../ui/switch";
import { SettingsSection } from "./SettingsSection";
import { statusToState, pairResultToState, validWaldoCode, validWaldoLabel, type WaldoConnectionState } from "../../lib/waldo-connection";

// The optional preload is the only production boundary. Never retain a token.
export function WaldoConnectionSection({ open = true, titleHidden = false }: { open?: boolean; titleHidden?: boolean }) {
 const [state, setState] = useState<WaldoConnectionState>({kind:"unavailable"});
 const generation = useRef(0);
 const opening = useRef<Promise<WaldoConnectionState> | null>(null);
 useEffect(() => {
  const ticket = ++generation.current;
  setState({kind:"unavailable"});
  const bridge = window.kennel?.waldoBridge;
  if (!open) opening.current = null;
  if (open && bridge) {
   // StrictMode replays effect setup; reuse this open's read, never poll.
   opening.current ??= (async (): Promise<WaldoConnectionState> => {
    try { return statusToState(await bridge.status()); }
    catch { return {kind:"error",reason:"failed"}; }
   })();
   void opening.current.then(next => {
    if (ticket === generation.current) setState(next);
   });
  }
  return () => { generation.current++; };
 }, [open]);
 const pair = async (input: {code:string;label:string;sessionConsent?:boolean}): Promise<WaldoConnectionState> => {
  const ticket = generation.current;
  const bridge = window.kennel?.waldoBridge;
  if (!open || !bridge) return {kind:"unavailable"};
  const result = await bridge.pair({...input, capabilities:input.sessionConsent ? ["machine_state_query","notify_local","shell_exec","session_query","session_control"] : ["machine_state_query","notify_local","shell_exec"]});
  if (ticket !== generation.current) return {kind:"unavailable"};
  if (!result.ok) return pairResultToState(result);
  return statusToState(await bridge.status());
 };
 return <><WaldoConnectionView state={state} action={pair} open={open} titleHidden={titleHidden} />{open && <MacOutputHistory />}</>;
}

// Pure presentation boundary for local development fixtures, not an API client.
export function WaldoConnectionView({ state, action, previewAction, open = true, titleHidden = false }: {
 state: WaldoConnectionState;
 action?: (input: { code: string; label: string; sessionConsent?: boolean }) => Promise<WaldoConnectionState>;
 previewAction?: (input: { code: string; label: string; sessionConsent?: boolean }) => Promise<WaldoConnectionState>;
 open?: boolean;
 titleHidden?: boolean;
}) {
 const { t } = useTranslation();
 const id = useId();
 const [code, setCode] = useState("");
 const [label, setLabel] = useState("");
 const [sessionConsent, setSessionConsent] = useState(false);
 const [pending, setPending] = useState(false);
 const [outcome, setOutcome] = useState<WaldoConnectionState | null>(null);
 const [invalid, setInvalid] = useState(false);
 const busy = useRef(false);
 const generation = useRef(0);
 useEffect(() => {
  setOutcome(null); setInvalid(false);
 }, [state]);
 useEffect(() => {
  if (!open) { setCode(""); setLabel(""); setOutcome(null); setPending(false); busy.current = false; generation.current++; }
  return () => { generation.current++; };
 }, [open]);
 const current = pending ? { kind: "pairing" as const } : outcome ?? state;
 const perform = action ?? previewAction;
 const canEnter = !!perform && current.kind === "unpaired" && open;
 const status = current.kind === "paired" ? current.transport : current.kind === "error" ? current.reason === "failed" && action ? "failedLive" : current.reason : current.kind;
 const submit = async (event: React.FormEvent) => {
  event.preventDefault();
  if (!canEnter || busy.current || !perform) return;
  if (!validWaldoCode(code) || !validWaldoLabel(label)) { setInvalid(true); return; }
  busy.current = true; setPending(true); setInvalid(false);
  const ticket = generation.current;
  try {
   const next = await perform({ code, label, sessionConsent });
   if (ticket === generation.current) setOutcome(next);
  } catch {
   // Never expose an exception that might contain a code or remote response.
   if (ticket === generation.current) setOutcome({ kind: "error", reason: "failed" });
  } finally {
   if (ticket === generation.current) { setCode(""); setPending(false); busy.current = false; }
  }
 };
 const tone = current.kind === "paired"
  ? current.transport === "online" ? { icon: CircleCheck, tile: "bg-good-bg text-good" } : { icon: Clock, tile: "bg-caution-bg text-caution" }
  : current.kind === "recovery_required" || current.kind === "error" ? { icon: CircleX, tile: "bg-critical-bg text-critical" }
  : { icon: Circle, tile: "bg-wd-inset text-wd-secondary" };
 const ToneIcon = tone.icon;
 const fieldClass = "h-11 rounded-md border border-transparent bg-wd-inset px-4 text-sm text-wd-primary placeholder:text-wd-tertiary focus-visible:border-(--border-focus) focus-visible:ring-0 focus-visible:shadow-[inset_0_0_0_var(--stroke-1)_var(--border-focus)] disabled:opacity-100 disabled:text-wd-secondary aria-invalid:border-critical aria-invalid:ring-0";
 return <SettingsSection title={t("settings.waldo.title")} titleHidden={titleHidden} sectionId="waldo">
  <div className="flex min-w-0 flex-col gap-8">
   <div className="flex flex-col gap-3">
    {previewAction && <p className="wd-body-s rounded-md bg-wd-inset px-4 py-3 text-wd-primary" data-testid="waldo-preview-label">{t("settings.waldo.preview")}</p>}
    <p className="wd-body-l text-wd-secondary">{t("settings.waldo.description")}</p>
   </div>
   <div className="flex flex-col gap-2">
    <div className="settings-grouped-rows" role="status" aria-live="polite">
     <div className="settings-row-bar items-start">
      <div className="flex min-w-0 items-start gap-3">
       <span className={`settings-row-icon grid size-7 shrink-0 place-items-center rounded-sm ${tone.tile}`} aria-hidden="true"><ToneIcon className="size-4" /></span>
       <div className="flex min-w-0 flex-col gap-0.5">
        <p className="wd-body text-wd-primary">{t(`settings.waldo.status.${status}`)}</p>
        {current.kind === "recovery_required" && <p className="wd-body-s text-wd-secondary">{t("settings.waldo.recoveryHelp")}</p>}
        {current.kind === "paired" && <div className="flex flex-col gap-0.5 break-all wd-body-s text-wd-secondary">
         <p>{t("settings.waldo.identity")}</p>
         {current.label && <p className="text-wd-primary">{current.label}</p>}
         {current.deviceId && <p className="wd-data">{current.deviceId}</p>}
        </div>}
       </div>
      </div>
     </div>
    </div>
    {current.kind === "paired" && <p className="wd-caption px-4 text-wd-tertiary">{t("settings.waldo.unknownAnswers")}</p>}
   </div>
   <div className="flex flex-col gap-2">
    <h3 className="wd-label px-4 text-wd-secondary">{t("settings.waldo.scope")}</h3>
    <ul className="settings-grouped-rows">
     <li className="settings-row-bar justify-start"><span className="settings-row-icon grid size-7 shrink-0 place-items-center rounded-sm bg-wd-inset text-wd-secondary" aria-hidden="true"><Activity className="size-4" /></span><span className="wd-body text-wd-primary">{t("settings.waldo.queries")}</span></li>
     <li className="settings-row-bar justify-start"><span className="settings-row-icon grid size-7 shrink-0 place-items-center rounded-sm bg-wd-inset text-wd-secondary" aria-hidden="true"><Bell className="size-4" /></span><span className="wd-body text-wd-primary">{t("settings.waldo.notifications")}</span></li>
    </ul>
    <p className="wd-caption px-4 text-wd-tertiary">{t("settings.waldo.approvals")}</p>
    <p className="wd-caption px-4 text-wd-tertiary">{t("settings.waldo.commands")}</p>
   </div>
   <form onSubmit={submit} className="flex flex-col gap-5" autoComplete="off">
    <div className="settings-grouped-rows">
     <div className="settings-row-bar">
      <label htmlFor={`${id}-session-consent`} className="wd-body text-wd-primary">{t("settings.waldo.sessionConsent")}</label>
      <Switch id={`${id}-session-consent`} checked={sessionConsent} disabled={!canEnter} onCheckedChange={setSessionConsent} />
     </div>
    </div>
    <div className="flex flex-col gap-2">
     <label htmlFor={`${id}-code`} className="wd-label text-wd-secondary">{t("settings.waldo.code")}</label>
     <Input id={`${id}-code`} type="password" value={code} onChange={e => setCode(e.target.value)} disabled={!canEnter} autoComplete="off" spellCheck={false} autoCapitalize="none" aria-describedby={`${id}-code-help`} aria-invalid={invalid && !validWaldoCode(code)} className={fieldClass} data-ph-no-capture />
     <p id={`${id}-code-help`} className="wd-body-s text-wd-tertiary">{t("settings.waldo.codeHelp")}</p>
     {invalid && !validWaldoCode(code) && <p role="alert" className="wd-body-s text-critical">{t("settings.waldo.invalidCode")}</p>}
    </div>
    <div className="flex flex-col gap-2">
     <label htmlFor={`${id}-label`} className="wd-label text-wd-secondary">{t("settings.waldo.label")}</label>
     <Input id={`${id}-label`} value={label} onChange={e => setLabel(e.target.value)} disabled={!canEnter} aria-describedby={`${id}-label-help`} aria-invalid={invalid && !validWaldoLabel(label)} className={fieldClass} />
     <p id={`${id}-label-help`} className="wd-body-s text-wd-tertiary">{t("settings.waldo.labelHelp")}</p>
     {invalid && !validWaldoLabel(label) && <p role="alert" className="wd-body-s text-critical">{t("settings.waldo.invalidLabel")}</p>}
    </div>
    <Button type="submit" variant="primary" className="self-end" disabled={!canEnter || !validWaldoCode(code) || !validWaldoLabel(label)}>{t(pending ? "settings.waldo.connecting" : "settings.waldo.connect")}</Button>
   </form>
  </div>
 </SettingsSection>;
}

function MacOutputHistory() {
 const {t} = useTranslation();
 const [rows,setRows] = useState<import("../../../main/mac-output-handler").MacActionSummary[]>([]);
 const [output,setOutput] = useState<string | null>(null);
 const [error,setError] = useState(false);
 const [busy,setBusy] = useState(false);
 const bridge = window.kennel?.waldoBridge;
 if (!bridge?.history || !bridge?.output) return null;
 const refresh = async () => {
  setBusy(true); setError(false); setOutput(null);
  try { const result=await bridge.history(); if(result.ok) setRows(result.requests); else setError(true); }
  catch { setError(true); } finally { setBusy(false); }
 };
 const show = async (row: typeof rows[number]) => {
  setBusy(true); setError(false);
  try { const result=await bridge.output({deviceId:row.device_id,id:row.id}); if(result.ok) setOutput(result.output); else setError(true); }
  catch { setError(true); } finally { setBusy(false); }
 };
 return <section className="px-3 text-sm" aria-label={t("settings.waldo.outputTitle")}>
  <p className="font-medium">{t("settings.waldo.outputTitle")}</p>
  <p className="text-muted-foreground">{t("settings.waldo.outputHelp")}</p>
  <Button type="button" disabled={busy} onClick={()=>void refresh()}>{t("settings.waldo.refreshOutput")}</Button>
  <ul>{rows.map(row=><li key={`${row.device_id}:${row.id}`}><Button type="button" disabled={busy} onClick={()=>void show(row)}>{row.command ?? row.action ?? t("settings.waldo.macAction")} — {row.project}</Button></li>)}</ul>
  {error && <p role="alert">{t("settings.waldo.outputUnavailable")}</p>}
  {output !== null && <pre className="max-h-96 overflow-auto whitespace-pre-wrap" data-ph-no-capture>{output || t("settings.waldo.noOutput")}</pre>}
 </section>;
}
