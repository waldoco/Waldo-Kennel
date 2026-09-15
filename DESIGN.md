# Design System — Kennel

> **Current product topology:** [Persistent mission runtime](docs/architecture/persistent-mission-runtime.md). Earlier “orchestrator system,” task/session, and spawn-worker labels in this visual-history document describe retained UI references, not current ontology or implementation order. New UI uses Outcome -> Contract -> Plan/DAG -> WorkUnit/current Attempt, with mission-level Supervisor intelligence and session drill-down.


> Source of truth for the Kennel desktop UI (Electron + React 19 + Tailwind v4
>
> - Radix/shadcn + xterm, in `frontend/src/renderer`). Read this before any visual
>   or UI change. Created by `/design-consultation` on 2026-06-09.

## ⚠️ Design direction — Kennel orchestrator system, Figma (SUPERSEDES all prior direction · 2026-08-22)

By explicit user decision (2026-08-22), the desktop app follows the **Kennel
orchestrator design system** authored in Figma:

- **Source of truth:** Figma file `Dl0WP9uIvx6QbSzZi7cZQY` ("Waldo"), section
  `2984-17556`. Board screen `2948:15618`, List screen `2960:16130`, choice panel
  `2960:16912`. The Figma is gospel down to spacing and stroke width; where any
  other part of this document conflicts with it, **Figma wins**.
- **Notch tuning:** Kennel Island (`packages/kennel-island`) uses this same system,
  tuned for the notch with reduced text hierarchy, simpler composition, and a
  `#000000` background at all times so it reads as continuous with the physical
  camera housing. Its components otherwise follow the same design language.

### The system in one page

**Surfaces** — a warm near-black ramp where elevation reads by temperature, not shadow:

| Role | Token | Dark | Where |
| --- | --- | --- | --- |
| Canvas | `--background` | `#151515` | window / content |
| Sidebar | `--sidebar` | `#161616` | left rail |
| Shell plate | `--color-bg-shell` | `#1a1a1a` | board lane, list group, segmented track |
| Card | `--card` | `#272725` | session card, list row plate, active segment |
| Raised | `--popover` | `#353533` | chips, secondary buttons, popovers, choice panel |
| Row hover | `--muted` | `#1e1e1e` | sidebar rows |

**Text** — `#fafaf8` primary, `#9a9a96` secondary (`--muted-foreground`), `#6b6b68`
passive (`--color-text-passive`), `#979797` list meta (`--color-text-meta`).

**Lines** — `rgb(255 255 255 / 8%)` (`--border`) and `rgb(255 255 255 / 10%)`
(`--input`), drawn at **0.6px** via the `hairline` utility on cards, chips and
buttons. Hairlines separate; they never frame.

**Lane hues** — Needs Choice `#fb8404`, Needs Input `#fbbc04`, Ready `#00cc6e`,
Running `#2388ff`. Links are `#2388ff` (`--color-link`), branch names `#8338ec`
(`--color-branch`). These are the only saturated colours in the app.

**Type** — SF Pro Rounded (`--font-family-base`, falls back to Geist off macOS) at a
uniform **+2% tracking** (`--tracking-base`, applied on `body`). Sizes: 16 title
(`text-brand`), 14 chrome (`text-sm`), 12 body/chips (`text-xs`), 10 meta
(`text-2xs`). Leading 1.3 (`leading-snug`) everywhere, 1.4 (`leading-body`) for
wrapped copy. **No monospace in product chrome** — the terminal keeps its own face.

**Radii** — 5 (`rounded-xs`) · 6 (`rounded-sm`) · 8 (`rounded-md`, the workhorse) ·
12 (`rounded-lg`) · 15 (`rounded-card`) · 16 (`rounded-panel`) · 18 (`rounded-group`).

**Spacing** — 3 / 5 / 8 / 10 / 15 / 18 / 20px on the existing `--space-*` scale.

### Composition rules

- A **board lane** is a `rounded-group` plate that fades out downward
  (`column-shell`): solid under the header, gone by the foot, so a lane holding
  one card does not box empty space. Lanes sit flush; the row reads as one surface.
- A **lane heading** is dot · name · count chip, with a reserved 26px slot on the
  right for the lane menu.
