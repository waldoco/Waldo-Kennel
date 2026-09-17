import { describe, expect, it } from "vitest";
import { parseHarnessAuthorityProposal } from "./harness-authority-command";
const digest = "a".repeat(64);
describe("parseHarnessAuthorityProposal", () => {
  it("accepts only exact pairing decisions", () => {
    expect(
      parseHarnessAuthorityProposal({
        action: "approve",
        intentId: " i ",
        digest,
        requestKey: " k ",
      }),
    ).toEqual({ action: "approve", intentId: "i", digest, requestKey: "k" });
  });
  it("accepts exact revoke", () => {
    expect(
      parseHarnessAuthorityProposal({
        action: "revoke",
        connectionId: "c",
        digest,
        expectedGeneration: 2,
        requestKey: "k",
      }),
    ).toEqual({
      action: "revoke",
      connectionId: "c",
      digest,
      expectedGeneration: 2,
      requestKey: "k",
    });
  });
  it.each([
    null,
    {},
    { action: "approve", intentId: "i", digest, requestKey: "k", extra: true },
    { action: "approve", intentId: "i", digest: "bad", requestKey: "k" },
    {
      action: "revoke",
      connectionId: "c",
      digest,
      expectedGeneration: 0,
      requestKey: "k",
    },
  ])("rejects invalid/expanded values", (x) =>
    expect(parseHarnessAuthorityProposal(x)).toBeNull(),
  );
});
