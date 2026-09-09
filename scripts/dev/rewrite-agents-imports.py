"""Day 10 (round 2): regex-rewrite the agentdetail subpath imports that
the round-1 string script missed. Round 1 only matched the bare
@/components/agentdetail path, not @/components/agentdetail/<sub>.

Also fixes the @/lib/agent (M948) comment in views/Roster.tsx.
"""
from __future__ import annotations

import os
import re
import sys
from pathlib import Path

ROOT = Path(r"D:\Codebox\PROJECTS\AGEZT\frontend\src")

# The bare subdir import may be followed by:
#   - nothing (just the dir)
#   - "/foo"  (a file in the subdir)
#   - "'; ... "  (closing quote in single quotes)
#   - "\"; ... " (closing quote in double quotes)
# Match both quote styles.
REWRITES = [
    # components/agentdetail (with optional subpath)
    (re.compile(r"""(['"])@/components/agentdetail((?:/[^'"]+)?)\1"""),
     r'\1@/features/agents/components/agentdetail\2\1'),
    # Roster.tsx stale comment about @/lib/agent
    (re.compile(r"@/lib/agent \(M948\)"),
     "@/features/agents/lib/agent"),
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
        for pat, repl in REWRITES:
            updated, count = pat.subn(repl, updated)
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