- A **session card** is `bg-card` + `hairline` at `rounded-card`, 18px padding,
  20px between blocks: provenance (agent · branch chip · age) → state line → title →
  summary → links → actions. Provenance leads because the card answers "whose work
  is this" before "what is it".
- The **state line** is coloured text, not a pill, and only carries a dot when that
  dot is moving (live agent activity, in-flight agent switch).
- The **list view** is the same lanes with each session on one line; long cells clip
  behind a fade (`list-cell-fade`) rather than ellipsing, so columns stay aligned.
- **List / Board** is a two-item segmented control on the shell plate, heading the
  lanes it governs. The choice persists (`kennel.sessions.viewMode`).

### First-run setup tour

`OnboardingTour` is the app's one modal that a person does not summon. Its shape
is borrowed from Xirp's getting-started flow (Spotify's docs, `xirp/getting-started`)
and rebuilt in this system:

- Centred dialog on a `rounded-panel` card plate, header and footer separated by
  hairlines from a **fixed-height** body — a tour that grows with its content
  moves its own footer buttons while a person is reaching for them, so long steps
  scroll instead.
- Header: step name · `· N of 4` · dot pager · close. Footer: `← Back` ·
  `Skip tour` · one primary action.
- Progress dots are **neutral greys**, never the lane hues. Orange and green mean
  "needs you" and "ready" everywhere else; spending them on a step counter would
  make them mean nothing.
- Four steps, each one decision that writes a real setting: default coding agent
  (from the daemon's probe), session alerts (fires a real notification so a person
  can confirm it reaches them), and layout (Board or List). Every one is reachable
  again from Settings → Replay welcome tour.
- It waits for the daemon to be ready before opening, and closing by any route —
  finish, skip, or the X — counts as answered.

### Known gaps (design elements without data or product wiring)

- Lane menu button: the 26px slot is reserved and empty — no lane-level action exists.
- Card summary (`BoardSessionPresentation.summary`) renders when supplied; the daemon
  does not yet supply it.
- Card action row (`SessionCardView` `actions` prop) is styled and empty; Instruct /
  Merge / pause are not wired.
- Figma's lane names "Needs Choice" and "Needs Input" describe a different lane model
  than the app's attention zones; `zone.action` stays "Needs you" and `zone.pending`
  stays "In review" until that model changes. "Ready" and "Running" were adopted.

## Product Context

- **What this is:** Kennel is an Electron desktop app for supervising many parallel
  AI coding-agent sessions, backed by a Go daemon (`backend/`). The `ao` CLI is the
  thin client over the same daemon.
- **Who it's for:** professional software engineers running multiple coding agents at
  once who need to delegate, watch, intervene, and ship PRs.
- **Space/peers:** agent orchestration / parallel-agent desktop tools.
- **Project type:** dark-mode-primary desktop app; terminal-dense; keyboard-driven;
  runs all day.
- **The one memorable thing:** leverage and speed — "I'm more in control here than
  babysitting N terminal tabs myself."

### Product flow (what the UI must serve)

Kennel is **Outcome-led**, not a flat list of sessions and not a legacy orchestrator
chat. The UI follows the canonical runtime:

- one owner timeline contains Contract clarification, fresh mission planning, execution,
  checks, Result, and Accept;
- the approved Plan graph sits above WorkUnit/current-Attempt cards;
- a mission-level Supervisor summary explains progress, drift, attention, and bounded
  automatic steering;
- each WorkUnit detail shows its current Attempt and persistent primary Codex thread;
- raw provider transcript and native child activity stay in drill-down;
- the daemon-owned MissionProjection supplies status and one true next action;
- legacy Orchestrator/Worker and `ao` session screens remain compatibility/history views
  until their removal gate, not the vNext information architecture.

## Aesthetic Direction

> **Superseded (2026-08-22):** the Figma banner at the top governs. This section is
> retained because the live look is still the same flat near-black / hairline
> family, so most of it reads true.

- **Direction:** flat, near-black, hairline-bordered, utilitarian. Industrial control
  surface, calm chrome, the terminal as the center of gravity.
- **Decoration level:** minimal. Type + 1px hairlines do all the work. No gradients,
  glow, blobs, or emoji.
- **Mood:** low-glare, dense, keyboard-native; signal-over-noise.
- **Reference:** a flat, hairline-bordered desktop control surface (primary, visual +
  structural). Tokens below were derived from that reference's renderer CSS.
