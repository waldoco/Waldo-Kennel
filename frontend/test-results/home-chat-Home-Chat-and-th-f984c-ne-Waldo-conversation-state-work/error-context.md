# Instructions

- Following Playwright test failed.
- Explain why, be concise, respect Playwright best practices.
- Provide a snippet of code with the fix, if possible.

# Test info

- Name: home-chat.spec.ts >> Home Chat and the global rail preserve one Waldo conversation state
- Location: e2e/home-chat.spec.ts:3:5

# Error details

```
Test timeout of 30000ms exceeded.
```

```
Error: locator.click: Test timeout of 30000ms exceeded.
Call log:
  - waiting for getByRole('button', { name: 'Open Waldo' })

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
          - link "Today" [active] [ref=e19] [cursor=pointer]:
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
                  - heading "Good evening, Shivansh." [level=2] [ref=e56]
                  - generic [ref=e57]:
                    - paragraph [ref=e58]: One open responsibility is worth carrying deliberately.
                    - button "Review the evening transition" [ref=e59] [cursor=pointer]: Review →
                - region "Evening brief" [ref=e60]:
                  - generic [ref=e61]:
                    - heading "Evening brief" [level=3] [ref=e62]
                    - generic [ref=e63]: Architecture preview
                  - generic [ref=e64]:
                    - paragraph [ref=e65]: The meeting decision was corrected and the revised-deck responsibility remains open.
                    - paragraph [ref=e66]: No Work Outcome or external send was created from today's proposal.
                    - paragraph [ref=e67]: One known capture gap should remain visible if you choose to close the day.
                - generic [ref=e68]:
                  - generic [ref=e69]:
                    - heading "Before you close" [level=3] [ref=e70]
                    - list "Before you close" [ref=e71]:
                      - listitem [ref=e72]:
                        - generic [ref=e74]: Review the deck responsibility
                      - listitem [ref=e75]:
                        - generic [ref=e77]: Choose what carries forward
                      - listitem [ref=e78]:
                        - generic [ref=e80]: Set tomorrow's re-entry
                  - generic [ref=e81]:
                    - generic [ref=e82]:
                      - heading "For tomorrow" [level=3] [ref=e83]
                      - generic [ref=e84]: Proposed, not added
                    - list "For tomorrow" [ref=e85]:
                      - listitem [ref=e86]:
                        - generic [ref=e87]: +
                        - generic [ref=e88]: Resume from the corrected deck follow-up
                      - listitem [ref=e89]:
                        - generic [ref=e90]: +
                        - generic [ref=e91]: Carry the source gap into tomorrow
                      - listitem [ref=e92]:
                        - generic [ref=e93]: +
                        - generic [ref=e94]: Start an explicit Closure review
              - region "Anything on your mind?" [ref=e96]:
                - generic [ref=e97]:
                  - heading "Anything on your mind?" [level=2] [ref=e98]
                  - generic [ref=e99]: Explicit note · Home
                - generic [ref=e100]:
                  - generic [ref=e101]: Quick Capture
                  - textbox "Quick Capture" [ref=e102]:
                    - /placeholder: Leave context for tomorrow…
                  - generic [ref=e103]: ⌘↵
                - paragraph [ref=e104]: Architecture preview — nothing is saved or turned into a responsibility.
            - complementary [ref=e105]:
              - region "Evening review" [ref=e106]:
                - generic [ref=e107]:
                  - paragraph [ref=e108]: Transition, not automatic closure
                  - heading "Evening review" [level=2] [ref=e109]
                  - paragraph [ref=e110]: Architecture preview · no live data or canonical state
                - generic [ref=e111]:
                  - generic [ref=e112]:
                    - region "What became true" [ref=e113]:
                      - heading "What became true" [level=3] [ref=e114]
                      - generic [ref=e115]:
                        - paragraph [ref=e116]: The deck instruction was corrected
                        - paragraph [ref=e117]: Prepare the revision; do not send it yet.
                    - region "Still unresolved" [ref=e118]:
                      - heading "Still unresolved" [level=3] [ref=e119]
                      - generic [ref=e120]:
                        - paragraph [ref=e121]: Deck follow-up
                        - paragraph [ref=e122]: The revision still needs preparation and review.
                    - region "Known source gap" [ref=e123]:
                      - heading "Known source gap" [level=3] [ref=e124]
                      - paragraph [ref=e125]: Meeting audio was unavailable from 3:10–3:24 PM.
                  - paragraph [ref=e126]: Closure begins only when you choose it. Reviewing this panel changes nothing.
                - link "Start Closure" [ref=e128] [cursor=pointer]:
                  - /url: "#/home/daily-close"
        - status "Home is getting a new look" [ref=e129]:
          - generic [ref=e130]:
            - generic [ref=e131]: Coming soon
            - heading "Home is getting a new look" [level=2] [ref=e132]
            - paragraph [ref=e133]: We're rebuilding Home to match the new design system. Your data and settings are untouched.
  - generic [ref=e134]:
    - button "Collapse sidebar" [ref=e135] [cursor=pointer]:
      - img [ref=e136]
    - button "Go back" [ref=e138] [cursor=pointer]:
      - img [ref=e139]
    - button "Go forward" [disabled] [ref=e141]:
      - img [ref=e142]
```

# Test source

