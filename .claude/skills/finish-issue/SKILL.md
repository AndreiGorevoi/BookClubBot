---
name: finish-issue
description: End-of-issue workflow — verifies the PR is open and linked to an issue, then runs a genuinely independent (cold-context) code review that posts findings as inline PR comments.
argument-hint: 'optional: effort level — low | medium | high (default: high)'
allowed-tools: [Bash, Agent]
---

# finish-issue

Runs the end-of-issue checklist after implementation is done and a PR is open.

The whole point of this step is an **unbiased** review. The author of a change
is the worst person to spot its blind spots, so the review MUST run in a fresh
agent that has no context from the implementation session — never review the diff
inline in the current session. (This was learned the hard way: an inline
self-review missed a real concurrency race that a cold-context agent caught.)

## Steps

### Step 1: Verify state

```bash
git branch --show-current        # must not be main
gh pr view --json number,title,body,state
```

Abort with a clear message if:
- Currently on `main` (nothing to review)
- No open PR exists for this branch (`gh pr view` fails or state != OPEN)

### Step 2: Check PR is linked to an issue

Inspect the PR body for `Closes #N`. If it's missing, warn the user — the PR should reference an issue before review. Do not abort; just flag it.

### Step 3: Run an independent, cold-context review

Spawn a **fresh `Agent`** (subagent_type `general-purpose`) to do the review. Do
NOT review the diff yourself in this session. Give the agent a self-contained
prompt — it knows nothing about the change — that tells it to:

- Gather the scope with `git -C <repo> diff main...HEAD` (fall back to `git diff HEAD~1` if needed).
- Read the changed files AND the surrounding code/contracts they depend on.
- Hunt for bugs across angles: line-by-line correctness, removed-behavior, cross-file/contract, concurrency (races, double-fire, lock held across I/O, TOCTOU), plus reuse/simplification/efficiency cleanups.
- Verify each candidate against the actual code (quote the line); drop refuted ones; keep CONFIRMED and realistic-PLAUSIBLE.
- Return ONLY a JSON array (max 10), most-severe first: `{"file","line","summary","failure_scenario"}`. Do NOT let it post to GitHub — it just returns JSON.

Scale breadth with the effort arg (default `high` = recall-biased, surface more).

### Step 4: Post findings as inline PR comments

For each finding the agent returns, post an inline comment on the PR:

```bash
gh api repos/<owner>/<repo>/pulls/<n>/comments -X POST \
  -f commit_id="<head-sha>" -f path="<file>" -F line=<line> -f side="RIGHT" \
  -f body="<summary + failure scenario>"
```

Use the PR head SHA (`gh pr view --json headRefOid -q .headRefOid`). If a finding's
line isn't in the diff, post it on the nearest changed line and say so in the body.

### Step 5: Report

Tell the user:
- The PR URL and the effort level used
- That the review ran in an independent cold-context agent
- A short summary of the findings (count + headline issues), and that they're now inline on the PR
- That the next step is to address them with `address-review`, then it's ready for their merge
