# Instructions

- Following Playwright test failed.
- Explain why, be concise, respect Playwright best practices.
- Provide a snippet of code with the fix, if possible.

# Test info

- Name: home-personal-agent-flows.spec.ts >> morning Catch Up preserves exact provenance return and one Quick Capture
- Location: e2e/home-personal-agent-flows.spec.ts:9:5

# Error details

```
Test timeout of 30000ms exceeded.
```

```
Error: locator.click: Test timeout of 30000ms exceeded.
Call log:
  - waiting for getByRole('button', { name: 'Inspect source' })
    - locator resolved to <button type="button" class="text-xs font-medium text-foreground underline underline-offset-4 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/60">Inspect source</button>
  - attempting click action
    2 × waiting for element to be visible, enabled and stable
      - element is visible, enabled and stable
      - scrolling into view if needed
      - done scrolling
      - <div role="status" aria-labelledby="home-coming-soon-heading" class="absolute inset-0 z-10 flex items-center justify-center bg-background/85 backdrop-blur-sm">…</div> intercepts pointer events
    - retrying click action
    - waiting 20ms
    2 × waiting for element to be visible, enabled and stable
      - element is visible, enabled and stable
      - scrolling into view if needed
      - done scrolling
      - <div role="status" aria-labelledby="home-coming-soon-heading" class="absolute inset-0 z-10 flex items-center justify-center bg-background/85 backdrop-blur-sm">…</div> intercepts pointer events
    - retrying click action
      - waiting 100ms
    55 × waiting for element to be visible, enabled and stable
       - element is visible, enabled and stable
       - scrolling into view if needed
       - done scrolling
       - <div role="status" aria-labelledby="home-coming-soon-heading" class="absolute inset-0 z-10 flex items-center justify-center bg-background/85 backdrop-blur-sm">…</div> intercepts pointer events
     - retrying click action
       - waiting 500ms

```

# Page snapshot