```ts
  1  | import { expect, test } from "@playwright/test";
  2  | 
  3  | test("Home Chat and the global rail preserve one Waldo conversation state", async ({ page }) => {
  4  | 	await page.goto("/#/home/chat");
  5  | 
  6  | 	await expect(page.getByRole("link", { name: "Chat" })).toHaveAttribute("aria-current", "page");
  7  | 	await page.getByRole("textbox", { name: "Message Waldo" }).fill("Keep this local draft");
  8  | 	await page.getByRole("tab", { name: "Activity" }).click();
  9  | 
  10 | 	await page.getByRole("link", { name: "Today" }).click();
> 11 | 	await page.getByRole("button", { name: "Open Waldo" }).click();
     |                                                         ^ Error: locator.click: Test timeout of 30000ms exceeded.
  12 | 	await expect(page.getByRole("tab", { name: "Activity" })).toHaveAttribute("aria-selected", "true");
  13 | 
  14 | 	await page.getByRole("tab", { name: "Conversation" }).click();
  15 | 	await expect(page.getByRole("textbox", { name: "Message Waldo" })).toHaveValue("Keep this local draft");
  16 | });
  17 | 
  18 | test("Home Chat answers a deterministic preview with source lineage and hands off to Activity", async ({ page }) => {
  19 | 	await page.goto("/#/home/chat");
  20 | 
  21 | 	await page.getByRole("textbox", { name: "Message Waldo" }).fill(
  22 | 		"What changed in the pricing workshop and what still needs me?",
  23 | 	);
  24 | 	await page.getByRole("button", { name: "Send preview" }).click();
  25 | 
  26 | 	await expect(page.getByText(/Two decisions changed/)).toBeVisible();
  27 | 	const decisionSource = page.getByRole("link", { name: "Pricing decision note" });
  28 | 	await expect(decisionSource).toBeVisible();
  29 | 	await expect(page.getByRole("link", { name: "Workshop calendar event" })).toBeVisible();
  30 | 	await expect(page.getByText("Local fixture only · no model, provider, send, or save")).toBeVisible();
  31 | 
  32 | 	await page.getByRole("button", { name: "Review in Activity" }).click();
  33 | 	await expect(page.getByRole("region", { name: "Specialist run" })).toBeVisible();
  34 | 	await expect(page.getByRole("complementary", { name: "Run evidence and authority" })).toBeVisible();
  35 | 	await expect(page.getByRole("button", { name: "Create specialist" })).toBeDisabled();
  36 | 	await page.getByRole("button", { name: "Accept" }).click();
  37 | 	await expect(page.getByText("Approved locally").first()).toBeVisible();
  38 | 	await page.getByRole("button", { name: "Pause specialist" }).click();
  39 | 	await expect(page.getByText("Paused").first()).toBeVisible();
  40 | 	await page.getByRole("button", { name: "Resume specialist" }).click();
  41 | 	await page.getByRole("button", { name: "Stop" }).click();
  42 | 	await expect(page.getByText("Stopped").first()).toBeVisible();
  43 | 	await page.getByRole("button", { name: "Retry preview" }).click();
  44 | 	await expect(page.getByText("Waiting for approval").first()).toBeVisible();
  45 | 	await page.getByRole("button", { name: "Return to responsibility" }).click();
  46 | 
  47 | 	await expect(page.getByRole("tab", { name: "Conversation" })).toHaveAttribute("aria-selected", "true");
  48 | 	await decisionSource.click();
  49 | 	await expect(page).toHaveURL(/home\/history\?record=pricing-decision-note/);
  50 | 	await expect(page.getByRole("heading", { name: "Pricing decision note" })).toBeVisible();
  51 | 	await expect(page.getByText("Pricing workshop note · local fixture")).toBeVisible();
  52 | });
  53 | 
  54 | test("compact Chat details trap focus, close with Escape, and restore their trigger", async ({ page }) => {
  55 | 	await page.setViewportSize({ width: 960, height: 700 });
  56 | 	await page.goto("/#/home/chat");
  57 | 
  58 | 	const trigger = page.getByRole("button", { name: "Open context details" });
  59 | 	await trigger.click();
  60 | 	const dialog = page.getByRole("dialog", { name: "Conversation context details" });
  61 | 	await expect(dialog).toBeVisible();
  62 | 	await expect(dialog.getByRole("button", { name: "Back to conversation" })).toBeFocused();
  63 | 	await page.keyboard.press("Escape");
  64 | 	await expect(dialog).toHaveCount(0);
  65 | 	await expect(trigger).toBeFocused();
  66 | });
  67 | 
  68 | test("narrow Home Chat remains an internally scrolling destination", async ({ page }) => {
  69 | 	await page.setViewportSize({ width: 760, height: 620 });
  70 | 	await page.goto("/#/home/chat");
  71 | 
  72 | 	const chat = page.getByRole("region", { name: "Waldo", exact: true });
  73 | 	await expect(chat).toBeVisible();
  74 | 	await expect(page.getByRole("button", { name: "Close Waldo" })).toHaveCount(0);
  75 | 	await expect
  76 | 		.poll(() => page.evaluate(() => document.documentElement.scrollWidth <= document.documentElement.clientWidth))
  77 | 		.toBe(true);
  78 | });
  79 | 
```