"""Day 11: bulk-replace @/views/Roster -> @/features/agents/components/Roster.

The 17 reference sites were updated in Day 10 to forward-compatible
paths under @/features/agents/.../lib/ for the @/lib/agent* imports,
but @/views/Roster itself still pointed at the old file. Now that
views/Roster.tsx has moved to features/agents/components/Roster.tsx,
those references need to follow.

Skips node_modules, dist, .git, .wrongstack.
"""
from __future__ import annotations

import os
import sys
from pathlib import Path

ROOT = Path(r"D:\Codebox\PROJECTS\AGEZT\frontend\src")

REWRITES = [
    ('"@/views/Roster"', '"@/features/agents/components/Roster"'),
    ("'@/views/Roster'", "'@/features/agents/components/Roster'"),
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
