"""Day 23 (god file split - 3. attempt): extract types + page from
features/workflows/components/Workflows.tsx (1597 satır).

Approach (per user): sub-component'ler (WfNodeView, NodePanel, CopilotPanel,
RunsDrawer, ...) sayfa içinde inline kalır. Sadece type declaration'lar
types.ts'ye, Workflows() default function page.tsx'e tasinir.

Output:
  types.ts    # 8 interface blogu (WfNode + WfSettings + WfEdge + Wf +
              # WorkflowChainKind + WfNodeData + WfRFNode + FieldKind +
              # FieldSpec + WfTemplate + WfRunNodeEvent + WfRun) — ~150 satır
  page.tsx    # imports + helpers + sub-components + Workflows() default — ~1300 satır
  Workflows.tsx # 5 satır re-export

Public API: degismez. features/workflows/index.ts hâlâ `./components/Workflows`'tan import ediyor; Workflows.tsx'in default export'u page.tsx'ten re-export ediyor.
"""
from __future__ import annotations

import sys
from pathlib import Path

ROOT = Path(r"D:\Codebox\PROJECTS\AGEZT\frontend\src\features\workflows\components")
SRC = ROOT / "Workflows.tsx"

# Pure-type declaration ranges (1-based, inclusive on both ends).
# Sadece `export interface` ve `export type` deklarelerini alir.
# NOT exported olan internal type'lar (WfNodeData, WfRFNode, FieldKind,
# FieldSpec) page.tsx'te kalir — cunku barrel disindan import edilmiyorlar.
TYPE_RANGES = [
    (61,  72),   # WfNode (exported)
    (75,  79),   # WfSettings (exported)
    (80,  84),   # WfEdge (exported)
    (85,  97),   # Wf (exported)
    (181, 186),  # WorkflowChainKind (exported)
    # WfNodeData (289-295) + WfRFNode (296) — INTERNAL, stay in page.tsx
    # FieldKind (362) + FieldSpec (363-369) — INTERNAL, stay in page.tsx
    (882, 889),  # WfTemplate (exported)
    (952, 979),  # WfRunNodeEvent + WfRun (both exported)
]

