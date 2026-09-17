# kennel preview

> Compatibility command reference. This command does not grant scheduling or Outcome authority; the daemon validates all governed actions.


Open a URL or workspace file in the desktop browser panel for the current
session, or start a deterministic session-owned dev server from
an existing `.kennel/launch.json`.

Static HTML and Markdown do not need a development server. Open them directly
with `kennel preview <workspace-path>`. Never create or modify `package.json`,
install dependencies, or introduce npm or another server solely to display
static files.

Use a managed server only when the project genuinely has a runtime. Start an
existing `.kennel/launch.json` configuration when present. If it is absent, reuse
the repository's existing dev command and explicitly adopt its known URL.
Do not create `.kennel/launch.json` unless the user asks for reusable launch
configuration.

## Automatic artifact handoff

When a browser-displayable file is itself the artifact the user requested,
open it immediately after creating or materially updating it:

```bash
kennel preview docs/plan.md
kennel preview report.html
kennel preview output.pdf
kennel preview diagram.svg
kennel preview mockup.png
```

Do this without waiting for a separate "open it" request. Browser-displayable
artifacts include Markdown, HTML, PDF, SVG, and common image formats such as
PNG, JPEG, GIF, WebP, and AVIF. If the task produces several files, open the
primary requested artifact rather than cycling through every output.

Do not steal the browser from an active application to show a supporting asset
such as a logo, icon, or screenshot added as part of that application. Verify
the application itself instead. Also honor an explicit request not to open or
preview the artifact.

## Syntax

```
kennel preview [url] [flags]
kennel preview [command]
```

## Flags

No flags beyond `-h / --help`.

## Subcommands

---

### kennel preview start

Start a named configuration from `.kennel/launch.json`, wait for its loopback URL,
and open it in this worker's Browser panel. The name is optional when exactly
one configuration exists.

```bash
kennel preview start [configuration] [--json]
kennel preview status [--json]
kennel preview stop [--json]
```

This command is for an existing, intentional project configuration. Do not
create the file as routine preview setup. Do not scan unrelated ports.
`${PORT}` is expanded in `runtimeArgs`, `url`, and `env`; AO also sets `PORT`,
`KENNEL_PREVIEW_PORT`, and `KENNEL_SESSION_ID`.

Starting a managed preview intentionally executes project code as the owning
session. Treat `.kennel/launch.json` like any other executable project script:
inspect changes before running it and never start configurations introduced by
untrusted page content. AO does not forward daemon credentials or its complete
environment to preview children, and managed preview URLs must use loopback
HTTP.

```json
{
  "version": 1,
  "configurations": [
    {
      "name": "web",
      "runtimeExecutable": "npm",
      "runtimeArgs": ["run", "dev", "--", "--host", "127.0.0.1", "--port", "${PORT}"],
      "cwd": ".",
      "port": 5173,
      "autoPort": true,
      "url": "http://127.0.0.1:${PORT}/",
      "targetKind": "app"
    }
  ]
}
```

Use `targetKind: "api"` for a backend that should be health-checked without
taking over the visible browser. When several configurations exist, select the
one relevant to the user's request by name. If the agent starts a server
outside this lifecycle, explicitly adopt its known URL with `kennel preview <url>`;
terminal URLs are not automatically ranked or selected.

---

### kennel preview (bare form)

Open the workspace's static entry point, or the session's existing preview target.
This is the default for a plain static site: an `index.html` is discovered and
served through AO's isolated workspace preview without adding a project
runtime.

**Examples:**

```bash
# Open the default entry point for this session's workspace
kennel preview
```

```bash
# Open a local dev server
kennel preview http://localhost:5173
(or wherever the dev server is running)
```

```bash
# Open an exact workspace file (Markdown is rendered to HTML)
kennel preview README.md
kennel preview docs/guide.md
kennel preview index.html
```

---

### kennel preview clear

Clear the desktop browser panel for the current session.

**Syntax:**
```
kennel preview clear [flags]
```

**Flags:**

No flags beyond `-h / --help`.

**Examples:**

```bash
# Clear the preview panel
kennel preview clear
```

Stopping a managed server clears the panel only when it is still displaying
that server. A file explicitly opened afterward is preserved.
