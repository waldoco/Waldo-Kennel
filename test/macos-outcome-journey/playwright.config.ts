import { defineConfig } from "@playwright/test";

// Packaged macOS Outcome journey harness. No webServer, no browser — this
// launches the REAL packaged Electron app (via _electron.launch) against an
// isolated profile and a disposable git fixture, driven entirely through
// frontend/node_modules/@playwright/test (the only package.json in this repo
// that declares it; see scripts/outcome-journey/run-outcome-journey.mjs).
//
// Deliberately NOT under frontend/e2e/: frontend/playwright.config.ts's
// "work" project globs testDir: "e2e" with only an @legacy-board exclusion,
// so a spec placed anywhere under frontend/e2e/ would be picked up by
// `npm run test:e2e` and launched against the dev:web Vite server with no
// Electron at all — failing there for a reason unrelated to this journey.
// Root-level test/ already holds test/e2e-pod (the Linux/xvfb/.deb real-app
// harness); this is its macOS sibling, not a variant of the frontend suite.
export default defineConfig({
	testDir: ".",
	testMatch: /outcome-journey\.spec\.ts/,
	// One journey, one worker: the spec drives one packaged app instance
	// through one linear sequence of owner actions. Parallelizing would only
	// mean two Electron apps fighting over the same fixture/profile discipline
	// this harness exists to prove is isolated.
	workers: 1,
	timeout: 15 * 60_000,
	reporter: [["line"], ["json", { outputFile: "test-results/results.json" }]],
});