- **Deliberate tradeoff:** to match that reference, we use the **system font stack** (not
  a custom typeface) and its neutral palette. We diverge in exactly one place: the
  accent is Kennel's **refined blue**, not the reference's jade green. The terminal
  keeps green (it is the agent CLI).

## Typography

System fonts only — no custom/Google fonts, zero font payload.

- **UI / body / display:** `-apple-system, BlinkMacSystemFont, "Segoe UI", Roboto,
Oxygen, Ubuntu, Cantarell, "Fira Sans", "Helvetica Neue", sans-serif` (San Francisco
  on macOS).
- **Mono / terminal / code / eyebrow labels:** `Menlo, Monaco, Consolas,
"Liberation Mono", "Courier New", monospace`.
- **Eyebrow labels** (section titles, dialog titles, the rail "PROJECTS" header):
  mono, **uppercase**, `letter-spacing: .12–.14em`, `--foreground-passive`.
- **Scale:** 14px base UI / sidebar (`text-sm`, weight 400) · 12px secondary + labels
  (`text-xs`) · 13px code/mono/terminal · 11px tiny · 10px micro + badges · 9px sidebar
  badge label. Buttons are `font-normal` (400), not bold.

## Color

A flat Radix-neutral near-black ramp carries the whole interface; color is rare
and meaningful. Values are sRGB approximations of the reference's `color(display-p3 …)` tokens.

### Dark (primary)

| Role                                 | Hex             |
| ------------------------------------ | --------------- |
| `--bg` canvas                        | `#111111`       |
| `--bg-1` surface                     | `#191919`       |
| `--bg-2` raised / hover / active row | `#222222`       |
| `--bg-3`                             | `#2a2a2a`       |
| `--fg` text                          | `#eeeeee`       |
| `--fg-muted`                         | `#b4b4b4`       |
| `--fg-passive`                       | `#6e6e6e`       |
| `--border` hairline                  | `#3a3a3a`       |
| `--border-1`                         | `#484848`       |
| **`--accent` (blue)**                | **`#5b9dff`**   |
| `--needs-you` / in-progress (amber)  | `#ffcc4a`       |
| `--success` / mergeable (green)      | `#6cb16c`       |
| terminal green                       | `#7bd88f`       |
| `--error` (red)                      | `#d4544f`       |
| text selection                       | `#3f8ef7` @ 35% |
| terminal bg                          | `#161616`       |

### Light (supported, not primary)

| Role                      | Hex                               |
| ------------------------- | --------------------------------- |
| canvas / surface / raised | `#fcfcfc` / `#ffffff` / `#ededee` |
| text / muted / passive    | `#1a1a1a` / `#666666` / `#9a9a9a` |
| border                    | `#e3e3e5`                         |
| accent (blue)             | `#2563eb`                         |
| amber / green / red       | `#9a6b00` / `#1a7f37` / `#c0392b` |

### Accent rules

- **Blue** = the live edge only: primary buttons, the active/selected session, focus
  rings. Never decorative.
- **Amber** = an agent needs you (blocked / `needs_input` / `review_pending`).
- **Green** = `mergeable`/success and terminal/agent CLI text.
- **Red** = `ci_failed` / destructive.
- These map 1:1 to the daemon's derived statuses.

### Status indicator (no text badges)

Session status is a single ~14px glyph in one fixed slot, never a text pill/badge:

- **Working / active** → an animated spinner (accent).
- **Has an open PR** → a PR icon, tinted by PR state: mergeable/approved green,
  `ci_failed` red, review/`changes_requested` amber, plain `pr_open` muted.
- **Otherwise** → a filled dot: `needs_input` amber (pulsing), idle/done muted gray.

Precedence: **working spinner > PR icon > dot**. Implemented as `StatusGlyph` in
`components/SideRail.tsx`; used in the orchestrator's Workers list. (Worker rows in the
left rail stay name-only — no glyph.)

## Spacing

- **Base unit:** 4px (Tailwind scale: 1=4, 1.5=6, 2=8, 3=12, 4=16, 5=20, 6=24).
- **Density:** compact / desktop-tight.
- **Control + row height:** `h-8` = 32px default; `h-7` = 28px small; `h-6` = 24px xs.
- Inputs `px-2.5 py-1`; buttons `px-2.5`, gap 1–1.5.

