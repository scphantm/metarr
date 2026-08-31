---
name: git-commit
description: Stage and commit Metarr changes the cheap way — `git add -u` plus explicit adds for genuinely new files instead of typing out every changed path, and a commit message sized to the actual diff instead of a reflexive essay. Use whenever the user asks for a commit, instead of defaulting to `git add -A`/`git add .` or re-deriving the full session's changes from scratch.
---

# Git commit

Two token sinks show up here every time: staging (typing out every changed path by hand) and message-writing (re-deriving a summary of changes already known from doing the work). Both are avoidable without touching the safety rule they're protecting — never `git add -A` or `git add .`, since either can sweep in an untracked secrets file or binary alongside real changes.

## Staging

```bash
git status --short   # once, to see the shape of what changed
git add -u            # every already-tracked modified/deleted file, in one call
git add path/to/new_file.go another/new_file.tsx   # only genuinely new (`??`) files, named explicitly
```

`git add -u` re-stages every file git already tracks that changed or was deleted — it cannot pick up a new untracked file, so it's exactly as safe as naming paths by hand for that risk, while costing one line instead of one line per file. It's the "prefer specific files over `-A`/`.`" rule's actual intent (don't accidentally stage something new and unreviewed), not a loophole around it. Only genuinely new files — the `??` lines in `git status --short` — need naming individually, and there are usually few of them even in a large session.

Skip this split only when the whole change *is* a small, known set of files (a single-file fix, a two-file feature) — naming them directly is already cheap and skips a `git status` round-trip.

## The commit message

Don't re-read the diff to remember what happened — by the time a commit is requested, the work is already known from having just done it. Write the message from that, not from `git log`/`git diff --stat` spelunking; reach for `git diff --cached --stat` only to sanity-check the *file list* matches what was intended, not to relearn the content.

Size the message to the actual scope, the same way the repo's own history does (see `git log --oneline -5` for the range this project uses — descriptive prose, not Conventional Commits): a single fix is one or two sentences; a long multi-part session gets one short paragraph per distinct piece, not a paragraph per file touched. Padding a small change to look thorough costs tokens for no reader benefit; under-explaining a large one just moves the cost onto whoever reads `git log` later.

Still use a HEREDOC for the message body (the harness's usual mechanics), but **leave off the `Co-Authored-By: Claude` trailer** — this repo's owner asked for commits without it, overriding the harness default. Keep the `Claude-Session` line unless told to drop that too.

## Verify once, not per-step

```bash
git commit -m "$(cat <<'EOF'
...
Claude-Session: https://claude.ai/code/session_...
EOF
)" && git log --oneline -1 && git status --short
```

One combined check after the commit (last commit line + a clean-or-not status) is enough — no need for a separate `git status` before *and* after, or re-running `git diff` post-commit to confirm what's already known to have been staged correctly.

## The mistake to not repeat

Typing every changed path into `git add` by hand when `git status --short` already showed nothing untracked worth reviewing individually — that's the `-A`/`.` safety rationale paid for twice, once for the actual review and once again for the keystrokes. `git add -u` gets the same safety for free once the untracked-file check is already done.
