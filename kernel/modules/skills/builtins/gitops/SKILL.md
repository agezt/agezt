---
name: git-ops
description: Work with git and GitHub safely — clone a repo, branch, commit with clear messages, push, and open a pull request with the gh CLI, resolve conflicts, and keep main clean — when a task involves changing code in a repository or shipping your own improvements as a PR
triggers: [git, github, gh, commit, branch, pull request, pr, clone, merge, rebase, repo, version control]
tools: [shell, code_exec]
---

# git-ops — change code and ship it as a PR, safely

When a task means editing code in a repository — fixing a bug, adding a feature,
or improving your own tooling — do it the way a careful engineer does: on a
branch, with clear commits, opened as a pull request for review. Never push
straight to `main`. You have full shell capability; this skill is the workflow
discipline that keeps the history clean and the change reviewable.

## The rules

1. **Never commit on `main`/`master`.** Branch first. The helper refuses to
   commit on a default branch so you can't do it by reflex.
2. **One logical change per branch**, named for what it does
   (`fix/login-null-deref`, `feat/csv-export`).
3. **Clear commit messages**: a concise imperative subject line, then a body
   explaining *why* when it isn't obvious.
4. **Never force-push a shared branch** (`main`, or a branch someone else is on).
   Force-push only your own un-merged feature branch, and prefer
   `--force-with-lease`.
5. **Open a PR, don't self-merge** unless the task explicitly authorizes it.

## Preflight

```sh
git status                         # clean tree? what branch?
gh auth status                     # is the GitHub CLI authenticated?
```
If `gh` isn't installed, install it via the computer-use skill. If it isn't
authenticated, the operator must run `gh auth login` (an interactive step you
cannot do for them) — say so rather than guessing a token.

## Workflow with the helper

`scripts/gitflow.sh` wraps the safe path. Use `skill op=files git-ops` to find
the bundle directory; run it via shell or code_exec from inside the repo.

```sh
scripts/gitflow.sh sync                     # fetch + fast-forward the default branch
scripts/gitflow.sh branch fix/typo-readme   # create + switch to a feature branch
# ... make your edits ...
scripts/gitflow.sh save "Fix: typo in README intro"   # add -A + commit (refuses on main)
scripts/gitflow.sh pr "Fix: README typo" "One-line fix to the intro paragraph."
                                            # push + open a PR with gh, prints the URL
```

`save` stages all changes and commits; it aborts if you are on the default branch
and tells you to branch first. `pr` pushes the current branch upstream and opens a
pull request against the default branch.

## Doing it by hand

The helper is a fast path, not a cage — raw git/gh is fine:

```sh
git switch -c feat/thing
git add -p && git commit -m "Feat: thing"
git push -u origin feat/thing
gh pr create --fill
```

## Conflicts

```sh
scripts/gitflow.sh sync          # update default branch first
git switch your-branch
git rebase main                  # replay your work on top; resolve, git add, git rebase --continue
# or, if a rebase is risky: git merge main
```
Resolve each conflicted file, `git add` it, continue. If a rebase goes wrong,
`git rebase --abort` returns you to safety — nothing is lost.

See `reference/recipes.md` for cloning + PR end-to-end, cherry-picking, undoing a
bad commit, and working in a throwaway worktree.
