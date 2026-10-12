# Launch-candidate profile isolation

The integrated launch candidate supports KENNEL_ELECTRON_DATA_DIR as an absolute Electron userData directory override. Without it, packaged and development defaults are unchanged. A relative override fails at startup rather than creating state in an arbitrary working directory.

For disposable packaged acceptance, set KENNEL_ELECTRON_DATA_DIR, KENNEL_DATA_DIR, KENNEL_RUN_FILE and KENNEL_BROWSER_RUNTIME_DIR to separate paths beneath the audit directory, and choose a free KENNEL_PORT. KENNEL_BROWSER_RUNTIME_DIR relocates Agent Browser run roots, which otherwise default to `~/.kennel/br` (`~/.kennel/dev/br` in development); like KENNEL_ELECTRON_DATA_DIR it must be absolute. Do not override HOME or reuse the production profile. This isolates Chromium state as well as daemon data. It does not bypass application permissions or provider authentication.

An explicitly isolated profile also suppresses move-to-Applications and newer-installed-app handoff. Otherwise acceptance can silently run a different bundle. Normal packaged launches retain the relocation policy.
