"""Day 16: bulk-replace import paths for the features/autonomy/,
features/overseer/, and features/sandbox/ carve-outs.

Replaces patterns in place across frontend/src/:
  autonomy:
    @/lib/autonomy   -> @/features/autonomy/lib/autonomy
    @/views/Autonomy -> @/features/autonomy/components/Autonomy
  overseer:
    @/views/Overseer -> @/features/overseer/components/Overseer
  sandbox:
    @/views/Sandbox  -> @/features/sandbox/components/Sandbox

Skips node_modules, dist, .git, .wrongstack.
"""
from __future__ import annotations

import os
import sys
from pathlib import Path

ROOT = Path(r"D:\Codebox\PROJECTS\AGEZT\frontend\src")

REWRITES = [
    # autonomy
    ('"@/lib/autonomy"',     '"@/features/autonomy/lib/autonomy"'),
    ("'@/lib/autonomy'",     "'@/features/autonomy/lib/autonomy'"),
    ('"@/views/Autonomy"',   '"@/features/autonomy/components/Autonomy"'),
    ("'@/views/Autonomy'",   "'@/features/autonomy/components/Autonomy'"),
    # overseer
    ('"@/views/Overseer"',   '"@/features/overseer/components/Overseer"'),
    ("'@/views/Overseer'",   "'@/features/overseer/components/Overseer'"),
    # sandbox
    ('"@/views/Sandbox"',    '"@/features/sandbox/components/Sandbox"'),
    ("'@/views/Sandbox'",    "'@/features/sandbox/components/Sandbox'"),
]

SKIP_DIRS = {".git", "node_modules", "dist", "build", "coverage"}

def iter_files(root: Path):
    for dirpath, dirnames, filenames in os.walk(root):
        dirnames[:] = [d for d in dirnames if d not in SKIP_DIRS]
        for name in filenames:
            p = Path(dirpath) / name
            if p.suffix in {".ts", ".tsx", ".js", ".jsx", ".mjs", ".cjs"}:
                yield p

def main() -> int:
    touched: list[tuple[Path, int]] = []
    for path in iter_files(ROOT):
        try:
            original = path.read_text(encoding="utf-8")
        except (UnicodeDecodeError, OSError):
            continue
        updated = original
        n = 0
        for old, new in REWRITES:
            if old in updated:
                count = updated.count(old)
                updated = updated.replace(old, new)
                n += count
        if updated != original:
            path.write_text(updated, encoding="utf-8", newline="")
            touched.append((path, n))
    touched.sort()
    for path, n in touched:
        rel = path.relative_to(ROOT.parent.parent)
        print(f"  {n:3d}  {rel}")
    print(f"\nTotal files touched: {len(touched)}")
    return 0

if __name__ == "__main__":
    sys.exit(main())
