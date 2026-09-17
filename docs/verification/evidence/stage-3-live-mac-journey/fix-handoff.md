# Kennel UX Fix Handoff

Source: `6c11583bec922d323ae81037041a17523ca0811f`; blocked live-run report: `2026-09-17-stage-3-live-mac-journey.md`.

Implement one bounded issue: **KUX-001 — make the real packaged owner journey expose Codex adapter installation and pairing.** The installed package reaches a ready local daemon but offers only General, AI providers/API keys, Updates, and Help. There is no discoverable owner route to install an adapter, issue a pairing intent, observe its state, or continue to proof/connection status.

In scope: a daemon-backed, owner-authorized surface that exposes the existing Codex adapter install and pairing operations with non-secret status, clear failure/retry state, and a stable return route. Out of scope: changing pairing cryptography, widening adapter authority, storing bearer material in the renderer, release publication, remote effects, pushes, PRs, merges, deployment, or release.

Required regression evidence: package the exact build; use Computer Use to find the Codex install control; run install and owner-issued pairing; prove valid challenge flow; prove wrong and replayed proof failures; verify rotate/revoke/restart/upgrade facts through daemon-backed UI and logs; retain exact screenshots and redact secrets. Run focused backend tests plus the full backend suite only after the UI route is present.
