"""Day 9: bulk-replace import paths for the features/runs/ carve-out.

Replaces three patterns in place across frontend/src/:
  @/lib/rundetail  -> @/features/runs/lib/rundetail
  @/lib/runfocus   -> @/features/runs/lib/runfocus
  @/views/Runs     -> @/features/runs/components/Runs

The scripts skips:
  - node_modules, dist, .git
  - any file under features/runs/ itself that does NOT need rewriting
    (the carved files now use the new paths; their tests self-import)
"""
from __future__ import annotations

import os
import sys
from pathlib import Path

ROOT = Path(r"D:\Codebox\PROJECTS\AGEZT\frontend\src")

REWRITES = [
    ('"@/lib/rundetail"', '"@/features/runs/lib/rundetail"'),
    ("'@/lib/rundetail'", "'@/features/runs/lib/rundetail'"),
    ('"@/lib/runfocus"', '"@/features/runs/lib/runfocus"'),
    ("'@/lib/runfocus'", "'@/features/runs/lib/runfocus'"),
    ('"@/views/Runs"', '"@/features/runs/components/Runs"'),
    ("'@/views/Runs'", "'@/features/runs/components/Runs'"),
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
        for old, new in REWRITES:
            if old in updated:
                updated = updated.replace(old, new)
        if updated != original:
            path.write_text(updated, encoding="utf-8", newline="")
            # count how many replacements happened in total
            n = sum(original.count(o) - updated.count(o) for o, _ in REWRITES)
            touched.append((path, n))
    touched.sort()
    for path, n in touched:
        rel = path.relative_to(ROOT.parent.parent)
        print(f"  {n:3d}  {rel}")
    print(f"\nTotal files touched: {len(touched)}")
    return 0

if __name__ == "__main__":
    sys.exit(main())
