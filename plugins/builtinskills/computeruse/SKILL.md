---
name: computer-use
description: Control the machine — install/update/remove software, and automate the desktop GUI (screenshot, click, type, hotkeys) when a task needs the real computer, not just a browser
triggers: [computer, desktop, gui, install, uninstall, screenshot, click, keyboard, app, software, automate]
tools: [shell, code_exec, artifacts]
---

# Computer use — full machine control

When a task needs the actual computer — install an app, run a CLI, or click
through a desktop GUI — use this. Two halves:

1. **Software management** via the `shell` tool. You have full machine
   permission: install, update, and remove software with the host's package
   manager (winget/choco on Windows, brew on macOS, apt on Linux) or language
   managers (npm/pip/cargo). If a command is missing, install it, then use it.

2. **GUI automation** via `scripts/desktop.py`, run through `code_exec`
   (language: python). It screenshots the screen, clicks at coordinates, types,
   sends hotkeys, scrolls, and can locate a control by a template image. A
   desktop session is required (it drives the real screen/keyboard); on a
   headless host it returns a clear error.

## One-time setup

Run `scripts/setup.sh` once (via code_exec or shell): it installs `pyautogui` +
`pillow`. Use `skill op=files computer-use` to find the bundle directory.

## Driving the desktop

`scripts/desktop.py` takes a JSON spec — an ordered list of actions, plus a final
screenshot by default. Run it with `code_exec`:

```
python scripts/desktop.py '{"actions":[{"type":"screenshot"}],"screenshot":true}'
```

See `reference/patterns.md` for the full action set (move/click/double_click/
type/press/scroll/wait/locate), package-manager recipes per OS, and the
see-then-act loop.

## Loop

1. Screenshot; read the PNG back to see the screen.
2. Decide the next action from what you see (find the control's coordinates, or
   `locate` it by a template image).
3. Click / type / press; screenshot again. Repeat until done.

Register screenshots with the `artifacts` tool to view them in the Files view.
Be deliberate with destructive actions — read the screen before confirming.
