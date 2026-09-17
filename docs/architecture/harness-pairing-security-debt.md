# Harness pairing security debt

Status: accepted limitation for the early release candidate. Closing slice: B2.1.

The local pairing adapter currently activates an owner-approved pairing intent when a caller presents its intent ID and immutable intent digest. It returns the resulting one-time challenge ID and possession secret over the protected local pairing transport. The local peer check restricts this transport to the daemon owner's OS account, but it does not authenticate the caller as the exact Kennel-launched harness process.

As a result, another process already running under the same OS user could race the intended harness after learning the intent ID and digest, receive the one-time secret first, and attempt to impersonate that harness. The intent digest, expiry, one-winner activation transaction, one-time proof consumption, and connection-generation fences limit other races and replay, but they do not close this exact-process delivery gap.

B2.1 closes the gap in the daemon session-launch path. Pairing proposals will bind a specific Kennel session/spawn reservation. After desktop-main owner confirmation, the daemon will activate the intent as part of that launch and pass the secret only to the selected child through a child-only inherited pipe implemented by each runtime/pty host. The public REST surface and renderer will never receive the secret, and adapter `request_challenge` will return only the challenge ID. B2.1 requires cross-platform coverage for the tmux and ConPTY/pty-host launch paths and wrong-process/same-UID adversarial tests.

Until B2.1 lands, do not describe the local pairing transport as exact-process authenticated or harness-bound. This accepted limitation applies only to the early build and remains release security debt, not the target security model.
