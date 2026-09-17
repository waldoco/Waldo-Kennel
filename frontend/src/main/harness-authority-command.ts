export type HarnessPairingDecisionProposal = {
  action: "approve" | "deny";
  intentId: string;
  digest: string;
  requestKey: string;
};
export type HarnessConnectionRevokeProposal = {
  action: "revoke";
  connectionId: string;
  digest: string;
  expectedGeneration: number;
  requestKey: string;
};
const hex = /^[0-9a-f]{64}$/;
function exact(x: unknown, keys: string[]): x is Record<string, unknown> {
  return (
    !!x &&
    typeof x === "object" &&
    !Array.isArray(x) &&
    Object.keys(x).length === keys.length &&
    keys.every((k) => Object.hasOwn(x, k))
  );
}
function text(x: unknown, max = 256) {
  return typeof x === "string" && !!x.trim() && x.length <= max;
}
export function parseHarnessAuthorityProposal(
  x: unknown,
): HarnessPairingDecisionProposal | HarnessConnectionRevokeProposal | null {
  if (
    exact(x, ["action", "intentId", "digest", "requestKey"]) &&
    (x.action === "approve" || x.action === "deny") &&
    text(x.intentId) &&
    text(x.requestKey) &&
    typeof x.digest === "string" &&
    hex.test(x.digest)
  )
    return {
      action: x.action,
      intentId: String(x.intentId).trim(),
      digest: x.digest,
      requestKey: String(x.requestKey).trim(),
    };
  if (
    exact(x, [
      "action",
      "connectionId",
      "digest",
      "expectedGeneration",
      "requestKey",
    ]) &&
    x.action === "revoke" &&
    text(x.connectionId) &&
    text(x.requestKey) &&
    typeof x.digest === "string" &&
    hex.test(x.digest) &&
    Number.isSafeInteger(x.expectedGeneration) &&
    Number(x.expectedGeneration) > 0
  )
    return {
      action: "revoke",
      connectionId: String(x.connectionId).trim(),
      digest: x.digest,
      expectedGeneration: Number(x.expectedGeneration),
      requestKey: String(x.requestKey).trim(),
    };
  return null;
}
