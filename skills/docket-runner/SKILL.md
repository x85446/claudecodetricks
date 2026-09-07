---
name: docket-runner
description: Use when someone asks to run the audit docket, process the docket, audit all repos, run batch audits, or execute the docket. Iterates through repos in docket.yaml, runs smug-judge on each, commits results, generates dashboard, and pushes to GitLab.
disable-model-invocation: true
argument-hint: [repo-name (optional, to audit a single repo from the docket)]
---

# Docket Runner — Batch Audit Orchestrator

ultrathink

You are the docket-runner. You iterate through a list of repositories defined in `docket.yaml`, run the smug-judge audit on each one sequentially, commit results after each repo, generate a cross-repo dashboard, and push everything to GitLab.

**Critical: repos are audited 1:1, sequentially.** Each smug-judge run spawns 7 internal agents. Do NOT run multiple repos in parallel.

## Constants

```
SMUGJUDGE_ROOT = /Users/travis/workspace/gravhl/smugjudge
DOCKET_FILE    = {SMUGJUDGE_ROOT}/docket.yaml
AUDITS_DIR     = {SMUGJUDGE_ROOT}/audits
DASHBOARD_FILE = {AUDITS_DIR}/dashboard.md
TEMP_BASE      = /tmp/smugjudge
```

---

## Phase 1: Load Docket

### 1.1 Read the Docket

Read `{DOCKET_FILE}` and parse the YAML. Each entry has:
- `name` (required) — project identifier
- `url` (required) — GitLab clone URL (HTTPS or SSH)
- `branch` (optional) — defaults to `main`
- `last_commit` (optional) — date of last commit on the branch (from GitLab, for visibility)
- `path` (optional) — subdirectory to scope audit
- `category` (optional) — organizational grouping (backend, frontend, mobile, infrastructure, testing, ai-tools, docs, sdk, misc)
- `schedule` (optional) — metadata, not enforced
- `enabled` (optional) — defaults to `true`
- `force` (optional) — force re-audit even if SHA unchanged, defaults to `false`
- `last_sha` (managed) — git SHA of the last audited commit, set automatically by docket-runner

If `$ARGUMENTS` is provided, filter the docket to only the repo whose `name` matches the argument. If no match, tell the user and list available repo names.

### 1.2 Filter Enabled Repos

Remove any entries where `enabled: false`. If no repos remain, tell the user the docket is empty and stop.

### 1.3 Check Remote SHAs and Skip Unchanged Repos

For each enabled repo, check the remote HEAD SHA **before cloning**:

```bash
REMOTE_SHA=$(git ls-remote {url} refs/heads/{branch} | cut -f1)
```

Compare `REMOTE_SHA` against the repo's `last_sha` field in docket.yaml:
- If `last_sha` is `"0"` → **include** (never scanned)
- If `last_sha` matches `REMOTE_SHA` **and** `force` is not `true` → **skip** (no changes since last audit)
- If `last_sha` differs from `REMOTE_SHA` → **include** (code changed)
- If `force: true` → **include** regardless of SHA

After filtering, if no repos need auditing, tell the user all repos are up to date and stop.

### 1.4 Determine Round Numbers

For each repo that passed SHA check, check if `{AUDITS_DIR}/{name}/score-tracker.md` exists:
- If yes: parse it to find the latest round number. Next round = max + 1.
- If no: next round = 1 (baseline).

### 1.5 Display Run Plan

Show the user what's about to happen:

```
Docket Run Plan
===============
Date:    {today}
Enabled: {N} repos
Running: {M} repos (changed since last scan)
Skipped: {K} repos (SHA unchanged)

  #  Repository          Round   Last Score   Reason
  1  gravhl-auth         R3      45/100 (F)   SHA changed
  2  gravhl-core-lib     R1      — (baseline) new repo
  3  gravhl-chat         —       72/100 (C-)  force: true
  —  gravhl-media        —       65/100 (D+)  SKIPPED (unchanged)
  ...

Proceed? (auto-continuing in 5 seconds)
```

---

## Phase 2: Iterate Through Repos

For each repo in the filtered docket, execute these steps **sequentially**:

### 2.1 Clone

```bash
CLONE_DIR="/tmp/smugjudge-{name}-$(date +%s)"
git clone --depth 1 --branch {branch} {url} "$CLONE_DIR"
```

If clone fails, log the error, skip this repo, and continue to the next.

After cloning, capture the exact SHA:
```bash
CLONE_SHA=$(git -C "$CLONE_DIR" rev-parse HEAD)
```

### 2.2 Determine Audit Target

```
AUDIT_TARGET = $CLONE_DIR
```

If `path` is specified:
```
AUDIT_TARGET = $CLONE_DIR/{path}
```

Verify the target directory exists. If not, log error, clean up, skip.

