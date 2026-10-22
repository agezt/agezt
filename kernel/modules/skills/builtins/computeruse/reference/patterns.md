# computer-use patterns

Two halves: **install/manage software** (the shell tool, full permission) and
**GUI control** (`scripts/desktop.py` via code_exec, needs a desktop session).

## Installing / updating / removing software

Use the `shell` tool. Pick the host's package manager:

- Windows: `winget install <id>` / `winget upgrade --all` / `winget uninstall <id>`
  (or `choco install <pkg>` if Chocolatey is present).
- macOS: `brew install <pkg>` / `brew upgrade` / `brew uninstall <pkg>`.
- Debian/Ubuntu: `sudo apt-get update && sudo apt-get install -y <pkg>`.
- Language tools: `npm i -g <pkg>`, `pip install <pkg>`, `cargo install <pkg>`.

You have full machine permission — if a command needs a dependency, install it,
then continue.

## GUI control (desktop.py)

Run via code_exec (language: python), passing a JSON spec. Each call runs an
ordered list of actions and (by default) returns a final screenshot path:

```json
{ "actions": [
    { "type": "screenshot" },
    { "type": "click", "x": 640, "y": 360 },
    { "type": "type", "text": "hello world" },
    { "type": "press", "keys": ["ctrl", "s"] }
  ],
  "screenshot": true }
```

### See, then act

Always start with a screenshot, read it back to find the coordinates of the
control you want, then click/type. The driver returns `screen` (the resolution)
and `screenshot` (an absolute PNG path) — register it with the `artifacts` tool
to view it in the Files view, or read it directly to look.

### Find a control by image

If you have a template PNG of a button, locate it instead of guessing
coordinates:

```json
{ "actions": [ { "type": "locate", "image": "/path/to/button.png" } ] }
```

Returns its `box` ({left, top, width, height}); click its centre.

### Notes & safety

- A GUI session is required; on a headless server the driver returns a clear
  error — there install nothing and report back.
- Be deliberate with destructive UI actions; prefer keyboard shortcuts and
  confirm dialogs by reading the screenshot first.
- Never type secrets in plain sight if a message will be shown; pull them from
  the vault/config.
