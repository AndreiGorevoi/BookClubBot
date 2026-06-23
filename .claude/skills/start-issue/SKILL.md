---
name: start-issue
description: Start a unit of work the project's way — create a GitHub issue, branch off fresh main as feat/|fix/<slug>, and provide the mandated PR template for when implementation is done.
argument-hint: 'a short description of the work (e.g. "persist voting state")'
allowed-tools: [Bash, AskUserQuestion]
---

# start-issue

Opens the front of the issue → branch → PR flow defined in CLAUDE.md, so the
ritual is consistent every time. (Never push directly to `main`; every change
gets its own branch and PR.)

## Steps

### Step 1: Decide title and type

From the argument, draft a concise issue title and pick the type:
- `feat` — new behavior/feature
- `fix` — bug fix

If the type is ambiguous, ask with AskUserQuestion. Derive a kebab-case `<slug>`
from the title (e.g. "Persist voting state" → `persist-voting-state`).

### Step 2: Create the issue

```bash
gh issue create --title "<title>" --body "<body>"
```

Body should state the problem/goal and a short checklist of scope. Capture the
issue number `N` from the URL it prints.

### Step 3: Branch off fresh main

```bash
git checkout main && git pull --ff-only
git checkout -b <feat|fix>/<slug>
```

Abort if `main` has uncommitted changes (warn the user first).

### Step 4: Report + hand over the PR template

Tell the user the issue number, the branch name, and that they can start
implementing. Remind them: when the work is done, open the PR with this exact
structure (first line MUST be `Closes #N` so GitHub auto-closes the issue):

```
Closes #N

## Summary
...

## Test plan
- [ ] ...

---
🤖 Created by [Claude Code](https://claude.ai/code) · Model: <active model id>
```

Fill `Model:` with the model actually in use (do not hardcode a stale id), e.g.:

```bash
gh pr create --base main --head <feat|fix>/<slug> --title "<title>" --body "$(cat <<'EOF'
Closes #N
...
EOF
)"
```

After the PR is open, the natural next steps are `finish-issue` (independent
review) and then `address-review`.
