# F0 decision artifact: @xyflow/react spike vs. React 19

Branch: `candidate/f0-react-flow-spike-f1-portable-primitives`
Base: `32168d0331c12b141d9fa8355100eb6b865b0b9e` (`origin/outcome-loop` tip at spike time — zero commits behind, no rebase concern).

## Recommendation

**PASS on every measured criterion, with promotion blocked pending two owner-side checks that this environment cannot perform:**

1. A real macOS VoiceOver pass (unverified here — see Accessibility below).
2. A manual click/pan check in the packaged app, given one jsdom-only test-environment gap described under Caveats (not observed in the real-Chromium harnesses used for perf/visual evidence).

Nothing in this branch switches the production graph. `MissionWorkUnitGraph` / `DecompositionGraph` are untouched, and the new `mission-canvas-config.ts` boundary defaults to the existing ordered-list renderer; only the spike's own fixture and tests ever construct `{ renderer: "flow" }`.

## Canonical-doc note

The packet cites "DESIGN.md Mission Control canvas" and "docs/product/experience.md Screen 6" — neither heading exists verbatim. The substance is present under different names: `docs/product/experience.md`'s **Watch** moment ("graph above the WorkUnit/current-Attempt board") and its **UX invariants**, and `DESIGN.md`'s **Product flow (what the UI must serve)** section. Those are what this spike was built against.

Separately, `DESIGN.md` declares itself superseded by a Figma file this environment cannot open. Visual tokens for the spike and the F1 gallery were therefore taken from the existing renderer code that already implements that Figma direction (`MissionWorkUnitGraph.tsx`, `DecompositionGraph.tsx` — `hairline border-border bg-card`, `text-2xs text-passive`, `border-l-status-*`), not re-derived from DESIGN.md's own (superseded) hex tables.

## Evidence table

| Criterion | Result | Detail |
| --- | --- | --- |
| React 19 build compatibility | PASS | `npm install @xyflow/react@12.11.6` (peer range `>=17`, no ERESOLVE); clean `vite build --config vite.renderer.config.ts` before and after |
| No production default switch | PASS | `mission-canvas-config.ts` defaults to `"list"`; nothing outside the spike fixture/tests requests `"flow"` |
| Bundle delta (production renderer) | +630 B raw / +9 B gzip (~0.01%) | See "Bundle" below — no new chunk reaches production |
| 75-node mount, no visible stalls | PASS | flow median 2.55–2.85 ms / p95 5.0–7.2 ms; list median 0.70–0.80 ms / p95 1.0–1.6 ms (Chromium 148, real browser) |
| Keyboard reachability, dependency order | PASS | list: roving tabindex in layer order; flow: every node face is a real `<button tabindex=0>` in fixture-array (= layer) order |
| State/blocker conveyed without color alone | PASS | every node's `aria-label` and visible text carry state + blocker; left-border color is supplementary |
| Selection ≠ execution | PASS | `onSelectNode` is the only callback in `MissionCanvasSpikeProps`; no mutation/authority prop exists |
| State-only update doesn't refit | PASS | test spies the live `ReactFlowInstance.fitView`: 0 calls across a state-only rerender, exactly 1 call on a revision bump |
| Reduced motion | PASS | `fitView`/`fitViewOptions` duration forced to `0` under `prefers-reduced-motion: reduce` |
| Cyclic / dangling dependency input | PASS | reuses `layerByDependency`'s existing cycle guard (depth-bounded, unknown ids → depth 0); pathological 3-node fixture renders without hanging |
| List fallback intact | PASS | default renderer; 51/51 production bundle files unchanged in identity, `@xyflow/react` unused at runtime unless `renderer:"flow"` is explicitly requested |
| Accessibility (automated) | PASS | role/name/keyboard evidence above |
| Accessibility (VoiceOver) | **UNVERIFIED** | no interactive macOS VoiceOver session in this environment — not simulated |
| Screenshots (desktop/narrow, light/dark) | PASS | captured via a throwaway harness (not committed); see "Screenshots" below |

## Bundle

Baseline (`vite build --config vite.renderer.config.ts`, clean tree, before any change): 51 JS/CSS files, **5,133,453 B raw / 1,408,757 B gzip**.

After F0 + F1 (identical build command): **5,134,083 B raw / 1,408,766 B gzip** — **+630 B / +9 B gzip (0.012% / 0.0006%)**, same 51 files by name, reproducible exactly across repeated rebuilds of the identical tree. Isolation check: reverting only `packages/product-ui/src/index.ts` to its pre-F1 content and rebuilding gave 5,134,088 B raw / 1,408,808 B gzip — *larger*, not smaller, than the full change. That disproves an earlier draft of this report, which guessed the delta came from the new product-ui exports; it does not. The true cause was not isolated further. What's confirmed instead: no new chunk is emitted, the same 51 files exist under the same names, and `@xyflow/react` does not reach the production bundle (nothing outside the spike's own fixture/tests imports it). The residual ~0.01% is noise-scale and not chased further.

Isolated marginal cost, if a future slice does wire `MissionCanvasSpike` into a production route (measured against a bare React 19 + ReactDOM baseline, same build command, no other app code):

| | Raw | Gzip |
| --- | --- | --- |
| `@xyflow/react` JS | +178.5 KB | +57.1 KB |
| `@xyflow/react` CSS | +15.4 KB | +2.6 KB |
| **Total** | **≈+194 KB** | **≈+59.7 KB** |

