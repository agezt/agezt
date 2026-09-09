#!/usr/bin/env python3
"""Bulk-rewrite the cmd/agt dial and encodeJSON shims.

Replaces:
  - dial(stderr)                -> dialpkg.New(stderr)
  - dialBase(base, stderr)      -> dialpkg.NewAtBase(base, stderr)
  - encodeJSON(w, v)            -> jsonout.Write(w, v)

then ensures each touched file imports:
  - dialpkg "github.com/agezt/agezt/cmd/agt/dial"  (or merges with existing)
  - "github.com/agezt/agezt/cmd/agt/jsonout"

Skips:
  - files in cmd/agt/dial/, cmd/agt/jsonout/, cmd/agt/keys/, etc. (new packages)
  - *_test.go (kept on shim for now; tests still work because shim exists)
  - main.go and cmd_register.go (the shim home)
  - docs/ and other trees

Run:  python scripts/dev/rewrite-dial-jsonout-shims.py
Dry:  python scripts/dev/rewrite-dial-jsonout-shims.py --dry
"""
from __future__ import annotations

import argparse
import re
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
AGT = ROOT / "cmd" / "agt"

# Match dial(stderr) but not dial.Base( or dialFoo(
DIAL_RE = re.compile(r'\bdial\((stderr)\)')
# Match dialBase(<ident>, stderr) — first arg is a single Go identifier
DIAL_BASE_RE = re.compile(r'\bdialBase\((\w+),\s*(stderr)\)')
# Match encodeJSON(<args>) — capture the full arg list
ENCODE_RE = re.compile(r'\bencodeJSON\(')

# Files we touch
CANDIDATES = [
    "acp.go", "agent.go", "approvals_log.go", "artifact.go", "backup.go",
    "main.go",
    "budget.go", "budget_check.go", "cache.go", "catalog_sync.go",
    "channel.go", "changelog.go", "check.go", "compare.go", "config.go", "configcenter.go",
    "conductor.go", "disk.go", "doctor.go", "edict.go", "edict_overlay.go",
    "execution_profile.go", "inbox.go", "journal_export.go", "journal_grep.go",
    "journal_head.go", "journal_import.go", "journal_stats.go",
    "journal_tail.go", "ha.go", "listeners.go", "market.go", "mcp.go",
    "memory.go", "memory_log.go", "netguard.go", "okr.go", "overseer.go",
    "peers.go", "plan_cost.go", "plan_dryrun.go", "plan_history.go",
    "plan_refine.go", "plan_validate.go", "plan_visualize.go", "plugin.go",
    "provider.go", "provider_chatgpt.go", "provider_connect.go",
    "provider_cost.go", "provider_import.go", "provider_log.go",
    "provider_setup.go", "pulse.go", "pulse_control.go", "quickstart.go",
    "ratelimit.go", "redact.go", "reflect.go", "research.go", "rollback.go",
    "runs.go", "schedule.go", "schedule_test_cmd.go", "seat.go", "send.go",
    "shutdown.go", "skill.go", "skill_diff.go", "skill_export.go",
    "skill_files.go", "skill_import.go", "skill_md.go", "skill_registry.go",
    "skill_registry_remote.go", "skill_workshop.go", "standing.go",
    "state.go", "status.go", "taste.go", "tenant.go", "tool.go",
    "toolforge.go", "token.go", "transcribe.go", "vault.go", "warden.go",
    "web.go", "webhook.go", "whoami.go", "why.go",
    "workboard.go", "workflow.go", "world.go", "world_log.go",
    "okr_runbook_test_skip.go",
]

# Files we never touch (shim home, already-dial-shim, new packages)
SKIP = {
    "commands.go", "dial_test.go",
    "provider_lookup.go",  # moved to providerlookup/
}


