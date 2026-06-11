# git-ops recipes

The helper (`scripts/gitflow.sh`) covers the safe path: sync → branch → save →
pr. For everything else, raw git/gh is fine. Common patterns:

## Clone a repo and open a PR end to end

```sh
gh repo clone owner/name && cd name        # or: git clone <url>
scripts/gitflow.sh branch feat/my-change
# ... edit files ...
scripts/gitflow.sh save "Feat: my change"
scripts/gitflow.sh pr "Feat: my change" "What and why."
```

## Check authentication before you push

```sh
gh auth status            # who am I, which scopes
git remote -v             # where will a push go?
```
If `gh auth status` fails, the operator must run `gh auth login` themselves —
it's interactive. Don't fabricate a token.

## Undo a bad commit (not yet pushed)

```sh
git reset --soft HEAD~1     # keep the changes staged, drop the commit
git reset --hard HEAD~1     # discard the commit AND its changes (careful)
```

## Amend the last commit

```sh
git add -A && git commit --amend -m "Better message"
# if already pushed to YOUR feature branch only:
git push --force-with-lease
```

## Cherry-pick a commit onto your branch

```sh
git switch your-branch
git cherry-pick <sha>
```

## Resolve a merge/rebase conflict

```sh
scripts/gitflow.sh sync
git switch your-branch
git rebase main
# for each conflict: edit the file, then:
git add <file>
git rebase --continue
# bail out safely if it gets messy:
git rebase --abort
```

## Work in a throwaway worktree (isolate risky edits)

```sh
git worktree add ../scratch -b experiment/thing
cd ../scratch
# ... experiment freely; the main checkout is untouched ...
git worktree remove ../scratch        # when done
```

## Inspect before acting

```sh
git log --oneline -10
git diff --stat main...HEAD           # what this branch changes vs main
gh pr list --state open               # open PRs on the repo
gh pr checks <number>                 # CI status for a PR
```

## Self-improvement loop

When improving your own tooling/code: clone or `cd` into the repo, branch, make
the change, run its tests, `save`, then `pr`. Keep the change small and
reviewable — one concern per PR — so the operator can approve quickly.
