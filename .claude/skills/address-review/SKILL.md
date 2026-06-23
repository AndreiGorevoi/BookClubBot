---
name: address-review
description: After a review, triage the open inline PR comments, apply fixes, push, and reply to every comment with the fix commit SHA (or a reject reason); offer to roll deferred findings into a follow-up issue.
argument-hint: 'optional PR number (default: the PR for the current branch)'
allowed-tools: [Bash, Agent]
---

# address-review

Closes the loop on a reviewed PR. Every inline comment must end with a reply —
either the fix or an explicit reason it was skipped (CLAUDE.md workflow step 5).

## Steps

### Step 1: Gather the open inline comments

```bash
PR=<n>   # or: gh pr view --json number -q .number
gh api repos/<owner>/<repo>/pulls/$PR/comments --paginate \
  -q '.[] | "id:\(.id) path:\(.path) line:\(.line // .original_line)\n\(.body)\n"'
```

Include comments from any reviewer (human, this project's review agent, external
bots like Codex). Skip ones that already have your reply.

### Step 2: Triage each finding

For each, decide and note the disposition:
- **Fix** — a real bug/cleanup in scope for this PR.
- **Defer** — real but belongs in a follow-up (out of this PR's scope).
- **Reject** — not a real problem (factually wrong, already handled, or pure
  style). Be willing to reject with a concrete reason; reviewers (including
  automated ones) lack full context and can be wrong.

When unsure whether a flagged bug is real, you may spawn an `Agent` to verify it
against the code before deciding.

### Step 3: Apply the fixes

Make the code changes for the "Fix" items. Then, from the repo root:

```bash
gofmt -w <changed-files>
go build ./... && go vet ./... && go test -short ./...
```

Commit with a message that summarizes what the review changed, and push.

### Step 4: Reply to every comment

For each comment, post a reply with the fix commit SHA and a one-line summary, or
the reason it was deferred/rejected:

```bash
gh api repos/<owner>/<repo>/pulls/$PR/comments/<comment-id>/replies -X POST \
  -f body="Fixed in <sha>: <what changed>."     # or "Deferred to #<m>: ..." / "Not a bug: <why>."
```

### Step 5: Deferred findings → follow-up issue

If anything was deferred, offer to capture it: append to an existing follow-up
issue or open a new one (`gh issue create`), and reference that issue number in
the corresponding replies.

### Step 6: Confirm CI and report

Wait for CI on the new commit, then report: what was fixed / deferred / rejected,
the follow-up issue (if any), CI status, and that the PR is ready for merge.
