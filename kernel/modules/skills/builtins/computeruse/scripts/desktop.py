#!/usr/bin/env python3
"""computer-use driver — control the real desktop (screenshot, click, type, hotkeys).

Usage:  python desktop.py '<json-spec>'   (or pipe the JSON on stdin)
Spec:   { "actions": [ {type, ...}, ... ], "screenshot": true }
Output: one JSON object on stdout: { ok, screen, screenshot?, results } or { ok:false, error }.

Action types:
  {"type":"screenshot"}                         -> capture the screen now (also auto-captured at end unless screenshot:false)
  {"type":"move","x":100,"y":200}
  {"type":"click","x":100,"y":200,"button":"left"}   (button: left|right|middle)
  {"type":"double_click","x":100,"y":200}
  {"type":"type","text":"hello"}                -> type text at the current focus
  {"type":"press","keys":["ctrl","s"]}          -> a hotkey chord (pyautogui.hotkey)
  {"type":"scroll","amount":-300}               -> scroll (negative = down)
  {"type":"wait","ms":500}
  {"type":"locate","image":"/path/to/template.png"}  -> find an image on screen, returns its box

Needs a real display (GUI session). On a headless host it returns a clear error.
Install once: pip install pyautogui pillow  (see scripts/setup.sh).
"""
import json
import os
import sys
import tempfile
import time


def read_spec():
    if len(sys.argv) > 1 and sys.argv[1].strip():
        return json.loads(sys.argv[1])
    data = sys.stdin.read()
    if data.strip():
        return json.loads(data)
    raise ValueError("no spec: pass a JSON spec as argv[1] or on stdin")


def run(spec):
    try:
        import pyautogui
    except Exception as e:  # noqa: BLE001
        raise RuntimeError(
            "pyautogui is not installed or no display is available "
            "(run scripts/setup.sh; a GUI session is required): " + str(e)
        )
    pyautogui.FAILSAFE = False

    width, height = pyautogui.size()
    out = {"ok": True, "screen": {"width": width, "height": height}, "results": []}

    for a in spec.get("actions", []):
        t = a.get("type")
        if t == "screenshot":
            out["results"].append({"type": "screenshot", "path": _shot(pyautogui)})
        elif t == "move":
            pyautogui.moveTo(a["x"], a["y"], duration=0.1)
            out["results"].append({"type": "move", "ok": True})
        elif t == "click":
            pyautogui.click(a["x"], a["y"], button=a.get("button", "left"))
            out["results"].append({"type": "click", "ok": True})
        elif t == "double_click":
            pyautogui.doubleClick(a["x"], a["y"])
            out["results"].append({"type": "double_click", "ok": True})
        elif t == "type":
            pyautogui.typewrite(str(a.get("text", "")), interval=0.01)
            out["results"].append({"type": "type", "ok": True})
        elif t == "press":
            keys = a.get("keys") or []
            if len(keys) == 1:
                pyautogui.press(keys[0])
            else:
                pyautogui.hotkey(*keys)
            out["results"].append({"type": "press", "ok": True})
        elif t == "scroll":
            pyautogui.scroll(int(a.get("amount", 0)))
            out["results"].append({"type": "scroll", "ok": True})
        elif t == "wait":
            time.sleep(max(0, int(a.get("ms", 500))) / 1000.0)
            out["results"].append({"type": "wait", "ok": True})
        elif t == "locate":
            box = pyautogui.locateOnScreen(a["image"], confidence=a.get("confidence", 0.9))
            out["results"].append(
                {"type": "locate", "found": bool(box),
                 "box": ({"left": box.left, "top": box.top, "width": box.width, "height": box.height} if box else None)}
            )
        else:
            raise ValueError("unknown action type: " + str(t))

    if spec.get("screenshot", True):
        out["screenshot"] = _shot(pyautogui)
    return out


def _shot(pyautogui):
    path = os.path.join(tempfile.mkdtemp(prefix="computeruse-"), "screen.png")
    pyautogui.screenshot(path)
    return path


def main():
    try:
        print(json.dumps(run(read_spec())))
    except Exception as e:  # noqa: BLE001
        print(json.dumps({"ok": False, "error": str(e)}))
        sys.exit(1)


if __name__ == "__main__":
    main()