def process(path: Path, dry: bool) -> tuple[int, int]:
    text = path.read_text(encoding="utf-8")
    orig = text

    n_dial = 0
    n_dialbase = 0
    n_encode = 0

    # encodeJSON: walk the text manually, find each `encodeJSON(`,
    # then find the matching `)`, and rebuild the text. The regex
    # approach ran into a Python-3.14-specific re.sub callable
    # return-value quirk; manual walk sidesteps it.

    out_parts: list[str] = []
    i = 0
    while i < len(orig):
        m = ENCODE_RE.search(orig, i)
        if not m:
            out_parts.append(orig[i:])
            break
        # Append everything before the match
        out_parts.append(orig[i:m.start()])
        # Walk parens to find the matching close
        depth = 1
        j = m.end()
        while j < len(orig) and depth > 0:
            ch = orig[j]
            if ch == "(": depth += 1
            elif ch == ")": depth -= 1
            j += 1
        args = orig[m.end():j - 1]
        out_parts.append(f"jsonout.Write({args})")
        n_encode += 1
        i = j  # one past the closing paren
    text = "".join(out_parts)

    def sub_dial(m: re.Match[str]) -> str:
        nonlocal n_dial
        n_dial += 1
        return "dialpkg.New(stderr)"

    def sub_dialbase(m: re.Match[str]) -> str:
        nonlocal n_dialbase
        n_dialbase += 1
        return f"dialpkg.NewAtBase({m.group(1)}, stderr)"

    text = DIAL_RE.sub(sub_dial, text)
    text = DIAL_BASE_RE.sub(sub_dialbase, text)

    if text == orig:
        return 0, 0

    # Ensure imports
    if (n_dial or n_dialbase) and '"github.com/agezt/agezt/cmd/agt/dial"' not in text:
        # The shim home used the bare "dial" alias; for the rewritten
        # form we need the dialpkg alias.
        if "dialpkg \"" in text or 'dialpkg "' in text:
            pass  # already there with alias
        else:
            # Replace the bare "dial" import with the dialpkg alias if the
            # bare form is present; otherwise append.
            if 'dialpkg "github.com/agezt/agezt/cmd/agt/dial"' in text:
                pass
            elif '"github.com/agezt/agezt/cmd/agt/dial"' in text:
                # Replace the bare import with the alias.
                text = text.replace(
                    '"github.com/agezt/agezt/cmd/agt/dial"',
                    'dialpkg "github.com/agezt/agezt/cmd/agt/dial"',
                )
            else:
                # Add a new import inside the import block.
                # Find the last "github.com/agezt/agezt/..." import line
                # and insert after it.
                import_re = re.compile(r'(\s*"github\.com/agezt/agezt/[^"]+"\n)(?=\s*\))', re.MULTILINE)
                m = import_re.search(text)
                if m:
                    text = text[:m.end()] + '\tdialpkg "github.com/agezt/agezt/cmd/agt/dial"\n' + text[m.end():]
                else:
                    # Fallback: insert after the import block opening line.
                    text = re.sub(
                        r'(import \(\n)',
                        r'\1\tdialpkg "github.com/agezt/agezt/cmd/agt/dial"\n',
                        text, count=1,
                    )

    if n_encode and '"github.com/agezt/agezt/cmd/agt/jsonout"' not in text:
        import_re = re.compile(r'(\s*"github\.com/agezt/agezt/[^"]+"\n)(?=\s*\))', re.MULTILINE)
        m = import_re.search(text)
        if m:
            text = text[:m.end()] + '\t"github.com/agezt/agezt/cmd/agt/jsonout"\n' + text[m.end():]
        else:
            text = re.sub(
                r'(import \(\n)',
                r'\1\t"github.com/agezt/agezt/cmd/agt/jsonout"\n',
                text, count=1,
            )

    n_changes = n_dial + n_dialbase + n_encode
    if not dry:
        path.write_text(text, encoding="utf-8")
    return n_changes, n_dial + n_dialbase + n_encode


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--dry", action="store_true")
    args = ap.parse_args()

    total_files = 0
    total_changes = 0
    for name in CANDIDATES:
        if name in SKIP:
            continue
        path = AGT / name
        if not path.exists():
            print(f"SKIP (missing): {name}")
            continue
        n, n_actual = process(path, args.dry)
        if n > 0:
            total_files += 1
            total_changes += n
            tag = "DRY" if args.dry else "OK"
            print(f"{tag} {name}: {n} changes")
    print()
    print(f"Total: {total_files} files, {total_changes} call-site changes"
          + (" (DRY RUN)" if args.dry else ""))
    return 0


if __name__ == "__main__":
    sys.exit(main())