## Layout

- **Approach:** fixed three-pane app shell, opens into the workbench (no marketing/dashboard home).
- **Panes:** `[ rail 240px ] [ center 1fr ] [ side rail 316px ]`.
- **Rail (240px), top → bottom:**
  1. **Orchestrator anchor** — pinned, single, visually distinct (blue 2px left bar,
     `--bg-2` fill, hub/`waypoints` icon, name "Orchestrator", a `5 agents · 2 need you`
     mono summary). This is Kennel's one addition over the reference. Default landing view.
  2. `PROJECTS` eyebrow label + a `+`.
  3. Project rows (folder icon + name) with nested **worker rows beneath**. Each project
     row has a hover-revealed **`+`** that opens the New-worker modal pre-scoped to that
     project (distinct from the `PROJECTS` header `+`, which registers a repo).
  4. **Footer:** `Search ⌘K`, `Settings ⌘,`. (No Library.)
  5. **Account** row pinned at the very bottom.
- **Worker rows are name-only.** Just the session name, truncated. Status, branch, diff,
  and PR live in the panes and topbar, never in the row. Selection = `--bg-2` fill + a
  2px blue left bar. (the reference itself shows a faint trailing timestamp; we omit it by choice.)
- **Center = the conversation.** Orchestrator → its coordination terminal (delegate here;
  composer reads "tell the orchestrator what to build"). Worker → the agent CLI terminal
  (tabbed per agent, e.g. `claude-code (1)`), with a composer (model selector, worktree
  path, `Accept edits`). The terminal **is** the conversation; no separate chat surface.
- **Side rail (316px):** orchestrator → a quiet **Workers** list (name + project + derived
  status). Worker → the **Git review rail**: `Changed N` → All files / Discard all / Stage
  all → file rows (`+adds −dels`, stage toggle) → `Commit message` + `Description` →
  **Commit & Push** (primary blue) → branch + `Create PR`.
- **Border radius:** `sm` 4px (scrollbar) · `md` 6px (buttons, inputs, toggles) ·
  `lg` 8px (rows, cards, panels) · `xl` 12px (modals) · `full` (badges/pills/dots).
- **Icons:** **lucide** only. No emoji.

### Topbar

- **Left (both):** `project / session` breadcrumb + pin; for the orchestrator, a hub icon
  - `Orchestrator`.
- **Right — worker session:** a **PR/CI status pill** that is the action
  (`PR #156 · mergeable` green / `CI failed` red / `review requested` amber /
  `Open PR` when none) → **Changes / Files / Terminal** view toggles → **⋯ session menu**
  (rename, restart, kill, claim PR — the `ao session …` commands).
- **Right — orchestrator:** **+ New worker** → Terminal toggle → **⋯ menu**. No diff toggles.

### WorkUnit release modal (historical spawn-worker visual reference)

You mostly let the orchestrator spawn workers from its conversation; the manual paths
(the topbar `+ New work`, a WorkUnit row action, or legacy `ao spawn`) open a modal that
mirrors the reference exactly. Launching from a project row pre-fills the Project field:

- Centered dialog, **12px radius**, `max-w` ~512px, `bg` canvas, `ring-1` at 10% fg,
  fade + zoom-95 enter.
- **Header:** eyebrow mono-uppercase title `New worker` + `×` close.
- **Body** (`gap` 15–16px): a **borderless large name field** (18px, auto-focus, slug
  rule "letters, numbers, hyphens") → **Project** selector → **Agent** selector
  (claude-code / codex / opencode / …) → a **"Based on"** bordered card with a segmented
  control `Branch · Issue · Pull Request` revealing a combobox → a **Prompt / Workspace**
  tab where Prompt is the worker's initial task (textarea).
- **Footer:** right-aligned single primary **`Spawn worker`** (blue) with a `⌘↵` keycap,
  disabled until valid.

## Motion

- **Approach:** minimal-functional. The one expressive exception: a status dot/spinner
  pulse on active/working sessions (opacity breathe) so "alive" is glanceable. Never
  animate text or layout.
