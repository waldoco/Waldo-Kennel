# Stage 7C - restart / kill / delivery-unknown matrix

Every cell runs against the real sqlite store. Outcome-service cells simulate a
daemon kill by closing the store mid-flight and building a fresh service over
the same data directory (`sqliteLifetime` in
`backend/internal/service/outcome/restart_matrix_test.go`); chat governed-turn
cells use two controllers over one directory (`sqlitetest.MustOpenAt`). No fake
durability anywhere in the matrix.

## Cells

1. **Daemon restart mid-attempt recovers exactly one continuation.**
   `TestRestartMatrix_DaemonRestartRecoversOneContinuation` - attempt left
   running, no receipt; fresh process `ContinueAuthorizedRuns` twice: exactly
   one active continuation, at most one replacement spawn, and no invented
   success (an attempt reaching succeeded/reconciled with no recorded result
   fails the test).

2. **Provider kill mid-turn: delivery_unknown stays nonterminal and blocking;
   no duplicate effect on reconnect.** Covered by the sqlite-backed chat
   suite: `TestGovernedTurnCrashMidDispatchRestartsUnknownWithoutRedelivery`
   and `TestSendDeliveryUnknownStaysDurablyQueued`
   (controller_test.go, real store via `openStore`), with the negative fence
   `TestGovernedTurnExplicitRejectionIsNotDeliveryUnknown`.

3. **Reconnect retry with the same request key returns the recorded receipt;
   no duplicate spawn, no new durable intent.**
   `TestRestartMatrix_SameRequestKeyReplaysAcrossRestart` - cross-restart
   replay of `rk-start`: zero spawns, zero new intents (exactly one run-intent
   row survives; the command is an admission input, only the request
   fingerprint is persisted for replay).

4. **Owner kill mid-flight: custody debris refuses new authorization until
   accounted for; cancel cleans conservatively and idempotently.**
   `TestRestartMatrix_KillLeavesCustodyDebrisRefused` - resume over the killed
   attempt is refused `RUN_CUSTODY_UNKNOWN` (refuseUnknownSurvivingWork);
   owner cancel then accounts for the debris, a replayed cancel completes the
   same cancellation, nothing remains running, and no success is invented.

5. **Needs-you across restart: queued -> ack -> refused transitions and
   supersession without duplicate effects.** Covered by sqlite-backed tests:
   `TestGovernedTurnCrashAfterClaimBeforeProviderContactAdoptsOnceOnRestart`
   (claim survives the crash; exactly one adoption, no second provider
   contact), `TestGovernedSteerExactReplayDoesNotContactProviderAgain`,
   projection/supersession semantics in
   `internal/storage/sqlite/store/needs_you_store_test.go`
   (`TestProjectNeedsYouSupersessionDominatesEveryCommandState`,
   `TestNeedsYouReconcileRequiresAffirmativeProviderResolution`), and
   service-level answer fencing in `needs_you_test.go` (stale generation and
   invalid choice never cross the provider boundary; a superseded answer never
   dispatches).

## Kill/falsifier harness

The kill switch for the outcome cells is `sqliteLifetime.crash`: closing the
real store mid-attempt and reopening a new service on the same file. The
falsifiers are the assertions above: invented success, duplicate spawn,
duplicate durable intent, unaccountable custody, and double dispatch each fail
their cell.
