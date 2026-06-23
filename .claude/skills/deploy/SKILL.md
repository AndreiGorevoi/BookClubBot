---
name: deploy
description: Deploy the BookClubBot to Docker Hub by creating and pushing a semver git tag, which triggers the GitHub Actions workflow that runs tests and builds/pushes the Docker image to beskar18/book-club-bot:<tag>.
argument-hint: 'optional: patch | minor | major (default: patch)'
allowed-tools: [Bash, AskUserQuestion]
---

# Deploy BookClubBot

Deployment is triggered by pushing a semver git tag to origin. GitHub Actions then:
1. Spins up MongoDB, runs `go test ./...`
2. Builds the Docker image
3. Pushes `beskar18/book-club-bot:<tag>` to Docker Hub

## Workflow

### Step 1: Check prerequisites

```bash
# Ensure we're on main and it's clean and pushed
git status --short
git log --oneline origin/main..HEAD   # any unpushed commits?
```

If there are uncommitted changes or unpushed commits on `main`, warn the user before continuing — deploying from a dirty/ahead state means the image won't reflect what's in `main`.

### Step 2: Determine next version

```bash
git tag --sort=-version:refname | head -1
```

Parse the latest tag (format `vMAJOR.MINOR.PATCH`). Apply the bump from the skill argument (default: `patch`):
- `patch` → increment PATCH
- `minor` → increment MINOR, reset PATCH to 0
- `major` → increment MAJOR, reset MINOR and PATCH to 0

If no tags exist, start at `v0.1.0`.

Present the proposed tag to the user and confirm before proceeding. Use AskUserQuestion if the bump type was not supplied as an argument.

### Step 3: Create and push the tag

```bash
git tag <new-tag>
git push origin <new-tag>
```

Do NOT force-push tags. Do NOT tag from a branch other than `main` without explicit user confirmation.

### Step 4: Report

After pushing, tell the user:
- The tag that was created (e.g. `v0.4.7`)
- The Docker image that will be produced: `beskar18/book-club-bot:v0.4.7`
- How to watch the CI run: `gh run list --limit 3` or the GitHub Actions URL
- That building/pushing the image does NOT redeploy a running env — the target
  must pull the new tag (or be redeployed) to pick it up.
- For a **sandbox smoke test**, remind them of the prerequisites that otherwise
  silently block testing: a `telegrammApiKey`, a reachable MongoDB, the bot
  added to a group (sets `groupId`), and a **short-duration config** — prod uses
  86400s (24h) for gather/poll, so a round there takes two days; point
  `APP_ENV` at the dev config (60s) to exercise a full round in minutes.

### Step 5: Optional — watch CI

If the user wants to monitor progress:

```bash
gh run list --limit 3
gh run watch   # interactive, requires gh CLI
```
