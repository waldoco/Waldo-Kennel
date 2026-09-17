# Stage 3 trusted adapter release resolver boundary

- Baseline: adapter install candidate `4abf35a32ba451a2046623d2ad88ece390ca55b8`
- Scope: verified release-index selection only

The resolver never decides that bytes are signed or trusted. It accepts opaque envelope bytes only through `HarnessReleaseTrustVerifier`, an explicit distribution-policy seam. Until a reviewed verifier exists, `UnavailableHarnessReleaseTrustVerifier` returns `ErrHarnessAdapterReleaseTrustUnavailable`; no self-described key, digest, URL, publisher string or repository content upgrades its own trust.

A verified index binds one trust-root identity, monotonic index sequence, expiry, content digest and target releases. Each release binds adapter ID, release sequence, version, artifact + manifest digests, OS/architecture, size and an expiry no later than the index. Resolution validates the whole index, rejects duplicate target/version entries and selects the highest release sequence strictly above the caller's trusted floor for one exact adapter/OS/architecture.

Signing algorithm, root rotation, threshold, transparency, network retrieval and caching remain distribution decisions. They must be supplied by a reviewed verifier rather than guessed in this slice.
