---
name: finish-issue
description: End-of-issue workflow — verifies the PR is open and properly linked to an issue, then triggers an independent code review that posts findings as inline PR comments.
argument-hint: 'optional: effort level — low | medium | high | ultra (default: high)'
allowed-tools: [Bash]
---

# finish-issue

Runs the end-of-issue checklist after implementation is done and a PR is open.

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

### Step 3: Run independent code review

Invoke the built-in code-review skill at the requested effort level (default `high`) with `--comment` so findings land as inline PR comments rather than in this session's output:

```
/code-review <effort> --comment
```

This spawns a fresh agent with no context from the current implementation session, giving an unbiased read of the diff.

### Step 4: Report

After the review completes, tell the user:
- The PR URL
- The effort level used
- That review comments (if any) are now on the PR
- That the issue is ready for their review and merge
