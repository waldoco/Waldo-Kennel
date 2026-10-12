import { expect, test } from "@playwright/test";

const baseSnapshot = {
  conversation: { id: "conversation-e2e", projectId: "kennel-design", revision: 5, latestTurnSequence: 2, createdAt: "2026-08-26T10:00:00Z", updatedAt: "2026-08-26T10:02:00Z" },
  episodes: [{ id: "episode-e2e", conversationId: "conversation-e2e", projectId: "kennel-design", ordinal: 1, state: "active", createdAt: "2026-08-26T10:00:00Z" }],
  turns: [
    { id: "turn-e2e-1", conversationId: "conversation-e2e", episodeId: "episode-e2e", projectId: "kennel-design", sequence: 1, role: "user", message: "What changed after restart?", contextRefs: [], createdAt: "2026-08-26T10:01:00Z" },
    { id: "turn-e2e-2", conversationId: "conversation-e2e", episodeId: "episode-e2e", projectId: "kennel-design", sequence: 2, role: "waldo", message: "The durable Project conversation is intact.", contextRefs: [], createdAt: "2026-08-26T10:02:00Z" },
  ],
  contextAttachments: [{ id: "context-e2e", conversationId: "conversation-e2e", projectId: "kennel-design", ref: { kind: "project", objectId: "kennel-design", provenance: { kind: "user", sourceId: "waldo-rail" } }, attachedRevision: 3, active: true, createdAt: "2026-08-26T10:00:30Z" }],
  continuationReceipts: [
    { id: "receipt-safe", operationId: "operation-safe", conversationId: "conversation-e2e", projectId: "kennel-design", fromEpisodeId: "episode-old", toEpisodeId: "episode-e2e", fromAgentSessionRef: "session-old", toAgentSessionRef: "session-new", action: "automatic", reason: "context_reserve", reasonDetail: "Context reserve reached with unchanged bindings.", triggerEvidence: { kind: "provider_context_meter", reference: "meter-e2e" }, materialChange: false, changedFields: [], contextDigest: "a".repeat(64), contextRefs: [], previousBindings: {}, replacementBindings: {}, effectsKnown: true, oldSessionFenced: true, replacementIdentityConfirmed: true, fenceReceiptRef: "fence-e2e", reconciliationRef: "reconcile-e2e", createdAt: "2026-08-26T10:03:00Z" },
    { id: "receipt-needs", operationId: "operation-needs", conversationId: "conversation-e2e", projectId: "kennel-design", fromEpisodeId: "episode-e2e", fromAgentSessionRef: "session-new", action: "needs_you", reason: "fresh_verifier", reasonDetail: "Fresh verifier boundary.", triggerEvidence: { kind: "verifier_boundary", reference: "verifier-e2e" }, materialChange: false, changedFields: [], contextDigest: "b".repeat(64), contextRefs: [], previousBindings: {}, replacementBindings: {}, effectsKnown: true, oldSessionFenced: false, replacementIdentityConfirmed: false, needsUserReason: "Start a fresh verifier Attempt without inheriting implementer conclusions.", createdAt: "2026-08-26T10:04:00Z" },
  ],
};

test("Project Waldo restores offline durable truth and local intent across renderer re-entry", async ({ page }) => {
  await page.addInitScript((snapshot) => {
    localStorage.setItem("kennel.waldo.snapshot.kennel-design", JSON.stringify(snapshot));
  }, baseSnapshot);

  await page.goto("/#/projects/kennel-design");
  const launcher = page.getByRole("button", { name: "Open Waldo" });
  await launcher.click();

  const rail = page.getByRole("region", { name: "Waldo" });
  await expect(rail.getByText("kennel-design", { exact: true })).toBeVisible();
  await expect(rail.getByText("Offline · showing last durable snapshot")).toBeVisible();
  await expect(rail.getByRole("log", { name: "Project Waldo conversation" }).getByRole("article")).toHaveCount(2);
  await expect(rail.getByText("Continued safely")).toBeVisible();
  await expect(rail.getByText("Needs you")).toBeVisible();

  await rail.getByRole("textbox", { name: "Message Waldo" }).fill("Read this back after another restart");
  await expect(rail.getByText("Draft stays local; it is not queued or sent.")).toBeVisible();

  await page.reload();
  await page.getByRole("button", { name: "Open Waldo" }).click();
  await expect(page.getByRole("region", { name: "Waldo" }).getByRole("textbox", { name: "Message Waldo" })).toHaveValue("Read this back after another restart");

  await page.setViewportSize({ width: 960, height: 760 });
  const reopenedRail = page.getByRole("region", { name: "Waldo" });
  const railBox = await reopenedRail.boundingBox();
  const centerBox = await page.locator(".center-panel-surface").boundingBox();
  expect(railBox).not.toBeNull();
  expect(centerBox).not.toBeNull();
  expect(railBox!.width).toBeLessThanOrEqual(centerBox!.width + 1);
  expect(await page.evaluate(() => document.documentElement.scrollWidth > document.documentElement.clientWidth)).toBe(false);

  await reopenedRail.getByRole("button", { name: "Close Waldo" }).click();
  await expect(launcher).toBeFocused();
});

test("Work search selection scopes Waldo to its durable Project conversation", async ({ page }) => {
  await page.addInitScript((snapshot) => {
    localStorage.setItem("kennel.waldo.snapshot.kennel-design", JSON.stringify(snapshot));
  }, baseSnapshot);

  await page.goto("/#/work?project=kennel-design");
  await page.getByRole("button", { name: "Open Waldo" }).click();

  const rail = page.getByRole("region", { name: "Waldo" });
  await expect(rail.getByText("kennel-design", { exact: true })).toBeVisible();
  await expect(rail.getByText("Offline · showing last durable snapshot")).toBeVisible();
  await expect(rail.getByRole("log", { name: "Project Waldo conversation" }).getByRole("article")).toHaveCount(2);
});
