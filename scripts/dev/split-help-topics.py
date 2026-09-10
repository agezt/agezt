"""Day 5b: split app/help/help.ts (2864 satır) into 6 topic modules +
types module + aggregator.

Input:  frontend/src/app/help/help.ts (single HELP record + interfaces +
FALLBACK_TOPIC + helpTopicFor)

Output:
  frontend/src/app/help/types.ts          HelpItem + HelpSection + HelpTopic
  frontend/src/app/help/converse.ts       8 topic (jarvis, chat, voice, inbox,
                                          artifacts, data, board, approvals)
  frontend/src/app/help/monitor.ts        12 topic (mission, health, activity,
                                          autonomy, taste, seats, okr, alerts,
                                          feed, insights, runs, budget)
  frontend/src/app/help/agents.ts         15 topic (agents, agent, roster,
                                          overseer, council, conductor,
                                          research, toolforge, mcp, acp,
                                          sandbox, flow, replay, analyst,
                                          search)
  frontend/src/app/help/automation.ts     3 topic (workflows, schedules,
                                          standing)
  frontend/src/app/help/knowledge.ts      4 topic (memory, world, skills,
                                          reflect)
  frontend/src/app/help/system.ts         22 topic (overview, setup, toolbox,
                                          channels, market, persona, prompts,
                                          configcenter, connections,
                                          providers, models, routing, chains,
                                          tools, catalog, policy, cache,
                                          storage, backup, wizards, workboard,
                                          incident)
  frontend/src/app/help/help.ts           aggregator: HELP + FALLBACK_TOPIC +
                                          helpTopicFor, imports from 6 topic
                                          files + types.ts
"""
from __future__ import annotations

import re
import sys
from pathlib import Path

ROOT = Path(r"D:\Codebox\PROJECTS\AGEZT\frontend\src\app\help")
SRC = ROOT / "help.ts"

# Section markers in source order
SECTIONS = [
    ("converse",  "Converse",   32,   429),   # jarvis..approvals (8 topics)
    ("monitor",   "Monitor",    431,  904),   # mission..budget   (12 topics)
    ("agents",    "Agents",     906,  1585),  # agents..search     (15 topics)
    ("automation","Automation", 1587, 1748),  # workflows..standing (3 topics)
    ("knowledge", "Knowledge",  1750, 1932),  # memory..reflect    (4 topics)
    ("system",    "System",     1934, 2843),  # overview..incident (22 topics)
]

