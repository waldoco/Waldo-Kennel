import { defineConfig } from "@playwright/test";

const port = Number(process.env.KENNEL_E2E_PORT ?? "5173");
const legacyPort = Number(process.env.KENNEL_E2E_LEGACY_PORT ?? "5174");

// Two renderer launch modes, one suite:
//
// - "work" runs the default Outcome-first launch (VITE_KENNEL_WORK_LAUNCH
//   unset). `/` redirects to `/work`; there is deliberately no second board.
// - "legacy-board" runs VITE_KENNEL_WORK_LAUNCH=0 so the session-board project
//   route stays reachable. The @legacy-board specs keep guarding the shipped
//   SessionsBoard until its coverage moves to the Outcome run lineage board
//   with daemon Attempt fixtures (#78).
//
// Each project gets its own dev:web server (VITE_NO_ELECTRON=1) — no Electron
// child to launch, which is all the browser-based e2e suite needs.
export default defineConfig({
	testDir: "e2e",
	projects: [
		{
			name: "work",
			grepInvert: /@legacy-board/,
			use: { baseURL: `http://127.0.0.1:${port}` },
		},
		{
			name: "legacy-board",
			grep: /@legacy-board/,
			use: { baseURL: `http://127.0.0.1:${legacyPort}` },
		},
	],
	webServer: [
		{
			command: `npm run dev:web -- --port ${port} --host 127.0.0.1`,
			port,
			reuseExistingServer: !process.env.CI,
		},
		{
			command: `npm run dev:web:legacy-board -- --port ${legacyPort} --host 127.0.0.1`,
			port: legacyPort,
			reuseExistingServer: !process.env.CI,
		},
	],
});