```yaml
- generic [ref=e4]:
  - generic [ref=e5]:
    - generic [ref=e8]:
      - generic [ref=e10]:
        - button "Outcomes" [ref=e11] [cursor=pointer]:
          - img [ref=e12]
        - generic [ref=e13]: Kennel
      - navigation "Home destinations" [ref=e17]:
        - group "Primary Home destinations" [ref=e18]:
          - link "Today" [ref=e19] [cursor=pointer]:
            - /url: /#/home
          - link "Chat" [ref=e20] [cursor=pointer]:
            - /url: /#/home/chat
          - link "Open Loops" [ref=e21] [cursor=pointer]:
            - /url: /#/home/open-loops
        - group "Review and continuity" [ref=e22]:
          - paragraph [ref=e23]: Review and continuity
          - link "Daily Close" [ref=e24] [cursor=pointer]:
            - /url: /#/home/daily-close
          - link "Memory Review" [ref=e25] [cursor=pointer]:
            - /url: /#/home/memory
          - link "Insights" [ref=e26] [cursor=pointer]:
            - /url: /#/home/history
      - generic [ref=e27]:
        - generic [ref=e28]: daemon stopped
        - button "Settings" [ref=e30] [cursor=pointer]:
          - img [ref=e31]
          - generic [ref=e34]: Settings
        - generic:
          - button:
            - img
    - main [ref=e36]:
      - generic [ref=e38]:
        - region "Home" [ref=e42]:
          - heading "Home" [level=1] [ref=e43]
          - generic [ref=e44]:
            - generic [ref=e46]:
              - status "Home availability" [ref=e47]:
                - paragraph [ref=e50]: Capture is paused
                - paragraph [ref=e51]: This preview uses explicit fixture context only.
              - generic [ref=e53]:
                - generic [ref=e54]:
                  - paragraph [ref=e55]: Saturday, 22 August
                  - heading "Good morning, Shivansh." [level=2] [ref=e56]
                  - generic [ref=e57]:
                    - paragraph [ref=e58]: One thing needs you today.
                    - button "Review the deck follow-up" [ref=e59] [cursor=pointer]: Review →
                - region "Morning brief" [ref=e60]:
                  - generic [ref=e61]:
                    - heading "Morning brief" [level=3] [ref=e62]
                    - generic [ref=e63]: Architecture preview
                  - generic [ref=e64]:
                    - paragraph [ref=e65]: One proposed commitment needs your judgment before it becomes a responsibility.
                    - paragraph [ref=e66]: The pitch-deck work remains separate until you explicitly continue it in Work.
                    - paragraph [ref=e67]: Two confirmed items are waiting; neither needs action right now.
                - generic [ref=e68]:
                  - generic [ref=e69]:
                    - heading "To do" [level=3] [ref=e70]
                    - list "To do" [ref=e71]:
                      - listitem [ref=e72]:
                        - generic [ref=e74]: Prepare the revised deck
                      - listitem [ref=e75]:
                        - generic [ref=e77]: Review the meeting decision
                      - listitem [ref=e78]:
                        - generic [ref=e80]: Confirm tomorrow's follow-up
                  - generic [ref=e81]:
                    - generic [ref=e82]:
                      - heading "Waldo suggests" [level=3] [ref=e83]
                      - generic [ref=e84]: Proposed, not added
                    - list "Waldo suggests" [ref=e85]:
                      - listitem [ref=e86]:
                        - generic [ref=e87]: +
                        - generic [ref=e88]: Draft a reply to Ashish
                      - listitem [ref=e89]:
                        - generic [ref=e90]: +
                        - generic [ref=e91]: Collect the latest deck evidence
                      - listitem [ref=e92]:
                        - generic [ref=e93]: +
                        - generic [ref=e94]: Prepare a Work handoff
              - region "Anything on your mind?" [ref=e96]:
                - generic [ref=e97]:
                  - heading "Anything on your mind?" [level=2] [ref=e98]
                  - generic [ref=e99]: Explicit note · Home
                - generic [ref=e100]:
                  - generic [ref=e101]: Quick Capture
                  - textbox "Quick Capture" [ref=e102]:
                    - /placeholder: Capture a thought…
                  - generic [ref=e103]: ⌘↵
                - paragraph [ref=e104]: Architecture preview — nothing is saved or turned into a responsibility.
            - complementary [ref=e105]:
              - region "Catch Up" [ref=e106]:
                - generic [ref=e107]:
                  - paragraph [ref=e108]: Saturday, 22 August
                  - heading "Catch Up" [level=2] [ref=e109]
                  - generic [ref=e110]:
                    - generic [ref=e111]: One decision · Architecture preview
                    - generic [ref=e112]: 1 of 1
                - article "Catch Up decision" [ref=e114]:
                  - paragraph [ref=e115]: Preview context — not live data
                  - generic [ref=e116]:
                    - generic [ref=e117]:
                      - paragraph [ref=e118]: User statement
                      - paragraph [ref=e119]: I'll send Ashish the revised deck tomorrow.
                    - generic [ref=e120]:
                      - paragraph [ref=e121]: Waldo proposal
                      - paragraph [ref=e122]: Prepare the revised deck for Ashish; do not send it.
                      - paragraph [ref=e123]: Proposed wording only. Your correction outranks the inference.
                    - generic [ref=e124]:
                      - paragraph [ref=e125]: Known capture gap
                      - paragraph [ref=e126]: Meeting audio unavailable from 3:10–3:24 PM. Waldo cannot know what changed during that interval.
                    - generic [ref=e127]:
                      - paragraph [ref=e128]: Source summary
                      - paragraph [ref=e129]: Meeting note · user-stated commitment · 3:08 PM
                      - paragraph [ref=e130]: Source material and Waldo's interpretation remain separate.
                      - button "Inspect source" [ref=e132] [cursor=pointer]
                  - generic [ref=e133]:
                    - button "Correct" [ref=e134] [cursor=pointer]
                    - button "Keep as note" [ref=e135] [cursor=pointer]
                    - button "Confirm Open Loop" [ref=e136] [cursor=pointer]
                    - button "Defer" [ref=e137] [cursor=pointer]
                    - button "Dismiss" [ref=e138] [cursor=pointer]
                  - button "Continue in Work" [ref=e139] [cursor=pointer]
                - button "Finish morning brief →" [ref=e141] [cursor=pointer]
        - status "Home is getting a new look" [ref=e142]:
          - generic [ref=e143]:
            - generic [ref=e144]: Coming soon
            - heading "Home is getting a new look" [level=2] [ref=e145]
            - paragraph [ref=e146]: We're rebuilding Home to match the new design system. Your data and settings are untouched.
  - generic [ref=e148]:
    - button "Collapse sidebar" [ref=e149] [cursor=pointer]:
      - img [ref=e150]
    - button "Go back" [disabled] [ref=e152]:
      - img [ref=e153]
    - button "Go forward" [disabled] [ref=e155]:
      - img [ref=e156]
```