def main() -> int:
    src = SRC.read_text(encoding="utf-8")
    lines = src.splitlines(keepends=False)

    # Extract the HELP record (everything between the first 'export const HELP:'
    # line and the closing '};' on line 2845)
    # Header: lines 0..29 (interfaces + export const HELP: = {)
    # Body:   lines 30..2844 (the topics)
    # Tail:   lines 2845..end (FALLBACK_TOPIC + helpTopicFor)

    # Find the exact opening of HELP
    help_open = None
    for i, ln in enumerate(lines):
        if ln.strip().startswith("export const HELP: Record<string, HelpTopic> = {"):
            help_open = i
            break
    assert help_open is not None, "could not find HELP open"
    # Header = lines[0..help_open]
    header = lines[: help_open + 1]  # includes the '{' line

    # Find the closing of HELP (the first '};' on its own line after help_open)
    help_close = None
    for j in range(help_open + 1, len(lines)):
        if lines[j].strip() == "};":
            help_close = j
            break
    assert help_close is not None, "could not find HELP close"
    # Tail = lines[help_close+1..end]
    tail = lines[help_close + 1 :]

    # Body = lines[help_open+1 .. help_close-1]
    # We further split body into the 6 sections.
    body = lines[help_open + 1 : help_close]

    # Section boundaries are given as 1-based source line numbers.
    # Convert to 0-based indices in `body` (where body[0] == line help_open+1).
    section_data = []
    for slug, label, start, end in SECTIONS:
        # 1-based `start` line corresponds to 0-based body index = start - (help_open+1) - 1
        idx_start = start - (help_open + 1) - 1
        idx_end = end - (help_open + 1) - 1
        section_data.append((slug, label, idx_start, idx_end))

    # ─── Write types.ts ──────────────────────────────────────────────
    types_path = ROOT / "types.ts"
    types_path.write_text(
        "// app/help/types.ts — shared types for the in-app manual.\n"
        "// Day 5b split (see scripts/dev/split-help-topics.py).\n"
        "\n"
        "export interface HelpItem {\n"
        "  /** Short bold lead-in — a control, concept, or column on the page. */\n"
        "  term: string;\n"
        "  /** What it does / how to use it. */\n"
        "  desc: string;\n"
        "}\n"
        "\n"
        "export interface HelpSection {\n"
        "  heading: string;\n"
        "  paragraphs?: string[];\n"
        "  items?: HelpItem[];\n"
        "}\n"
        "\n"
        "export interface HelpTopic {\n"
        "  title: string;\n"
        "  /** One- or two-sentence orientation: what this page is for. */\n"
        "  intro: string;\n"
        "  sections: HelpSection[];\n"
        "  /** Practical \"did you know\" pointers, rendered as callouts. */\n"
        "  tips?: string[];\n"
        "  /** Other views that complete the story; chips navigate there. */\n"
        "  related?: { id: string; label: string }[];\n"
        "}\n",
        encoding="utf-8",
    )
    print(f"  wrote {types_path.name}")

    # ─── Write the 6 topic files ────────────────────────────────────
    header_for_section = (
        "// app/help/{slug}.ts — the {label} section of the in-app manual.\n"
        "// Day 5b split (see scripts/dev/split-help-topics.py).\n"
        "// Topics: {topics}\n"
        "//\n"
        "// Each file exports a Record<string, HelpTopic> with just the\n"
        "// topics that fall under the {label} section. The aggregator in\n"
        "// help.ts spreads all six into the single HELP record. Topics\n"
        "// remain plain data so they stay testable and tree-shakeable;\n"
        "// <HelpDrawer> owns all presentation.\n"
        "\n"
        "import type {{ HelpTopic }} from \"./types\";\n"
        "\n"
        "export const {exportName}: Record<string, HelpTopic> = {{\n"
    )

    for slug, label, idx_start, idx_end in section_data:
        section_lines = body[idx_start : idx_end + 1]
        # Find topic ids in this slice (the first 2-space-id + colon + brace pattern)
        topic_ids = []
        for ln in section_lines:
            m = re.match(r"^  ([a-z][a-z0-9]*):\s*\{$", ln)
            if m:
                topic_ids.append(m.group(1))
        # exportName is the slug uppercased first letter (e.g. converse -> Converse)
        export_name = slug[0].upper() + slug[1:]
        out = (
            header_for_section.format(
                slug=slug,
                label=label,
                topics=", ".join(topic_ids),
                exportName=export_name,
            )
            + "\n".join(section_lines)
            + "\n};\n"
        )
        (ROOT / f"{slug}.ts").write_text(out, encoding="utf-8")
        print(f"  wrote {slug}.ts ({len(topic_ids)} topics: {', '.join(topic_ids)})")

    # ─── Rewrite help.ts as aggregator ──────────────────────────────
    # Keep the original module-level comment, drop the interfaces + body,
    # import from 6 topic files + types, spread into HELP, re-export
    # FALLBACK_TOPIC + helpTopicFor.
    # The header already has lines[0..help_open] — we need to also keep the
    # tail (FALLBACK_TOPIC + helpTopicFor) verbatim.
    new_help = []
    # Module-level comment
    new_help.append(
        "// help.ts — the in-app manual. Aggregates the 6 topic sections\n"
        "// (converse, monitor, agents, automation, knowledge, system)\n"
        "// into the single HELP record, plus the dispatcher and fallback.\n"
        "// Day 5b split: each section now lives in its own file under\n"
        "// app/help/ so the 64-topic dictionary isn't one 2864-line\n"
        "// god-file anymore. The public API is unchanged — HelpTopic +\n"
        "// HELP + FALLBACK_TOPIC + helpTopicFor still import from\n"
        "// @/app/help just as before.\n"
    )
    # Imports
    new_help.append("import { Converse } from \"./converse\";")
    new_help.append("import { Monitor } from \"./monitor\";")
    new_help.append("import { Agents } from \"./agents\";")
    new_help.append("import { Automation } from \"./automation\";")
    new_help.append("import { Knowledge } from \"./knowledge\";")
    new_help.append("import { System } from \"./system\";")
    new_help.append("import type { HelpTopic } from \"./types\";")
    new_help.append("")
    new_help.append("export type { HelpTopic } from \"./types\";")
    new_help.append("")
    new_help.append("export const HELP: Record<string, HelpTopic> = {")
    new_help.append("  ...Converse,")
    new_help.append("  ...Monitor,")
    new_help.append("  ...Agents,")
    new_help.append("  ...Automation,")
    new_help.append("  ...Knowledge,")
    new_help.append("  ...System,")
    new_help.append("};")
    new_help.append("")
    # Tail verbatim (FALLBACK_TOPIC + helpTopicFor) — but skip the empty
    # lines that the source had, keeping minimal whitespace.
    for ln in tail:
        # Drop the very first blank line right after the closing brace
        if ln.strip() == "":
            continue
        new_help.append(ln)

    (ROOT / "help.ts").write_text("\n".join(new_help) + "\n", encoding="utf-8")
    print(f"  rewrote help.ts as aggregator ({len(new_help)} lines)")

    return 0

if __name__ == "__main__":
    sys.exit(main())
