import { useTranslation } from "react-i18next";
import { useState } from "react";
import { useReviseOutcomeContract, type ContractRevisionRecord } from "../../hooks/useOutcome";
import { IntakeAuthorityEditor } from "./IntakeContractReview";
import { Button } from "../ui/button";

const lines = (value: string) => value.split("\n").map((line) => line.trim()).filter(Boolean);
const fieldClass = "mt-1 block min-h-20 w-full rounded-md border border-border bg-background p-2 text-sm";

/** Mounted against an immutable revision; CDC never overwrites an unsaved draft. */
export function MissionContractEditor({ outcomeId, contract, disabled }: {
 outcomeId: string; contract: ContractRevisionRecord; disabled: boolean;
}) {
 const { t } = useTranslation();
 const [draft, setDraft] = useState<ContractRevisionRecord | null>(null);
 if (!draft) return <Button data-testid="contract-edit" className="mb-3" variant="outline" disabled={disabled} onClick={() => setDraft(contract)}>{t("mission.editor.edit")}</Button>;
 return <div>{draft.id !== contract.id && <p role="alert" className="mb-2 text-sm">{t("mission.editor.changed")}</p>}<ContractDraft outcomeId={outcomeId} contract={draft} disabled={disabled || draft.id !== contract.id} onClose={() => setDraft(null)} /></div>;
}

function ContractDraft({ outcomeId, contract, disabled, onClose }: { outcomeId: string; contract: ContractRevisionRecord; disabled: boolean; onClose: () => void }) {
 const { t } = useTranslation();
 const [goal, setGoal] = useState(contract.goal);
 const [review, setReview] = useState(contract.review);
 const [constraints, setConstraints] = useState(contract.constraints.join("\n"));
 const [nonGoals, setNonGoals] = useState(contract.nonGoals.join("\n"));
 const [stops, setStops] = useState((contract.stopConditions ?? []).join("\n"));
 const [criteria, setCriteria] = useState(contract.criteria.map((criterion) => ({
  text: criterion.text,
  evidence: (contract.evidenceExpectations?.filter((item) => item.criterionId === criterion.criterionId).flatMap((item) => item.descriptions) ?? []).join("\n"),
 })));
 const [authority, setAuthority] = useState(contract.authorityCeiling ?? {
  readWorkspace: false, writeWorkspace: false, executeLocal: false, useNetwork: false,
  commitLocal: false, createPr: false, deploy: false, externalEffect: false,
 });
 const mutation = useReviseOutcomeContract(outcomeId);

 const busy = disabled || mutation.pending;
 return <form className="mb-4 space-y-4 rounded-lg border border-border bg-card p-4" onSubmit={(event) => {
  event.preventDefault();
  if (busy) return;
  void mutation.save({
   expectedRevision: contract.number, goal, review, successCriteria: criteria.map((item) => item.text),
   criterionEvidence: criteria.map((item) => lines(item.evidence)),
   constraints: lines(constraints), nonGoals: lines(nonGoals), stopConditions: lines(stops),
   authorityCeiling: authority, clarification: contract.clarification,
   temporalCondition: contract.temporalCondition ?? undefined, facets: contract.facets,
  }).then(onClose).catch(() => { /* The mutation exposes the daemon error below. */ });
 }}>
  <p className="text-sm">{t("mission.editor.note", { revision: contract.number + 1 })}</p>
  <fieldset disabled={busy} className="space-y-4">
   <label className="block text-sm">{t("mission.editor.goal")}<textarea data-testid="contract-goal" required className={fieldClass} value={goal} onChange={(e) => setGoal(e.target.value)} /></label>
   <section className="space-y-3" aria-label={t("mission.criteria")}>
    {criteria.map((criterion, index) => <div key={index} className="space-y-2 border-b border-border pb-3">
     <label className="block text-sm">{t("mission.editor.criterion", { number: index + 1 })}<textarea data-testid={`contract-criterion-${index}`} required className={fieldClass} value={criterion.text} onChange={(e) => setCriteria(criteria.map((item, i) => i === index ? { ...item, text: e.target.value } : item))} /></label>
     <label className="block text-sm">{t("mission.editor.evidence")}<textarea data-testid={`contract-evidence-${index}`} className={fieldClass} value={criterion.evidence} onChange={(e) => setCriteria(criteria.map((item, i) => i === index ? { ...item, evidence: e.target.value } : item))} /></label>
     <Button type="button" variant="ghost" disabled={criteria.length === 1} onClick={() => setCriteria(criteria.filter((_, i) => i !== index))}>{t("mission.editor.remove", { number: index + 1 })}</Button>
    </div>)}
    <Button type="button" variant="outline" onClick={() => setCriteria([...criteria, { text: "", evidence: "" }])}>{t("mission.editor.add")}</Button>
   </section>
   <label className="block text-sm">{t("mission.editor.review")}<textarea required className={fieldClass} value={review} onChange={(e) => setReview(e.target.value)} /></label>
   <label className="block text-sm">{t("mission.editor.constraints")}<textarea className={fieldClass} value={constraints} onChange={(e) => setConstraints(e.target.value)} /></label>
   <label className="block text-sm">{t("mission.editor.nonGoals")}<textarea className={fieldClass} value={nonGoals} onChange={(e) => setNonGoals(e.target.value)} /></label>
   <label className="block text-sm">{t("mission.editor.stops")}<textarea className={fieldClass} value={stops} onChange={(e) => setStops(e.target.value)} /></label>
   <section aria-label={t("mission.editor.permissions")}><h3 className="mb-2 text-sm font-medium">{t("mission.editor.permissions")}</h3><IntakeAuthorityEditor value={authority} onChange={setAuthority} /></section>
   <div className="flex gap-2"><Button data-testid="contract-save" type="submit">{t(mutation.pending ? "mission.editor.saving" : "mission.editor.save")}</Button></div>
  </fieldset>
  <Button data-testid="contract-discard" type="button" variant="ghost" disabled={mutation.pending} onClick={onClose}>{t("mission.editor.discard")}</Button>
  {mutation.failure && <p role="alert" className="text-sm text-destructive">{mutation.failure.message}</p>}
 </form>;
}
