# Harness connection and authority

- Status: proposed canonical contract for final architecture review
- Scope: local installation, plugin/skill trust, reconnect, and harness compatibility

Kennel owns the connection. A plugin, skill, hook, or cloud callback is an untrusted adapter until the daemon accepts a versioned, mutually authenticated session.

## Install and reconnect path

1. Kennel discovers supported harness installations and signed-in cloud endpoints without copying harness credentials.
2. It records harness kind, executable or endpoint identity, resolved version, protocol/capability fingerprint, and source.
3. It installs or upgrades only the Kennel-owned adapter package that matches the negotiated protocol. The manifest is signed or content-addressed, carries a minimum/maximum compatibility range, and declares every requested capability.
4. The desktop creates a one-time pairing challenge through the S1 owner path. The adapter proves transport possession over a user-local protected channel; Kennel returns a short-lived, least-privilege transport capability bound to the installation, adapter digest, harness identity, app run, mission, and proposal/transport classes. Pairing authenticates the adapter transport only. It never proves that content came from the owner.
5. Kennel runs a capability check before showing `connected`. The single visible state is `connected`, `degraded`, or `action needed`, with the detected version, missing capability, and one repair action.
6. Desktop reconnect attaches to the already-running daemon and resumes the existing mission from durable identity. It does not restart a healthy daemon or create a new mission. Token rotation and re-pairing do not rewrite mission lineage.

Long-lived bearer secrets never enter prompts, renderer JavaScript, argv, repository files, logs, or plugin-managed storage. An adapter cannot mint an owner principal, widen its capability, or authorize itself.

## Propose versus authorize

The harness may inspect its admitted worktree, edit admitted files, run admitted development commands, publish evidence, ask questions, and propose steering, rework, Plan changes, permission changes, or external effects. Those proposals are data, not authority.

Plugin-originated owner content must carry a separate one-time owner proof minted by an authenticated owner surface for the exact content hash, mission, target/question generation, command class, and expiry. The proof may be returned through the adapter but the owner bearer never is. If the plugin cannot obtain that proof, it submits only a proposal and Kennel round-trips the exact content through the authenticated desktop/CLI owner surface. The daemon labels adapter authentication and owner authentication separately and treats adapter-only content as non-authoritative.

Kennel may automatically deliver ordinary chat turns and answers to already-open questions when this separate owner proof matches the current mission, content, question generation, and existing authority. Minting that narrow proof is a low-friction authenticated action, not a native material-authority confirmation dialog. These routine continuations do not open a native confirmation dialog.

A native confirmation is required for a material authority transition: approving or revising a Contract or Plan, widening filesystem or network access, adding an external effect, changing the approved coding profile, replacing or abandoning an Attempt when not already covered by an explicit policy, or Accepting a Result. The confirmation names the exact revision, capability delta, target, and effect. The daemon, not the adapter, validates and records it.

## Drift and update policy

Every launch and reconnect negotiates protocol and capabilities at runtime. Known-compatible versions proceed. A newer or older version with the same required fingerprint may proceed with recorded provenance. Missing or changed required behavior enters `degraded` or `action needed`; Kennel never silently guesses or downloads an unreviewed adapter.

CI tests the oldest supported, current pinned, and latest available harness versions against captured and live protocol fixtures. Cloud endpoints record server capability fingerprints because their behavior may change without a local binary update. A reviewed compatibility manifest can add a version range or adapter release without changing mission semantics. Active missions pin their negotiated profile; an update affects a new Attempt or an explicit reconnect migration, never a turn in place.

## Acceptance falsifiers

- An unpaired adapter, or a paired adapter without separate exact owner proof, can submit an owner-authorized command.
- Routine question answering causes native confirmation spam.
- Restarting the desktop forks or loses an otherwise healthy mission.
- The UI says `connected` before a real authenticated capability check.
- A harness update silently changes the admitted profile or mission semantics.

## Stage 4C falsifier-to-evidence map

| Authority claim | Deterministic evidence |
|---|---|
| Drift classification has one exact typed repair | `adapterinstall.TestRepairForExactDriftClassMapping` |
| Spoofed or stale connection bindings cannot reach claim creation | `harnesscommand.TestServerWireRejectsSpoofStaleExpiredRevokedAndRotatedOldCredentials` |
| Expired, revoked, or rotated-old credentials fail identically | The same wire test asserts exact `genericFailure` bytes and zero claim-store calls for every case. |
| A stale run file cannot accept a different daemon | `daemon.TestRunFileOwnerServing` already falsifies both wrong service and wrong PID. |