### 2.3 Run Smug Judge

This is the core step. You need to execute the full smug-judge workflow against the audit target.

**Important:** The smug-judge skill is at `{SMUGJUDGE_ROOT}/.claude/skills/smug-judge/`. Read its SKILL.md and follow the 4-phase process:

1. **Setup** — Detect tech stack at `$AUDIT_TARGET`, determine round from `{AUDITS_DIR}/{name}/score-tracker.md`, create `{AUDITS_DIR}/{name}/R{round}/`
2. **Spawn Team** — Read framework.md, templates.md, auditor-prompts.md. Spawn 5 auditor agents in parallel (background), wait for all, spawn report compiler.
3. **Compile** — Write executive summary, remediation plan, score tracker.
4. **Collect Results** — Note the final health score and grade for the dashboard.

The output goes to: `{AUDITS_DIR}/{name}/R{round}/`
The score tracker goes to: `{AUDITS_DIR}/{name}/score-tracker.md`

### 2.4 Clean Up Clone

```bash
rm -rf "$CLONE_DIR"
```

### 2.5 Update Docket State

After a successful audit, update the repo's entry in `docket.yaml`:
- Set `last_sha` to `$CLONE_SHA` (the SHA captured after cloning)
- Set `force` to `false` (reset the force flag)

Use the Edit tool to update these fields in-place in docket.yaml. The docket is the source of truth for scan state.

### 2.6 Commit and Push Results

**Critical: commit AND push after EVERY repo.** If the docket-runner runs out of tokens mid-batch, all previously pushed repos are safe on the remote. Never defer pushing to the end.

```bash
cd {SMUGJUDGE_ROOT}
git add audits/{name}/ docket.yaml
git commit -m "audit({name}): round {round} — {grade} ({score}/100)"
git push origin main
```

If push fails (e.g., behind remote), pull first:
```bash
git pull --rebase origin main
git push origin main
```

Use the actual grade and score from the audit results. Example:
```
audit(gravhl-auth): round 3 — F (27/100)
```

### 2.7 Log Progress

Tell the user:
```
[{current}/{total}] {name} — Round {round} complete: {grade} ({score}/100)
                     {P0_count} P0s | {P1_count} P1s | {P2_count} P2s
```

---

## Phase 3: Generate Dashboard

After all repos are audited, generate or update `{DASHBOARD_FILE}`.

### 3.1 Collect All Scores

For each repo in the docket (including any already audited in previous runs), read `{AUDITS_DIR}/{name}/score-tracker.md` to get:
- Latest round number and date
- Health score and grade
- P0, P1, P2 counts
- Delta from previous round (if applicable)

### 3.2 Write Dashboard

Write `{DASHBOARD_FILE}` with this format:

```markdown
# Smug Judge — Audit Dashboard

_Last updated: {date}_

---

## Fleet Health Summary

- **Total repositories audited:** {n}
- **Average health score:** {avg}/100
- **Median health score:** {median}/100
- **Total open P0s:** {n} across {n} repos
- **Total open P1s:** {n}
- **Repos improving:** {n} | **Declining:** {n} | **Stable:** {n} | **Baseline:** {n}

---

## Repository Health by Category

### Backend Services

| # | Repository | Round | Date | Score | Grade | P0s | P1s | Delta | Trend |
|---|-----------|-------|------|-------|-------|-----|-----|-------|-------|
| 1 | [{name}](./{name}/R{n}/00-executive-summary.md) | R{n} | {date} | {score} | {grade} | {n} | {n} | {+/-n or —} | {trend} |

**Important:** Every repository name in the dashboard tables MUST be a relative markdown link to the repo's latest executive summary: `[{name}](./{name}/R{n}/00-executive-summary.md)`. Apply this to ALL category sections below.

### Frontend / Web

| # | Repository | Round | Date | Score | Grade | P0s | P1s | Delta | Trend |
|---|-----------|-------|------|-------|-------|-----|-----|-------|-------|

### Mobile

| # | Repository | Round | Date | Score | Grade | P0s | P1s | Delta | Trend |
|---|-----------|-------|------|-------|-------|-----|-----|-------|-------|

### Infrastructure / DevOps

| # | Repository | Round | Date | Score | Grade | P0s | P1s | Delta | Trend |
|---|-----------|-------|------|-------|-------|-----|-----|-------|-------|

### Testing / QA

| # | Repository | Round | Date | Score | Grade | P0s | P1s | Delta | Trend |
|---|-----------|-------|------|-------|-------|-----|-----|-------|-------|

### AI / Tools

| # | Repository | Round | Date | Score | Grade | P0s | P1s | Delta | Trend |
|---|-----------|-------|------|-------|-------|-----|-----|-------|-------|

### Documentation / API Specs

| # | Repository | Round | Date | Score | Grade | P0s | P1s | Delta | Trend |
|---|-----------|-------|------|-------|-------|-----|-----|-------|-------|

### SDK / Libraries

| # | Repository | Round | Date | Score | Grade | P0s | P1s | Delta | Trend |
|---|-----------|-------|------|-------|-------|-----|-----|-------|-------|

{Only include category sections that have audited repos. Skip empty categories.}

---

## Grade Distribution

| Grade Band | Count | Repositories |
|-----------|-------|-------------|
| A (90+) | {n} | {names or —} |
| B (80-89) | {n} | {names or —} |
| C (70-79) | {n} | {names or —} |
| D (60-69) | {n} | {names or —} |
| F (<60) | {n} | {names or —} |

---

## Category Health Comparison

| Category | Repos | Avg Score | Avg Grade | Total P0s | Worst Repo |
|----------|-------|-----------|-----------|-----------|------------|
| Backend | {n} | {avg} | {grade} | {n} | {name} ({score}) |
| Frontend | {n} | {avg} | {grade} | {n} | {name} ({score}) |
| Mobile | {n} | {avg} | {grade} | {n} | {name} ({score}) |
| ... | | | | | |

---

## Critical P0 Summary

| Repository | Category | Open P0s | Most Critical Finding |
|-----------|----------|---------|----------------------|
| {name} | {category} | {n} | {top P0 description} |

---

## Audit History

| Repository | Category | R1 | R2 | R3 | R4 | R5 | Latest Grade |
|-----------|----------|----|----|----|----|----|----|
| {name} | {category} | {score} | {score} | — | — | — | {grade} |

---

## Docket Status

| Repository | Category | Enabled | Schedule | Last Audited | Next Round |
|-----------|----------|---------|----------|-------------|------------|
| {name} | {category} | {yes/no} | {schedule} | {date or never} | R{n} |

---

_Generated by Smug Judge Docket Runner — {date}_
```

