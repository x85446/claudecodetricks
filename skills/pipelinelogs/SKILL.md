# /pipelinelogs — GitLab Pipeline Log Fetcher

Fetch all jobs from the **last 3 pipelines** from GitLab CI/CD, save them to `temp/pipelines/` with structured naming, and present a summary with failed job output.

## Overview

The `/pipelinelogs` skill automates what Bryan used to do manually: copy-pasting CI/CD job logs into `temp/`. It fetches the **3 most recent pipelines** with all their jobs, downloads each job's trace, saves them to numbered files with status in the name, and generates a `_summary.md` per pipeline plus an overall `_overview.md`.

**No picker, no prompts** — it always grabs the latest 3 pipelines automatically.

## Prerequisites

- `glab` CLI installed (`brew install glab`)
- Authenticated: `glab auth login` with a GitLab personal access token (scopes: `api`, `read_api`, `read_repository`)
- Verify with: `glab auth status`

## Trigger

```
/pipelinelogs          # Fetches all jobs from the latest 3 pipelines (always)
```

User-invocable. Also triggered by keywords: pipeline, ci, build logs, pipeline logs.

---

## Constants

```
GITLAB_PROJECT = gravhl/gravhl-mobile-rn
OUTPUT_DIR = temp/pipelines/
TARGET_PIPELINE_COUNT = 3
```

### Job Order Map

The project pipeline has these jobs in order. Use this map for numbered filenames:

| Order | Job Name      | Stage  |
|-------|---------------|--------|
| 01    | lint          | lint   |
| 02    | test          | test   |
| 03    | build-android | build  |
| 04    | build-ios     | build  |
| 05    | notify_slack  | notify |

If a job appears that is NOT in this map, assign it the next available number (06, 07, ...) and log a note in the summary.

---

## Phase 1: Fetch 3 Pipelines

**Goal:** Fetch the 3 most recent pipelines and all their jobs. No picker — fully automatic.

### Steps

1. **Check glab auth**:
   ```bash
   glab auth status
   ```
   If not authenticated, stop and tell the user:
   ```
   glab is not authenticated. Run: glab auth login
   Token scopes needed: api, read_api, read_repository
   Create token at: https://gitlab.com/-/user_settings/personal_access_tokens
   ```

2. **Fetch the 3 most recent pipelines as JSON**:
   ```bash
   glab api "projects/gravhl%2Fgravhl-mobile-rn/pipelines?per_page=3"
   ```

3. **For each pipeline**, fetch ALL its jobs:
   ```bash
   glab api "projects/gravhl%2Fgravhl-mobile-rn/pipelines/<pipeline_id>/jobs?per_page=100"
   ```

4. **Tell the user** what was found:
   ```
   Fetching logs for 8 jobs across 3 pipelines:
     #75 (dev, passed): lint, test, build-android, build-ios, notify_slack
     #74 (mr8, failed): build-android, build-ios
     #73 (dev, passed): lint
   ```

---

## Phase 2: Fetch Job Traces & Save (per pipeline)

**Goal:** Download each job's log trace and save to structured files.

### Steps

1. **Create output directory per pipeline**:
   ```
   temp/pipelines/{pipelineIID}_{branch}_{shortSHA}_{status}/
   ```
   - `{pipelineIID}` = pipeline IID (e.g., `73`) — for sort order
   - `{branch}` = pipeline ref (e.g., `dev`, `mr8` for merge request refs)
   - `{shortSHA}` = first 7 chars of commit SHA
   - `{status}` = `failed`, `passed`, etc.
   - For MR refs like `refs/merge-requests/8/head`, shorten to `mr8`
   - Example: `temp/pipelines/73_mr8_e8a6c58_failed/`
   - Use `mkdir -p` to create it

2. **For each job**, fetch its log trace:
   ```bash
   glab api projects/gravhl%2Fgravhl-mobile-rn/jobs/<job_id>/trace 2>/dev/null
   ```
   Note: `glab ci trace` may prompt interactively. Use the API endpoint directly for non-interactive fetching.

   **If a job was skipped**, it may have no trace. Write a note: "Job was skipped — no log output."

3. **Save each job log** to a numbered file:
   ```
   {order}_{jobname}_{status}.md
   ```
   - Replace underscores in job name with hyphens for consistency
   - Example filenames:
     - `01_lint_passed.md`
     - `02_test_passed.md`
     - `03_build-android_passed.md`
     - `04_build-ios_failed.md`
     - `05_notify-slack_skipped.md`

4. **Each `.md` file format**:
   ```markdown
   # Job: {job_name}

   - **Status:** {status}
   - **Stage:** {stage}
   - **Duration:** {duration formatted as Xm Ys}
   - **Job ID:** {job_id}
   - **Started:** {started_at}
   - **Finished:** {finished_at}

   ---

   ## Log Output

   ```
   {full log trace output}
   ```
   ```

---

## Phase 3: Generate Summaries

