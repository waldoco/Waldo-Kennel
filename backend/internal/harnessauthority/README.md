# Harness authority surface

This slice owns durable pairing proposals, native-shell decisions, one-time
activation, revocation receipts, and renderer-safe list/detail projections.

It deliberately does not add a new `change_log.event_type`. The existing CDC
vocabulary has no honest project-level harness-authority invalidation, and
reusing `session_updated` would falsely bind connection authority to a session.
B2.75 owns freezing the shared mutation envelope and retrofitting these rows.
Until that lands, callers refresh these bounded read projections after the
preload IPC command resolves; reconnect/reload also re-reads durable state.