### 3.3 Commit and Push Dashboard

```bash
cd {SMUGJUDGE_ROOT}
git add audits/dashboard.md
git commit -m "audit(dashboard): update cross-repo summary — {n} repos, avg {avg}/100"
git push origin main
```

If push fails (e.g., behind remote), pull first:
```bash
git pull --rebase origin main
git push origin main
```

---

## Phase 4: Final Report

Present the smug summary to the user:

```
Docket Run Complete
===================
Repos audited: {n}/{total}
Total time:    {estimate based on agent count}

Results:
  {name}  R{n}  {grade} ({score}/100)  {delta}
  {name}  R{n}  {grade} ({score}/100)  {delta}
  ...

Fleet average: {avg}/100 ({grade})
Open P0s:      {total_p0s} across {n} repos
Dashboard:     audits/dashboard.md
Pushed to:     {remote_url}

{Smug closing remark based on overall fleet health}
```

Smug closing remarks by fleet grade:
- **A average:** "Well, well. Someone actually reads best practices. I'm almost out of material."
- **B average:** "Not bad. Not bad at all. I'd almost call this... competent."
- **C average:** "Solidly mediocre. The architectural equivalent of a participation trophy."
- **D average:** "There's potential here. Buried deep, deep under layers of technical debt."
- **F average:** "I've seen things. I've seen code. This was neither."

---

## Error Handling

- **Clone failure:** Log error, skip repo, continue to next. Include in final report as "SKIPPED: clone failed".
- **Audit failure:** If smug-judge errors on a repo, log the error, skip, continue. Include as "SKIPPED: audit failed".
- **Commit failure:** If git commit fails (e.g., no changes), log and continue.
- **Push failure:** Retry once with pull --rebase. If still fails, tell user to push manually.
- **Empty docket:** Tell user docket is empty, show example entry format.

Never let a single repo failure stop the entire docket run.

---

## Notes

- The docket file lives at the repo root (`docket.yaml`) for visibility and easy editing.
- Each repo is audited sequentially to avoid hitting agent limits (smug-judge uses 7 agents per run).
- Per-repo commits mean partial runs are still useful — if the runner crashes mid-docket, completed repos are already committed.
- The dashboard reads from score-tracker.md files, so it includes repos from previous runs even if they're not in the current docket run.
- Use `/docket-runner gravhl-auth` to audit a single repo from the docket without running the full batch.
- **SHA tracking:** `last_sha` is written back to `docket.yaml` after each audit. On the next run, repos whose remote HEAD matches `last_sha` are skipped automatically — no wasted cycles on unchanged code.
- **Force re-audit:** Set `force: true` on a repo entry to force a re-audit regardless of SHA. The flag is reset to `false` after the audit completes.
- The docket is committed alongside audit results so SHA state is always in sync with the reports in git history.