**Goal:** Create a `_summary.md` per pipeline directory, plus a top-level `_overview.md`.

### Per-Pipeline `_summary.md`

Create `_summary.md` inside each pipeline's directory:

```markdown
# Pipeline #{pipeline_iid} — {branch} @ {shortSHA}

**Date:** {YYYY-MM-DD}
**Branch:** {branch}
**Commit:** {shortSHA} — "{commit_message}"
**Status:** {overall_status}
**Duration:** {total_duration formatted as Xm Ys}

## Jobs

| # | Job | Status | Duration |
|---|-----|--------|----------|
| 01 | lint | passed | 1m 12s |
| 02 | test | passed | 3m 45s |
| 03 | build-android | passed | 14m 23s |
| 04 | build-ios | failed | 25m 0s |
| 05 | notify-slack | skipped | 0s |

## Failed Jobs

### build-ios (04_build-ios_failed.md)

Last 50 lines of output:
\`\`\`
... error output here ...
\`\`\`
```

- For each **failed** job, include the **last 50 lines** of its log output.
- If **no jobs failed**, replace "Failed Jobs" section with: `## Result\n\nAll jobs passed successfully.`

### Top-Level `_overview.md`

Create `temp/pipelines/_overview.md` summarizing the fetch:

```markdown
# Pipeline Overview — {date}

Fetched all jobs from last 3 pipelines for gravhl/gravhl-mobile-rn.

| # | Pipeline | Branch | SHA | Status | Jobs | Duration | Failed Job |
|---|----------|--------|-----|--------|------|----------|------------|
| 1 | #75 | dev | a1b2c3d | passed | 5 | 24m 19s | — |
| 2 | #74 | mr8 | e8a6c58 | failed | 2 | 39m 23s | build-ios |
| 3 | #73 | dev | f4e5d6c | passed | 1 | 1m 12s | — |

## Directories

- `75_dev_a1b2c3d_passed/`
- `74_mr8_e8a6c58_failed/`
- `73_dev_f4e5d6c_passed/`
```

---

## Phase 4: Present to User

**Goal:** Show results inline and point to saved files.

### Steps

1. **Print the overview table** inline (from `_overview.md`)

2. **For failed jobs**, show a brief excerpt of each failure

3. **List all directories created**:
   ```
   Pipeline logs saved to temp/pipelines/:

     _overview.md
     75_dev_a1b2c3d_passed/
     74_mr8_e8a6c58_failed/
     73_dev_f4e5d6c_passed/
   ```

4. **Offer next steps** if there were failures:
   ```
   Failed job logs are ready for analysis. Would you like me to:
   - Analyze a specific failure?
   - Compare failed vs passed pipeline runs?
   ```

---

## Tool Usage Priority

| Tool | Purpose |
|------|---------|
| **Bash** | `glab` CLI commands (api calls for pipelines, jobs, traces) |
| **Write** | Save log files, summaries, and overview to `temp/pipelines/` |

No `AskUserQuestion` needed — skill runs fully automatically.

---

## Edge Cases

| Scenario | Behavior |
|----------|----------|
| `glab` not installed | Stop: "Install glab: `brew install glab`" |
| `glab` not authenticated | Stop: show auth instructions |
| Fewer than 3 pipelines exist | Fetch however many exist |
| Job has no trace (skipped/pending) | Write note: "Job was skipped — no log output." |
| Unknown job name (not in order map) | Assign next number (06+), note in summary |
| Network error during trace fetch | Note which job failed to fetch, continue with others |
| Pipeline is still running | Fetch what's available, note running jobs |
| Very large log output (>1MB) | Save full log to file, but only show last 50 lines in summary |
| Directory already exists (re-run) | Overwrite existing files |

---

## Example Run

```
User: /pipelinelogs

Phase 1: Fetch 3 Pipelines
  Checking glab auth... authenticated as brmcc
  Pipeline #75 (dev, passed): 5 jobs
  Pipeline #74 (mr8, failed): 2 jobs
  Pipeline #73 (dev, passed): 1 job

  Fetching logs for 8 jobs across 3 pipelines:
    #75: lint, test, build-android, build-ios, notify_slack
    #74: build-android, build-ios
    #73: lint

Phase 2: Fetch Traces & Save
  [Processing #75...] 5 traces saved
  [Processing #74...] 2 traces saved
  [Processing #73...] 1 trace saved

Phase 3: Generate Summaries
  Generated _summary.md for #75, #74, and #73
  Generated _overview.md

Phase 4: Present
  | # | Pipeline | Branch | SHA | Status | Jobs | Failed Job |
  |---|----------|--------|-----|--------|------|------------|
  | 1 | #75 | dev | a1b2c3d | passed | 5 | — |
  | 2 | #74 | mr8 | e8a6c58 | failed | 2 | build-ios |
  | 3 | #73 | dev | f4e5d6c | passed | 1 | — |

  Pipeline logs saved to temp/pipelines/
```
