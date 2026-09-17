# First-run setup contract

The renderer in this slice uses only current source-of-truth surfaces:

- Electron supervisor `DaemonStatus` for daemon startup/readiness.
- `GET /agents` for installed and authorized harness inventory.
- `PATCH /settings/reasoning` for selecting Codex reasoning.
- Native `installTmux` IPC for an owner-confirmed Homebrew install attempt.
- Existing Project folder chooser, repository preflight, Project creation, Outcome intake, Contract review, planning conversation, and Mission Control routes.
- Existing project-scoped Codex discovery/pairing in Project Settings. The tour names the native confirmation boundary and never claims that global reasoning selection performs project pairing.

## Backend gaps that must stay visible

### Setup readiness envelope

No endpoint currently reports a complete, current first-run prerequisite state. Add a daemon-owned read:

```json
{
  "state": "ready | action_needed | checking | error",
  "checkedAt": "RFC3339",
  "prerequisites": [
    {
      "id": "runtime | git | data_directory | admission_policy",
      "state": "ready | missing | incompatible | unknown | error",
      "requiredFor": ["planning", "execution"],
      "version": "optional",
      "message": "safe daemon-authored copy",
      "repair": "install_tmux | open_help | restart_daemon | none"
    }
  ]
}
```

Until that exists, the tour labels tmux state unknown and says it is checked at first session launch. It must not infer readiness from platform, PATH, or a successful installer exit.

### Durable onboarding progress

Current completion is renderer-local. To continue the tour across devices/reinstalls and tie it to real milestones, add a daemon-owned record with states `not_started`, `in_progress`, `skipped`, `completed`; completed milestone IDs; last Project/Outcome; and schema version. Milestones advance only from canonical facts: Project exists, Codex project pairing is connected, Outcome exists, Contract is confirmed, Plan is approved, and Mission Control was entered. UI visits alone do not count.

### Project creation return identity

The global Project flow navigates after creation but does not publish a typed completion event to onboarding. A durable milestone projection should return or expose the exact Project ID and its current pairing/readiness so contextual guidance can continue at the next real step without DOM observation or route guessing.

## UX behavior

- First run opens only after daemon readiness, never over the startup/error screen.
- Every asynchronous operation has a pending label and disables duplicate submission.
- Missing Codex, failed inventory, cancelled tmux install, and install failure have distinct recovery copy.
- Skip and close retain the existing one-time semantics; Settings can reopen the tour.
- The final action enters the real Project flow. No Project, Outcome, Contract, Plan, pairing, or execution state is faked for the tour.