- **Easing:** enter `ease-out`, exit `ease-in`, move `ease-in-out`. The CSS keywords are
  too soft to read as deliberate, so each is pinned to a curve in `renderer/styles.css`
  and referenced by name — never spelled inline:
  `--ease-out: cubic-bezier(0.23, 1, 0.32, 1)` · `--ease-in: cubic-bezier(0.4, 0, 1, 1)` ·
  `--ease-in-out: cubic-bezier(0.77, 0, 0.175, 1)` ·
  `--ease-drawer: cubic-bezier(0.32, 0.72, 0, 1)` for panels that travel an edge.
  `frontend/src/site-theme/tokens.css` mirrors the same three for the marketing site.
- **Duration:** the tokens are `--duration-fast: 120ms` · `--duration-normal: 150ms` ·
  `--duration-slow: 240ms` (`tokens.css`), surfaced as `duration-fast|normal|slow`
  utilities. Nothing in the product should exceed 240ms. Status pulse is the one
  loop, at 1.8s.
- **Exits are shorter than entries** — the system is responding, not deciding:
  overlay 80/60ms · modal 120/70ms · popover 150/100ms · tooltip 125/90ms ·
  sheet 240/150ms.
- **Anchored surfaces scale from their trigger,** not from their own centre: popover,
  dropdown, context menu, select and tooltip all set `transform-origin` from the Radix
  content variable. Modals are the exception and stay centred — they are not anchored
  to anything. Nothing enters from `scale(0)`; modals start at `0.95`, popovers at `0.98`.
- **Pressables take `--scale-press` (0.97) on `:active`.** A control that does not move
  under the press gives the user nothing confirming the click landed. This applies to
  buttons, switches, menu items and the island's controls alike.
- **Keyboard-initiated surfaces do not animate.** The command palette is summoned by ⌘K
  dozens of times a session; an entrance on the panel reads as latency between the
  keypress and a usable caret. Its scrim still fades so the context change is not a hard
  cut. This is a deliberate exception to the modal enter rule above.
- **Only `transform` and `opacity`.** Meters and progress fills are full-size elements
  scaled with `scaleX()` from a left origin, never elements whose `width` is animated —
  width relayouts its row on every frame.

## Implementation notes

- The renderer (`frontend/src/renderer/styles.css`) currently uses **Inter** and a
  grayscale-blue theme. Migrate to this system: drop the Inter `font-family`, adopt the
  system stack, and replace the token values with the neutral ramp + blue accent above.
- Keep tokens as CSS custom properties under `:root` (dark) and `:root[data-theme="light"]`.

## Decisions Log

| Date       | Decision                                                               | Rationale                                                                                          |
| ---------- | ---------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------- |
| 2026-06-09 | Match the reference's visual language exactly                          | User direction; the reference is the demonstrated model for this app's UI.                         |
| 2026-06-09 | System font, not a custom typeface (e.g. Geist)                        | The reference uses the system stack; fidelity + native feel + zero font payload over brand type.   |
| 2026-06-09 | Refined **blue** accent, not the reference's jade green                | User's explicit pick; blue for primary/active/focus, terminal stays green.                         |
| 2026-06-09 | Single global **Orchestrator** anchor, orchestrator-first default view | The one real difference from the reference; orchestrator is the human-facing coordinator.          |
| 2026-06-09 | **Name-only** worker rows                                              | User direction; status/branch/diff live in panes + topbar, not the row.                            |
| 2026-06-09 | Removed **Library** from the rail footer                               | User direction; footer is Search + Settings only.                                                  |
| 2026-06-09 | Topbar right = PR/CI pill + view toggles + ⋯ menu (worker)             | Surfaces the actionable PR/CI state from the daemon; desktop-tool precedent.                       |
| 2026-06-09 | Spawn modal mirrors the reference's Create Task                        | Consistency with the reference; mapped to `ao spawn` params.                                       |
| 2026-08-22 | Motion curves and durations become named tokens, used everywhere       | `duration-fast` compiled to nothing (Tailwind has no `--duration-*` namespace) and Sheet's animation classes did not exist at all; naming the values makes the gaps visible instead of silent. |
| 2026-08-22 | Command palette opens with no entrance animation                       | Keyboard-summoned surfaces are opened dozens of times a session; any entrance reads as latency. Raycast precedent. Deviates from the modal enter rule by design. |
| 2026-08-22 | Landing site gates every `hover:` behind `(hover: hover)`              | Touch devices synthesise hover on tap and leave it stuck; the marketing site is the only surface with touch users. |
