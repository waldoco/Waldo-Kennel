# Instructions

- Following Playwright test failed.
- Explain why, be concise, respect Playwright best practices.
- Provide a snippet of code with the fix, if possible.

# Test info

- Name: home-personal-agent-flows.spec.ts >> evening Today reaches a truthful Daily Close receipt and Insights
- Location: e2e/home-personal-agent-flows.spec.ts:32:5

# Error details

```
Error: Channel closed
```

```
Error: locator.click: Target page, context or browser has been closed
Call log:
  - waiting for getByRole('link', { name: 'Start Closure' })
    - locator resolved to <a href="#/home/daily-close" class="flex w-full items-center justify-center rounded-md border border-border bg-raised/45 px-4 py-3 text-sm font-medium text-foreground transition-colors hover:border-border-strong hover:bg-raised focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/70 motion-reduce:transition-none">Start Closure</a>
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
    34 × waiting for element to be visible, enabled and stable
       - element is visible, enabled and stable
       - scrolling into view if needed
       - done scrolling
       - <div role="status" aria-labelledby="home-coming-soon-heading" class="absolute inset-0 z-10 flex items-center justify-center bg-background/85 backdrop-blur-sm">…</div> intercepts pointer events
     - retrying click action
       - waiting 500ms

```

```
Error: browserContext.close: Target page, context or browser has been closed
```