## Performance

Method: a throwaway Vite + Playwright (Chromium) harness — not committed — mounting `MissionCanvasSpike` directly from this branch's source via a path alias, timing with `react-dom`'s `flushSync` to bound each React commit (excludes GPU paint, includes real React + `@xyflow/react` reconciliation cost). ≥10 measured runs after 3 warm-up runs each; two independent full runs shown.

Machine: macOS 27.0 (26A428), Apple M4, Node v22.23.2, Chromium 148.0.7778.96 (Playwright-managed).

| Metric | Run 1 (median / p95) | Run 2 (median / p95) |
| --- | --- | --- |
| Mount 75 nodes — list | 0.80 / 1.10 ms | 0.75 / 1.60 ms |
| Mount 75 nodes — flow | 2.60 / 5.00 ms | 2.55 / 7.20 ms |
| State-only update — list | 0.50 / 1.40 ms | 0.45 / 0.60 ms |
| State-only update — flow | 1.50 / 3.40 ms | 1.60 / 3.70 ms |
| fit-view (`duration:0`) — flow | 0.70 / 1.00 ms | 0.75 / 1.10 ms |

No threshold for "visible stall" is defined in canonical docs; reporting the measured numbers rather than inventing one. All are well under a single frame (16.7 ms) at p95.

## Screenshots

Captured with the same throwaway harness at 1280×900 (desktop) and 400×900 (narrow), dark and light (`data-theme`) — desktop dark/light and narrow dark for the canvas; desktop dark/light and narrow dark for the F1 gallery. Not committed to the repo (no image files are on the F0 file allowlist); described here from direct inspection:

- **Flow, dark, desktop**: 75 cards laid out in 10 dependency layers, routed `smoothstep` edges visible between layers (fixed once — see Caveats), left-border color per state plus a visible state-name label on every card, pan/zoom controls bottom-left, background dot grid.
- **Flow, light**: same layout, tokens flip correctly (`:root[data-theme="light"]` path), no contrast/clipping issues observed.
- **Flow, narrow (400px)**: canvas viewport crops to the fit-view window as expected for a pannable canvas; no horizontal page overflow.
- **List, dark, desktop/narrow**: ordered `STEP N` groups, single-column reflow at 400px, long labels truncate cleanly, zero layout clipping.
- **F1 gallery, dark/light/narrow**: all eight primitives render with correct tone colors + text labels, a visibly disabled vs. an enabled `ApprovalCard` primary button, a "Sending…" pending `BoundedComposer` distinct from a disabled one, a red-bordered destructive `SelectionActionBar` action, and a collapsed-by-default `InspectorShell` activity disclosure with its footer intact — all confirmed by both direct pixel inspection and the automated tests below.

## Caveats

- **Edge rendering required a fix.** The custom React Flow node type initially had no `<Handle>` elements, so `@xyflow/react` silently drew zero edges (100 expected). Fixed by adding invisible target/source handles to `MissionCanvasNodeFace`; confirmed 100/100 edges render and are visible in both themes. Worth flagging because this failure mode is silent — no console error, just an empty edge layer.
- **jsdom-only pointer-event gap.** A full `userEvent.click` (which dispatches a real mousedown→mouseup→click sequence) on a flow node throws an uncaught `TypeError: Cannot read properties of null (reading 'document')` from `d3-zoom`'s pane-level `mousedowned` handler, because jsdom's synthesized `mousedown` carries `event.view === null`. A plain `fireEvent.click` (single click event, no mousedown) does not trigger it, and it was **not observed** in the real-Chromium perf/visual harnesses. Recorded as a test-environment caveat, not a production defect; recommend confirming a real click in the packaged app before promotion.

## Test matrix run

- Focused: `MissionCanvasSpike.test.tsx` (14 tests) + `mission-canvas-config.test.ts` (3 tests) — 17/17 pass.
- `packages/product-ui`: `npm run typecheck && npm test && npm run build` — 18 test files / 105 tests pass; `import-boundary.test.ts` passes unmodified (all 8 new primitives import only `react` and `./utils`, no anchors, no escapes).
- Full frontend: `npx tsc --noEmit` — same 4 pre-existing type errors as the unmodified base commit (in `main.ts`, `owner-command-handler.test.ts`, `bridge.ts`; unrelated to this work, confirmed byte-identical against a baseline typecheck run before any change), zero new errors.
- Full frontend: `npx vitest run --config vite.renderer.config.ts` — 238/238 test files, 2862 passed / 6 skipped (pre-existing skips).
- Full frontend: `vite build --config vite.renderer.config.ts` — clean, see Bundle above.

## Allowlist note

One edit falls outside the packet's literal F0 file allowlist: `frontend/src/renderer/i18n/renderer-coverage.test.ts`. This repo-wide test fails on any hardcoded English JSX text under `src/renderer/components`, with no exemption for dev/preview-only code — and the F1 instructions require the fixture gallery to use literal strings, never i18n keys (the shared primitives are translation-agnostic; labels come from props). The repo already has a `deferredLocalizationFiles` registry for exactly this situation (used for the still-unlocalized chat surface); one line was added there for `components/preview/MissionPrimitivesGallery.tsx`, with a comment explaining why. No locale JSON files were touched, and no other file outside the allowlist was edited.
