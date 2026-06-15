# Worktree Strategy - AGEZT

## Current Status (2026-06-14)

| Worktree | Branch | Path | Status |
|----------|--------|------|--------|
| Primary | `main` | `D:/Codebox/PROJECTS/AGEZT` | ✅ Active |
| Secondary | `worktree-m951-webui-modernize` | `.claude/worktrees/m951-webui-modernize` | ✅ Synced |

**No difference between branches** - worktree is fully merged.

---

## Recommended Worktree Strategy

### Principle: Single Source of Truth

**Rule**: `main` is the only branch that matters. Worktrees are temporary.

### When to Use Worktrees

| Scenario | Use Worktree? | Reason |
|----------|--------------|--------|
| Long-running feature (>1 week) | ✅ Yes | Isolate from main development |
| Experimental/Risky change | ✅ Yes | Easy cleanup if experiment fails |
| Multiple people on same feature | ✅ Yes | Shared isolation |
| Quick fix (<1 day) | ❌ No | Overhead not worth it |
| Documentation only | ❌ No | No conflict risk |
| Single developer, small changes | ❌ No | Branch and PR is simpler |

### Workflow

```
1. Create worktree from main (or latest release tag)
   git worktree add ../worktree-feature-X main

2. Work in the worktree
   cd ../worktree-feature-X
   git checkout -b feature-x

3. When ready to merge:
   git checkout main
   git merge feature-x
   git push

4. Cleanup worktree when merged
   git worktree remove ../worktree-feature-X
```

### Cleanup Policy

- [ ] Remove worktree within 1 week of merge
- [ ] Delete feature branch after merge
- [ ] No stale worktrees > 2 weeks old

### Current Issue: m951 Worktree

The `worktree-m951-webui-modernize` worktree is **synced with main** and could be removed:

```bash
# Remove when confirmed no longer needed
git worktree remove .claude/worktrees/m951-webui-modernize
```

### Alternative: Branch-Based Isolation

For single developer, consider **branch-based isolation** instead of worktrees:

```bash
# Instead of worktree
git checkout -b feature-x
# Work... commit... push...
# When done:
git checkout main
git merge feature-x
git branch -d feature-x
```

**Benefits**:
- No filesystem complexity
- Easier to switch contexts
- Works better with IDE git integration

**Tradeoffs**:
- Cannot have 2 branches checked out simultaneously (without worktree)
- Stash or commit to switch

---

## Migration Plan

1. [ ] Evaluate if `worktree-m951-webui-modernize` is still needed
   - If no pending work: Remove it
   - If work in progress: Continue using it

2. [ ] If keeping worktrees: Document worktree purpose in `.project/WORKTREES.md`

3. [ ] Establish cleanup reminder in CI/CD or git hooks

---

## Quick Commands

```bash
# List all worktrees
git worktree list

# Add new worktree
git worktree add ../worktree-NAME -b feature-branch

# Remove worktree (after merge)
git worktree remove ../worktree-NAME

# Prune stale worktree references
git worktree prune
```
