# Stage 3 pairing-intent owner wiring

- Baseline: approved S3.3 `66312355dbd85aebf5da47b58990fe8f6552f733`
- Scope: authenticated daemon owner-command route to open an exact S3.3 pairing intent

## Path

1. An owner surface proposes pairing and supplies the exact discovered S3.2 tuple. Renderer content never receives the owner-command bearer.
2. Electron main (or another existing authenticated owner client) sends that closed tuple (without an app-run ID) to the daemon's loopback-only `/internal/owner-commands/harness-pairing-intents` route with the inherited `KennelOwner` token.
3. The route authenticates through the existing `ownercommand.Authority`, takes the app-run ID from its typed authentication result, rejects request-controlled app-run IDs and other unknown fields/classes, fixes challenge and connection expiry on the daemon side, and calls `Coordinator.Issue`.
4. The response returns the intent ID and caller-only secret to that authenticated owner client. The adapter-facing protected local transport can retrieve only the already-open intent ID, then prove the tuple plus secret. It cannot call `Coordinator.Issue`, choose capabilities, or obtain the secret through its surface.

This does not make pairing owner command authority. It only makes opening the adapter-transport possession window an authenticated owner action. The resulting bearer still carries transport classes only, and all owner content/effects require the separate S1 owner-proof routes.

## Failure boundaries

- Missing owner authority, coordinator, or store means the route is absent or fails closed.
- LAN requests are hidden by `localControlRequest`; missing/wrong bearer is unauthorized.
- Capability classes use the frozen closed set. Expiry is daemon-owned.
- Exact request body is bounded and unknown fields are rejected. The pairing tuple app-run ID comes only from the typed owner authentication result, never the request body.
- No adapter package imports the HTTP owner route or receives an Issue-capable interface.