def main() -> int:
    src = SRC.read_text(encoding="utf-8")
    lines = src.splitlines(keepends=False)

    # Collect type block lines (deduplicated, in order)
    seen: set[int] = set()
    type_lines: list[str] = []
    for start, end in TYPE_RANGES:
        for ln_no in range(start, end + 1):
            if ln_no in seen:
                continue
            seen.add(ln_no)
            type_lines.append(lines[ln_no - 1])

    # types.ts
    types_content = (
        "// types.ts — pure workflow type definitions (extracted from Workflows.tsx).\n"
        "// Day 23 god file split (3. attempt — minimal + types-only).\n"
        "//\n"
        "// Workflows.tsx used to be 1597 satir with everything inline. The split\n"
        "// is intentionally conservative: only type declarations move here; the\n"
        "// rest of the file (helpers + sub-components + main page) stays in\n"
        "// page.tsx as a single co-located unit. Future splits can peel off\n"
        "// nodes.tsx / panels.tsx / flow.ts once the cross-import graph is\n"
        "// mapped out (see scripts/dev/split-workflows-god-file.py for the\n"
        "// aggressive 8-file attempt that was rolled back).\n"
        "\n"
        "import type { Node as RFNode } from \"@xyflow/react\";\n"
        "import type { Tone } from \"@/lib/tone\";\n"
        "\n"
        + "\n".join(type_lines)
        + "\n"
    )
    (ROOT / "types.ts").write_text(types_content, encoding="utf-8")
    print(f"  wrote types.ts ({len(type_lines)} lines)")

    # page.tsx = the original Workflows.tsx MINUS the type ranges, with the
    # type declarations replaced by `import type { ... } from "./types"`.
    # The page is still 1300+ lines but the type/impl split is now explicit.
    page_lines = []
    for ln_no, ln in enumerate(lines, start=1):
        if ln_no in seen:
            continue
        page_lines.append(ln)

    # Replace the first removed block (WfNode at line 61) with a clean
    # section header + import line, so the import ordering is visible.
    # Strategy: insert a "type imports" block right after the existing
    # external imports, before the first remaining content.
    type_imports_block = [
        "",
        "// ---- types (extracted to ./types) ----",
        "import type {",
        "  WfNode, WfSettings, WfEdge, Wf,",
        "  WorkflowChainKind,",
        "  WfTemplate, WfRunNodeEvent, WfRun,",
        "} from \"./types\";",
    ]
    # Find insertion point: right after the imports block (line 57 was the last import)
    # The original imports ended at line 57, then line 58 was blank, line 59 was the
    # "graph model" comment, then WfNode started at line 61.
    # So the type import block should go RIGHT AFTER the last import, before the
    # graph model comment.
    # But the page_lines are now stripped of type lines. The "graph model" comment
    # IS in page_lines (lines 59-60). Let me insert the type imports before that comment.
    insertion_index = None
    for i, ln in enumerate(page_lines):
        if "graph model" in ln:
            insertion_index = i
            break
    if insertion_index is None:
        print("  ERROR: could not find graph model comment", file=sys.stderr)
        return 1
    final_page_lines = page_lines[:insertion_index] + type_imports_block + page_lines[insertion_index:]

    page_content = (
        "// page.tsx — workflow page + helpers + sub-components (extracted from Workflows.tsx).\n"
        "// Day 23 god file split (3. attempt — minimal + types-only).\n"
        "//\n"
        "// This is still ~1300 satır because all the sub-components (WfNodeView,\n"
        "// NodePanel, FieldSpec forms, CopilotPanel, RunsDrawer, etc.) and their\n"
        "// helpers (NODE_META, toFlow, summarize, workflowChainKind) stay here\n"
        "// together. The public surface is identical: the default export is still\n"
        "// `Workflows`, and features/workflows/index.ts still re-exports the same\n"
        "// CopilotPanel + RunsDrawer + 9 helpers as before.\n"
        "\n"
        + "\n".join(final_page_lines)
        + "\n"
    )
    (ROOT / "page.tsx").write_text(page_content, encoding="utf-8")
    print(f"  wrote page.tsx ({len(final_page_lines)} lines, was {len(lines) - len(type_lines)})")

    # Workflows.tsx: thin re-export (also re-exports the page's helpers + sub-
    # components so existing barrel imports from
    # features/workflows/index.ts keep working unchanged).
    slim = (
        "// Workflows.tsx — the workflows page. Re-exports the default Workflows()\n"
        "// implementation from ./page, the pure type definitions from ./types,\n"
        "// and the same sub-components + helpers (CopilotPanel, RunsDrawer,\n"
        "// toFlow, fromFlow, portsForNode, summarize, workflowChainKind,\n"
        "// workflowRunSourceLabel, runToStatus) that the original god file\n"
        "// exported. Day 23 carved the 1597-line file into types.ts + page.tsx +\n"
        "// this re-export; the public surface (consumed by nav.tsx and\n"
        "// features/workflows/index.ts) is unchanged.\n"
        "export { Workflows as default, Workflows } from \"./page\";\n"
        "export {\n"
        "  CopilotPanel,\n"
        "  RunsDrawer,\n"
        "  toFlow,\n"
        "  fromFlow,\n"
        "  portsForNode,\n"
        "  summarize,\n"
        "  workflowChainKind,\n"
        "  workflowRunSourceLabel,\n"
        "  runToStatus,\n"
        "} from \"./page\";\n"
        "export type {\n"
        "  WfNode,\n"
        "  WfSettings,\n"
        "  WfEdge,\n"
        "  Wf,\n"
        "  WorkflowChainKind,\n"
        "  WfTemplate,\n"
        "  WfRunNodeEvent,\n"
        "  WfRun,\n"
        "} from \"./types\";\n"
    )
    SRC.write_text(slim, encoding="utf-8")
    print(f"  rewrote Workflows.tsx as re-export ({len(slim.splitlines())} lines)")

    return 0

if __name__ == "__main__":
    sys.exit(main())
