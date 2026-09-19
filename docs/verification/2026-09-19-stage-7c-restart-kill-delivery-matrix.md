# Stage 7C - restart / kill / delivery-unknown matrix

Every cell runs against the real sqlite store. Outcome-service cells simulate a
daemon kill by closing the store mid-flight and building a fresh service over
the same data directory (`sqliteLifetime` in
`backend/internal/service/outcome/restart_matrix_test.go`). The only fake in
those cells is provider liveness evidence (`fakeHeartbeats`): it holds no
durable state, so a fresh lifetime can prove nothing about pre-crash sessions
until termination evidence is recorded - sqlite is the only durability.
Chat governed-turn cells run a replacement controller generation over the same
durable store (`sqlitetest.MustOpenAt`).

## Cells

1. **Daemon restart mid-attempt recovers exactly one continuation.**
   `TestRestartMatrix_DaemonRestartRecoversOneContinuation` - attempt left
   running, no receipt. In the fresh lifetime, evidence-free liveness mutates
   nothing (the stranded row stays running; the reconcile loop decides
   nothing without facts) and continuation mints no blind duplicate (zero
   spawns). Once durable termination evidence exists (the bound provider
   session ended with the old process), `EvaluateAttemptLiveness` settles the
   row as `reconciled` - ended and accounted for, result unclassified, never
   `succeeded`. The terminal record alone still authorizes nothing: the run
   goes `needs_you` / `attempt_replacement_required` with zero spawns. Only
   the explicit owner replacement decision (`RecoverAttempt` replace, with a
   replacement receipt; the terminal record is never rewritten) lets the next
   continuation mint exactly one fresh attempt - new row, attempt number 2,
   fresh request identity, same work unit - and repeated ticks keep it at
   exactly one.

2. **Provider kill mid-turn: delivery_unknown stays nonterminal and blocking;
   no duplicate effect on reconnect.** Covered by the sqlite-backed chat
   suite: `TestGovernedTurnCrashMidDispatchRestartsUnknownWithoutRedelivery`
   and `TestSendDeliveryUnknownStaysDurablyQueued`
   (controller_test.go, replacement controller generation over the same
   durable store via `openStore`), with the negative fence
   `TestGovernedTurnExplicitRejectionIsNotDeliveryUnknown`.

3. **Reconnect retry with the same request key returns the recorded receipt;
   no duplicate spawn, no new durable intent.**
   `TestRestartMatrix_SameRequestKeyReplaysAcrossRestart` - cross-restart
   replay of `rk-start`: zero spawns, zero new intents (exactly one
   run-intent row survives; the command is an admission input, only the
   request fingerprint is persisted for replay), and the continuation tick
   that runs after the replay still sees exactly the original attempt - same
   id, still running, no new row.

4. **Owner kill mid-flight: custody debris refuses new authorization until
   accounted for; cancel cleans conservatively and idempotently.**
   `TestRestartMatrix_KillLeavesCustodyDebrisRefused` - resume over the killed
   attempt is refused `RUN_CUSTODY_UNKNOWN`; owner cancel then accounts for
   the debris, a replayed cancel completes the same cancellation, nothing
   remains running, and no success is invented.

5. **Needs-you across restart: a durable question survives with its
   generation; stale/invalid answers are refused without effect; a durable
   successor session supersedes it; the successor's answer lands durably and
   replays idempotently after a second restart.**
   `TestRestartMatrix_NeedsYouQuestionSurvivesRestartAckRefusalAndSupersession`
   - a capability-escalation question created against the real attempt/session
   lineage in one lifetime is still open with its immutable generation in the
   next; stale-generation and invented-option answers are refused and leave
   the row untouched; a durable successor session makes the successor
   escalation the live one and the superseded row leaves the projection
   (answering it is refused at the store and unaddressable at the facade);
   the successor's deny answer resolves durably; after a second restart the
   recorded answer still stands, an exact replay returns the same receipt
   with no second effect, and a conflicting replay under a new request key is
   refused. Component-level evidence beneath this cell: the chat adoption
   fence
   (`TestGovernedTurnCrashAfterClaimBeforeProviderContactAdoptsOnceOnRestart`,
   `TestGovernedSteerExactReplayDoesNotContactProviderAgain` - real store,
   replacement controller generation), the pure projection semantics in
   `internal/storage/sqlite/store/needs_you_store_test.go`
   (`TestProjectNeedsYouSupersessionDominatesEveryCommandState`,
   `TestNeedsYouReconcileRequiresAffirmativeProviderResolution` - projection
   functions, no database), and the service-level fencing in
   `needs_you_test.go` (stale generation, invalid choice, and superseded
   answers never cross the provider boundary - in-memory store fake, no
   durability claims).

## Kill/falsifier harness

The kill switch for the outcome cells is `sqliteLifetime.crash`: closing the
real store mid-attempt and reopening a new service on the same file. The
falsifiers are the assertions above: invented success, duplicate spawn,
duplicate durable intent, unaccountable custody, blind retry of a terminal
record, and double dispatch each fail their cell.
