# ADR 0016: Provider protocol compatibility is negotiated at runtime, never assumed from a version

**Status:** Accepted

**Date:** 2026-09-14

**Depends on:** ADR 0011 (Go control plane and non-authoritative intelligence), ADR 0013 (Codex harness reasoning mode)

## Context

The Codex app-server protocol is marked experimental upstream, its generated
schema describes only the build that produced it, and upstream offers no
stability promise. Kennel's compatibility rule was a version floor: any Codex
build newer than a tested minimum was trusted, and every Chat capability was
then advertised from a static table compiled into the driver.

That rule fails in both directions. A version number says nothing about method
shape: a newer build that renames or drops a method Kennel depends on passes
the floor and then fails mid-session as a generic `-32601` or, worse, as a
payload field that silently unmarshals to zero — presenting as a dead session
or an agent that produced no output. In the other direction, the static table
advertised features (rollback, steer, skills, MCP reload) for builds Kennel
never exercised, so the UI could offer a button guaranteed to fail at runtime.

This is not hypothetical. Between codex-cli 0.153.4 and 0.154.0 the emitted
schema changed packaging (flat files to versioned `v1`/`v2` bundles), and the
upstream documentation on `main` already describes methods no released build
ships. It also under-describes reality in the other direction: a live
handshake against 0.154.0 on 2026-09-14 showed the server accepting request
fields and returning response fields (`activePermissionProfile`) that its own
schema does not declare. Field-level schema gating would therefore
false-positive; method-level negotiation does not.

## Decision

Compatibility with an experimental provider protocol is **negotiated against
the provider's own declared surface at runtime**, per installed binary.

- Before a session, worktree, or provider thread is created, the driver asks
  the installed build for its protocol schema (a local, unauthenticated
  metadata command), parses the declared method surface, and compares it
  against Kennel's requirements.
- A **floor** of required methods — session lifecycle, turn control, streaming
  notifications, and every approval kind — must be declared with the direction
  Kennel uses. A build missing any floor method is refused with
  `ErrChatDriverIncompatible` naming the exact methods, before any side
  effect. This is the same fail-closed posture as governed execution policy.
- Each **optional** capability maps to the methods that back it. A build
  missing those methods loses exactly that capability in the negotiated set
  the UI consumes; the rest of the session is unaffected. New provider methods
  never count against compatibility: Kennel does not have to use everything a
  build declares.
- The negotiated surface is fingerprinted with the generator's digest
  algorithm and cached per binary identity (path, mtime, size), so a provider
  upgrade underneath a running daemon is renegotiated rather than served
  stale claims. Fetch failures are not cached and fail closed: Kennel never
  falls back to claiming an unverified protocol.
- Negotiated provenance — installed version, live surface digest versus the
  generated pin, degraded capabilities, missing floor — is reported through a
  driver interface and recorded in session logs. It is observability, never
  authority.
- The checked-in generated bindings stay **pinned to one provider build for
  reproducible conformance tests only**. The pin is a test fixture, not a
  runtime compatibility rule. A scheduled CI lane installs the latest
  published provider build, runs conformance, regenerates the bindings, and
  fails on unreviewed drift, so upstream changes surface as a reviewed diff
  before they surface as a user failure.

Payload-shape truth stays where it already lives: generated-type conformance
tests, the driver's live activation checks (for example, native reasoning
verifies the bounded permission profile actually activated on the opened
thread), and explicit refusals. Schema payloads are not gated at runtime
because the schema provably under-describes the server.

## Consequences

- A Codex upgrade that keeps the surface intact requires no Kennel change; one
  that removes optional methods degrades the exact feature with a named
  reason; one that breaks the floor is refused before it can strand a session.
- `generate-json-schema` becomes load-bearing metadata. Its removal is itself
  detectable drift and fails closed.
- Other providers keep their own compatibility stories. Claude rides the ACP
  adapter's initialize-time version and capability negotiation; Pi has no
  governed path yet. This ADR sets the bar any new experimental transport must
  meet: negotiate, degrade honestly, never assume.
- The drift CI detects upstream change but does not itself update bindings;
  regeneration is a reviewed human commit, per the repository's promotion
  rules.
