import { expect, test } from "@playwright/test";

test.use({
  userAgent:
    "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
  viewport: { width: 1512, height: 982 },
});

test("Home gives Waldo the inspector and restores the Today glance exactly", async ({ page }) => {
  await page.goto("/#/home?homePhase=morning&homeContext=catch_up");

  const launcher = page.getByRole("button", { name: "Open Waldo" });
  const glance = page.getByRole("complementary", { name: "Today at a glance" });
  await expect(glance).toBeVisible();
  await launcher.click();

  await expect(page.getByRole("region", { name: "Waldo" })).toBeVisible();
  await expect(glance).toHaveCount(0);
  await expect(page.getByRole("textbox", { name: "Quick capture" })).toHaveCount(1);

  await page.getByRole("button", { name: "Close Waldo" }).click();
  await expect(glance).toBeVisible();
  await expect(launcher).toBeFocused();
});

test("the configured shortcut opens the same global Waldo rail", async ({ page }) => {
  await page.goto("/#/home");
  await expect(page.getByRole("button", { name: "Open Waldo" })).toBeVisible();

  await page.keyboard.press("Meta+Shift+Space");

  await expect(page.getByRole("region", { name: "Waldo" })).toBeVisible();
  await expect(page.getByRole("button", { name: "Open Waldo" })).toHaveAttribute(
    "aria-expanded",
    "true",
  );
});

test("Conversation episodes preserve context and semantic boundaries", async ({ page }) => {
  await page.goto("/#/home");
  await page.getByRole("button", { name: "Open Waldo" }).click();

  await expect(page.getByRole("status", { name: "Waldo preview boundary" })).toContainText(
    "No model or agent is running",
  );
  await expect(page.getByRole("tab", { name: "Conversation" })).toHaveAttribute(
    "aria-selected",
    "true",
  );
  await expect(page.getByText("Contextual", { exact: true })).toBeVisible();
  await expect(page.getByRole("region", { name: "Observation preview" })).toContainText(
    "Candidate",
  );
  await expect(page.getByRole("region", { name: "Proposal preview" })).toContainText(
    "Needs you",
  );
  await page.getByRole("button", { name: "Review proposed command" }).click();
  await expect(page.getByRole("status", { name: "Proposal preview status" })).toContainText(
    "Nothing was created, sent, or saved",
  );

  await page.getByRole("button", { name: "Detach context" }).click();
  await expect(page.getByText("No attached context")).toBeVisible();

  await page.getByRole("button", { name: "Launch readiness" }).click();
  const result = page.getByRole("region", { name: "Result preview" });
  await expect(result).toContainText("Result ready");
  await expect(result).toContainText("Outcome · Unknown");
  await page.getByRole("button", { name: "Show result detail" }).click();
  await expect(result).toContainText("No verification or AcceptanceDecision exists");
});

test("Activity exposes one bounded delegated run under Waldo", async ({ page }) => {
  await page.goto("/#/home");
  await page.getByRole("button", { name: "Open Waldo" }).click();

  const conversationTab = page.getByRole("tab", { name: "Conversation" });
  await conversationTab.focus();
  await page.keyboard.press("ArrowRight");
  await expect(page.getByRole("tab", { name: "Activity" })).toBeFocused();

  const activity = page.getByRole("region", { name: "Agent activity preview" });
  await expect(activity).toContainText("Running");
  await expect(activity).toContainText("Current step");
  await page.getByRole("button", { name: "Inspect run" }).click();
  await expect(activity).toContainText("Bounded research specialist");
  await expect(activity).toContainText("No external messages, mutations, or acceptance");
  await expect(activity).toContainText("Calendar, decision note, current Work context");
  await expect(activity).toContainText("Returns to Work · no Outcome created");
  await expect(activity).toContainText("Result ready · Outcome · Unknown");
  await expect(activity).not.toContainText("Verified outcome");
});

test("Activity previews a governed specialist without creating or running one", async ({ page }) => {
  await page.goto("/#/work");
  await page.getByRole("button", { name: "Open Waldo" }).click();
  await expect(page.getByRole("button", { name: "Create specialist" })).toHaveCount(0);
  await page.getByRole("tab", { name: "Activity" }).click();
  await page.getByRole("button", { name: "Create specialist" }).click();

  const builder = page.getByRole("region", { name: "Specialist preview builder" });
  await expect(builder.getByLabel("Authority ceiling")).toHaveValue(
    "Read and draft only; no outward action",
  );
  await expect(builder.getByLabel("Waldo return destination")).toHaveValue("Work");
  await builder.getByRole("button", { name: "Preview specialist" }).click();

  const specialist = page.getByRole("article", { name: "Research brief specialist" });
  await expect(specialist).toContainText("Ready · preview only");
  await expect(specialist).toContainText("Nothing was created, connected, run, or saved");
  await specialist.getByRole("button", { name: "Pause specialist" }).click();
  await expect(specialist).toContainText("Paused · preview only");
  await specialist.getByRole("button", { name: "Revoke specialist" }).click();
  await expect(specialist).toContainText("Revoked · preview only");
  await page.setViewportSize({ width: 960, height: 760 });
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth > document.documentElement.clientWidth,
    ),
  ).toBe(false);
});

