#!/bin/sh
# computer-use setup: install the desktop-automation deps. Run via code_exec or shell.
# A GUI session is required at run time (pyautogui controls the real screen/keyboard).
set -e

echo "installing pyautogui + pillow (python) ..."
# Prefer pip; fall back to pip3. On Linux, pyautogui also needs python3-tk and
# scrot for screenshots — install them with the shell tool if a run reports them
# missing (you have full machine permission).
python -m pip install --quiet pyautogui pillow 2>/dev/null \
  || pip install --quiet pyautogui pillow 2>/dev/null \
  || pip3 install pyautogui pillow

echo "computer-use ready: drive it with scripts/desktop.py"
echo "(Linux note: if screenshots fail, install 'scrot' and 'python3-tk' via the shell tool.)"
