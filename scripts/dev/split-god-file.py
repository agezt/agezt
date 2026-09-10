"""Day 23 god file split — generic version.

Splits a 1000+ satır god file into:
- types.ts (exported type declarations only)
- page.tsx (everything else + Workflows() default)
- <file>.tsx (30 satır re-export)

Usage:
  python split-god-file.py <path-to-god-file> <label> [type_range ...]

Args:
  path-to-god-file  absolute or relative path to the file to split
  label             package label, e.g. "workflows" / "schedules"
  type_range        one or more "START:END" line ranges (1-based, inclusive)
                    that are pure type declarations to extract. If omitted,
                    only the page split happens (no types.ts created).

Example:
  python split-god-file.py frontend/src/features/schedules/components/Schedules.tsx schedules 757:757
"""
from __future__ import annotations

import re
import sys
from pathlib import Path


def split(src: Path, label: str, type_ranges: list[tuple[int, int]]):
    content = src.read_text(encoding="utf-8")
    lines = content.splitlines(keepends=False)

    # Collect type block lines (dedup, in order)
    seen: set[int] = set()
    type_lines: list[str] = []
    for start, end in type_ranges:
        for ln_no in range(start, end + 1):
            if ln_no in seen:
                continue
            seen.add(ln_no)
            type_lines.append(lines[ln_no - 1])

    # Build the import line for the type block (collect names from `export (interface|type) X`)
    type_names: list[str] = []
    for ln in type_lines:
        m = re.match(r"^export (?:interface|type) (\w+)", ln)
        if m:
            type_names.append(m.group(1))

    # types.ts (only if there are type blocks)
    if type_lines:
        types_content = (
            f"// types.ts — {label} type definitions (extracted from {src.name}). Day 23 god file split.\n"
            f"//\n"
            f"// Only exported type/interface declarations live here. Internal types\n"
            f"// stay in page.tsx because they aren't consumed outside the\n"
            f"// package's barrel. Public surface (page.tsx re-exports) unchanged.\n"
            "\n"
            + "\n".join(type_lines)
            + "\n"
        )
        (src.parent / "types.ts").write_text(types_content, encoding="utf-8")
        print(f"  wrote types.ts ({len(type_lines)} lines, {len(type_names)} types: {', '.join(type_names)})")
        # page.tsx type imports block
        type_imports_block = [
            "",
            f"// ---- types (extracted to ./types) ----",
            f"import type {{ {', '.join(type_names)} }} from \"./types\";",
        ]
    else:
        type_imports_block = []
        print("  no types.ts (no exported types)")

    # page.tsx = original minus the type ranges, with type import inserted
    page_lines = [ln for i, ln in enumerate(lines, start=1) if i not in seen]
    if type_imports_block:
        # Find insertion point: right after the LAST import statement.
        # Imports come in 3 shapes:
        #   import x from "y";                          (single-line)
        #   import { a, b, } from "y";                  (single-line, multi-name)
        #   import {\n  a,\n  b,\n} from "y";           (multi-line)
        # The last line of all three is `from "y"` or `from "y";` (or `} from "y";`).
        # Track the index of the LAST such line and insert after it.
        last_from_index = -1
        for i, ln in enumerate(page_lines):
            stripped = ln.strip()
            if re.match(r'^from\s+["\']', stripped):
                # `from "x";` (single-line import, the `import x` part is on a previous line)
                last_from_index = i
            elif stripped.startswith("} from "):
                # End of a multi-line `import { ... } from "y";` block
                last_from_index = i
            elif stripped.startswith("import ") and " from " in stripped:
                # `import { a, b } from "y";` on a single line — the `from "y"` is at the end
                last_from_index = i
        if last_from_index >= 0:
            insertion_index = last_from_index + 1
        else:
            # No imports found — insert at the very top (after the leading comment block)
            insertion_index = 0
        # Also skip past the blank line(s) that typically follow the import block
        while insertion_index < len(page_lines) and page_lines[insertion_index].strip() == "":
            insertion_index += 1
        page_lines = page_lines[:insertion_index] + type_imports_block + page_lines[insertion_index:]

    page_content = (
        f"// page.tsx — {label} page + helpers + sub-components (extracted from {src.name}). Day 23 god file split.\n"
        f"//\n"
        f"// Still ~{len(page_lines)} satır because the sub-components stay inline (the\n"
        f"// page uses them; future slices can peel off nodes/panels/flow once the\n"
        f"// cross-import graph is mapped out).\n"
        "\n"
        + "\n".join(page_lines)
        + "\n"
    )
    (src.parent / "page.tsx").write_text(page_content, encoding="utf-8")
    print(f"  wrote page.tsx ({len(page_lines)} lines)")

    # slim re-export
    # Discover what to re-export from the page
    page_module = src.parent / "page.tsx"
    pc = page_module.read_text(encoding="utf-8")
    re_exports = []
    for m in re.finditer(r"^export function (\w+)", pc, re.MULTILINE):
        re_exports.append(m.group(1))
    # Also include default export
    default_match = re.search(r"^export default function (\w+)", pc, re.MULTILINE)
    default_name = default_match.group(1) if default_match else None

    slim_lines = [
        f"// {src.name} — re-exports the page + types from this package's\n",
        f"// helpers + sub-components. Day 23 god file split carved the original\n",
        f"// 1000+ satır file into types.ts + page.tsx + this re-export; the public\n",
        f"// surface is preserved so consumers (features/{label}/index.ts, nav.tsx)\n",
        f"// don't change.\n",
    ]
    if default_name:
        slim_lines.append(f"export {{ {default_name} as default, {default_name} }} from \"./page\";")
    if re_exports:
        slim_lines.append("export {")
        slim_lines.append("  " + ",\n  ".join(re_exports) + ",")
        slim_lines.append("} from \"./page\";")
    if type_lines:
        slim_lines.append("export type {")
        slim_lines.append("  " + ",\n  ".join(type_names) + ",")
        slim_lines.append("} from \"./types\";")
    slim = "\n".join(slim_lines) + "\n"
    src.write_text(slim, encoding="utf-8")
    print(f"  rewrote {src.name} as re-export ({len(slim_lines)} lines)")

    return 0


def main() -> int:
    if len(sys.argv) < 3:
        print(__doc__, file=sys.stderr)
        return 1

    src = Path(sys.argv[1]).resolve()
    if not src.exists():
        print(f"  ERROR: {src} not found", file=sys.stderr)
        return 1

    label = sys.argv[2]
    type_ranges: list[tuple[int, int]] = []
    for arg in sys.argv[3:]:
        start, end = arg.split(":", 1)
        type_ranges.append((int(start), int(end)))

    return split(src, label, type_ranges)


if __name__ == "__main__":
    sys.exit(main())