test("wide Home constrains Waldo to the context region and keeps its body scrollable", async ({ page }) => {
  await page.goto("/#/home");
  await page.getByRole("button", { name: "Open Waldo" }).click();
  await page.getByRole("tab", { name: "Activity" }).click();
  await page.getByRole("button", { name: "Inspect run" }).click();

  const rail = page.getByRole("region", { name: "Waldo" });
  const activity = rail.getByRole("region", { name: "Agent activity preview" });
  const railBox = await rail.boundingBox();
  const centerBox = await page.locator(".center-panel-surface").boundingBox();
  expect(railBox).not.toBeNull();
  expect(centerBox).not.toBeNull();
  expect(railBox!.width).toBeLessThanOrEqual(442);
  expect(railBox!.y + railBox!.height).toBeLessThanOrEqual(centerBox!.y + centerBox!.height + 1);

  const body = rail.getByTestId("waldo-rail-body");
  await expect.poll(() => body.evaluate((element) => element.scrollHeight > element.clientHeight)).toBe(true);
  await body.evaluate((element) => element.scrollTo({ top: element.scrollHeight }));
  await expect(activity.getByText("Result ready · Outcome · Unknown")).toBeVisible();
});

test("Home exposes completed continuity screens without promoting them to primary", async ({ page }) => {
  await page.goto("/#/home");

  const primary = page.getByRole("group", { name: "Primary Home destinations" });
  await expect(primary.getByRole("link", { name: "Today" })).toBeVisible();
  await expect(primary.getByRole("link", { name: "Open loops" })).toBeVisible();

  const continuity = page.getByRole("group", { name: "Review and continuity" });
  await continuity.getByRole("link", { name: "Daily close" }).click();
  await expect(page.getByRole("heading", { name: "Close the day deliberately" })).toBeVisible();
  await expect(page.getByRole("textbox", { name: "Quick capture" })).toHaveCount(0);

  await page.getByRole("link", { name: "Memory review" }).click();
  await expect(page.getByRole("heading", { name: "Memory review" })).toBeVisible();
  await page.getByRole("link", { name: "Insights" }).click();
  await expect(page.getByRole("heading", { name: "Insights" })).toBeVisible();
});

test("Insights keeps interpretation review separate from inspectable Records", async ({ page }) => {
  await page.goto("/#/home/history");

  await expect(page.getByRole("status", { name: "Insights preview boundary" })).toContainText(
    "deterministic local examples",
  );
  await page.getByRole("button", { name: "Why this?" }).click();
  await expect(
    page.getByText(/use the open block for the deck revision/i),
  ).toBeVisible();
  await page.getByRole("button", { name: "Confirm insight" }).click();
  await expect(page.getByText("Confirmed", { exact: true })).toBeVisible();

  await page.getByRole("tab", { name: "Records" }).click();
  await expect(page.getByRole("region", { name: "Records" })).toContainText(
    "inspectable evidence",
  );
  await expect(page.getByRole("tab", { name: "Continuity" })).toHaveAttribute(
    "aria-selected",
    "true",
  );
});

test("narrow Waldo becomes a full-content layer with a visible Back action", async ({ page }) => {
  // Electron enforces a 960px minimum window width. The center panel is still
  // narrow after the Home sidebar, so this is the real desktop boundary.
  await page.setViewportSize({ width: 960, height: 760 });
  await page.goto("/#/home/open-loops");
  await page.getByRole("button", { name: "Open Waldo" }).click();

  const rail = page.getByRole("region", { name: "Waldo" });
  await expect(rail).toBeVisible();
  await expect(rail.getByText("Back", { exact: true })).toBeVisible();

  const railBox = await rail.boundingBox();
  const centerBox = await page.locator(".center-panel-surface").boundingBox();
  expect(railBox).not.toBeNull();
  expect(centerBox).not.toBeNull();
  expect(Math.abs(railBox!.width - centerBox!.width)).toBeLessThanOrEqual(2);

  const hasOverflow = await page.evaluate(
    () => document.documentElement.scrollWidth > document.documentElement.clientWidth,
  );
  expect(hasOverflow).toBe(false);
});

test("Work opens one Waldo right region and retains the Home Work switch", async ({ page }) => {
  await page.goto("/#/work");

  await page.getByRole("button", { name: "Open Waldo" }).click();

  await expect(page.getByRole("region", { name: "Waldo" })).toHaveCount(1);
  await expect(page.getByRole("navigation", { name: "Waldo mode" })).toBeVisible();
  await expect(page.getByRole("button", { name: "Work", exact: true })).toHaveAttribute(
    "aria-pressed",
    "true",
  );
});