# Test source

```ts
  1   | import { expect, test } from "@playwright/test";
  2   | 
  3   | test.use({
  4   | 	userAgent:
  5   | 		"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
  6   | 	viewport: { width: 1512, height: 982 },
  7   | });
  8   | 
  9   | test("morning Catch Up preserves exact provenance return and one Quick Capture", async ({ page }) => {
  10  | 	await page.goto("/#/home?homePhase=morning&homeContext=catch_up");
  11  | 
  12  | 	await expect(page.getByRole("heading", { name: "Good morning, Shivansh." })).toBeVisible();
  13  | 	await expect(page.getByRole("heading", { name: "Catch Up" })).toBeVisible();
  14  | 	await expect(page.getByRole("textbox", { name: "Quick Capture" })).toHaveCount(1);
  15  | 
  16  | 	const inspect = page.getByRole("button", { name: "Inspect source" });
> 17  | 	await inspect.click();
      |                ^ Error: locator.click: Test timeout of 30000ms exceeded.
  18  | 	await expect(page.getByRole("dialog", { name: "Source provenance" })).toBeVisible();
  19  | 	await page.keyboard.press("Escape");
  20  | 	await expect(inspect).toBeFocused();
  21  | });
  22  | 
  23  | test("afternoon Today recalibrates before the next commitment", async ({ page }) => {
  24  | 	await page.goto("/#/home?homePhase=afternoon&homeContext=before_next");
  25  | 
  26  | 	await expect(page.getByRole("heading", { name: "Good afternoon, Shivansh." })).toBeVisible();
  27  | 	await expect(page.getByRole("heading", { name: "Before your next thing" })).toBeVisible();
  28  | 	await expect(page.getByText("Pricing workshop", { exact: true })).toBeVisible();
  29  | 	await expect(page.getByPlaceholder("Note what changed…")).toBeVisible();
  30  | });
  31  | 
  32  | test("evening Today reaches a truthful Daily Close receipt and Insights", async ({ page }) => {
  33  | 	await page.goto("/#/home?homePhase=evening&homeContext=evening_review");
  34  | 
  35  | 	await expect(page.getByRole("heading", { name: "Good evening, Shivansh." })).toBeVisible();
  36  | 	await page.getByRole("link", { name: "Start Closure" }).click();
  37  | 	await expect(page.getByRole("heading", { name: "Close the day deliberately" })).toBeVisible();
  38  | 	await page.getByRole("button", { name: "Review complete — preview Daily Close" }).click();
  39  | 	await expect(page.getByRole("heading", { name: "Daily Close preview" })).toBeVisible();
  40  | 	await expect(page.getByText("Preview receipt — nothing was saved or closed")).toBeVisible();
  41  | 	await page.getByRole("link", { name: "Inspect Insights" }).click();
  42  | 	await expect(page.getByRole("heading", { name: "Insights" })).toBeVisible();
  43  | });
  44  | 
  45  | test("Open Loops explains responsibility and keeps Work handoff preview-only", async ({ page }) => {
  46  | 	await page.goto("/#/home/open-loops");
  47  | 
  48  | 	await page.getByRole("button", { name: "Deck follow-up" }).click();
  49  | 	await expect(page.getByText("Prepare the revised deck for Ashish; do not send it yet.")).toBeVisible();
  50  | 	await page.getByRole("button", { name: "Continue in Work" }).click();
  51  | 	await expect(page.getByRole("status", { name: "Open Loop preview status" })).toContainText(
  52  | 		"No Work Outcome or responsibility link has been created",
  53  | 	);
  54  | 	await expect(page.getByRole("textbox", { name: "Quick Capture" })).toHaveCount(0);
  55  | });
  56  | 
  57  | test("Memory Review rejects only the local candidate preview", async ({ page }) => {
  58  | 	await page.goto("/#/home/memory");
  59  | 
  60  | 	await expect(page.getByText("Candidate — not memory")).toBeVisible();
  61  | 	await page.getByRole("button", { name: "Reject" }).click();
  62  | 	await expect(page.getByRole("status", { name: "Memory review status" })).toContainText(
  63  | 		"Rejected in this preview — no durable memory changed",
  64  | 	);
  65  | 	await expect(page.getByRole("textbox", { name: "Quick Capture" })).toHaveCount(0);
  66  | });
  67  | 
  68  | test("Home and Work return to the last meaningful route in each mode", async ({ page }) => {
  69  | 	await page.goto("/#/home/open-loops");
  70  | 
  71  | 	await page.getByRole("button", { name: "Work", exact: true }).click();
  72  | 	await expect(page).toHaveURL(/\/#\/work$/);
  73  | 	await page.getByRole("button", { name: "Home", exact: true }).click();
  74  | 	await expect(page).toHaveURL(/\/#\/home\/open-loops$/);
  75  | });
  76  | 
  77  | test("narrow Home replaces the index with detail and returns exact focus", async ({ page }) => {
  78  | 	await page.setViewportSize({ width: 720, height: 760 });
  79  | 	await page.goto("/#/home/open-loops");
  80  | 
  81  | 	const row = page.getByRole("button", { name: "Deck follow-up" });
  82  | 	await row.click();
  83  | 	const back = page.getByRole("button", { name: "Back to Open Loops" });
  84  | 	await expect(back).toBeVisible();
  85  | 	await back.click();
  86  | 	await expect(row).toBeFocused();
  87  | 
  88  | 	const hasOverflow = await page.evaluate(
  89  | 		() => document.documentElement.scrollWidth > document.documentElement.clientWidth,
  90  | 	);
  91  | 	expect(hasOverflow).toBe(false);
  92  | });
  93  | 
  94  | test("Home never exposes Work project or session navigation", async ({ page }) => {
  95  | 	await page.setViewportSize({ width: 900, height: 760 });
  96  | 	await page.goto("/#/home/history");
  97  | 
  98  | 	await expect(page.getByRole("navigation", { name: "Home destinations" })).toBeVisible();
  99  | 	await expect(page.getByRole("link", { name: "Today" })).toBeVisible();
  100 | 	await expect(page.getByRole("link", { name: "Open Loops" })).toBeVisible();
  101 | 	await expect(page.getByRole("link", { name: /projects/i })).toHaveCount(0);
  102 | 	await expect(page.getByRole("link", { name: /sessions/i })).toHaveCount(0);
  103 | 
  104 | 	const hasOverflow = await page.evaluate(
  105 | 		() => document.documentElement.scrollWidth > document.documentElement.clientWidth,
  106 | 	);
  107 | 	expect(hasOverflow).toBe(false);
  108 | });
  109 | 
```