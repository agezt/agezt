#!/bin/sh
# git-ops safe-workflow helper. Keeps main clean: branch, commit, push, PR.
# Run from inside the target repository.
#
# Usage:
#   gitflow.sh sync                       # fetch + fast-forward the default branch
#   gitflow.sh branch <name>              # create + switch to a feature branch
#   gitflow.sh save "<message>"           # add -A + commit (refuses on default branch)
#   gitflow.sh pr "<title>" ["<body>"]    # push current branch + open a PR via gh
#   gitflow.sh status                     # branch, upstream, short status
set -e

die() { echo "gitflow: $*" >&2; exit 1; }
in_repo() { git rev-parse --is-inside-work-tree >/dev/null 2>&1 || die "not a git repository"; }

# default_branch resolves origin's HEAD (main/master); falls back to main.
default_branch() {
  b="$(git symbolic-ref --quiet --short refs/remotes/origin/HEAD 2>/dev/null | sed 's@^origin/@@')"
  [ -n "$b" ] && { echo "$b"; return; }
  for c in main master; do
    git show-ref --verify --quiet "refs/heads/$c" && { echo "$c"; return; }
  done
  echo main
}
current_branch() { git rev-parse --abbrev-ref HEAD; }

cmd="${1:-}"
[ -n "$cmd" ] || die "usage: sync|branch|save|pr|status ..."
in_repo

case "$cmd" in
  sync)
    def="$(default_branch)"
    git fetch --prune origin
    cur="$(current_branch)"
    if [ "$cur" = "$def" ]; then
      git merge --ff-only "origin/$def" && echo "fast-forwarded $def"
    else
      git fetch origin "$def:$def" 2>/dev/null && echo "updated $def (you are on $cur)" \
        || echo "fetched; could not ff $def while on $cur (uncommitted overlap?)"
    fi
    ;;
  branch)
    name="${2:-}"; [ -n "$name" ] || die "branch needs <name>"
    git switch -c "$name" 2>/dev/null || git checkout -b "$name"
    echo "on branch $name"
    ;;
  save)
    msg="${2:-}"; [ -n "$msg" ] || die "save needs a \"<message>\""
    cur="$(current_branch)"; def="$(default_branch)"
    [ "$cur" != "$def" ] || die "refusing to commit on default branch '$def' — run: gitflow.sh branch <name>"
    git add -A
    git diff --cached --quiet && die "nothing staged to commit"
    git commit -m "$msg"
    echo "committed on $cur"
    ;;
  pr)
    title="${2:-}"; body="${3:-}"
    [ -n "$title" ] || die "pr needs a \"<title>\""
    cur="$(current_branch)"; def="$(default_branch)"
    [ "$cur" != "$def" ] || die "you are on '$def' — branch + commit before opening a PR"
    git push -u origin "$cur"
    command -v gh >/dev/null 2>&1 || die "gh not installed; branch pushed — open the PR in the GitHub UI"
    if [ -n "$body" ]; then
      gh pr create --base "$def" --head "$cur" --title "$title" --body "$body"
    else
      gh pr create --base "$def" --head "$cur" --title "$title" --fill
    fi
    ;;
  status)
    echo "branch:   $(current_branch)  (default: $(default_branch))"
    echo "upstream: $(git rev-parse --abbrev-ref --symbolic-full-name @{u} 2>/dev/null || echo none)"
    git status --short
    ;;
  *)
    die "unknown command: $cmd (sync|branch|save|pr|status)"
    ;;
esac
