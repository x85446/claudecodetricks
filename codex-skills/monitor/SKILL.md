---
name: monitor
description: Use when someone asks to monitor a fix, watch a deployment, check on something every few minutes, keep an eye on CI, or invokes $monitor.
---


# monitor

A context-aware monitoring loop. The user says `$monitor 5` after a fix/merge/deploy; the skill reads the conversation, figures out what's being waited on, proposes a plan, and loops in the background until the thing is verified done (or stuck).

## Usage

Argument: <minutes> [what to monitor]. `$1` is its first word; `$ARGUMENTS` is the whole thing.

<!-- codex-port: `argument-hint` has no Codex frontmatter home; folded into this Usage section. Argument substitution is documented for Codex custom prompts but not for skills, so the meaning is stated in prose rather than left to the token alone. -->

Auto-fixes aggressively during the loop. Stays silent between cycles. Notifies only at the final state: success, stuck, or cycle-cap hit.

## Arguments

- `<minutes>` (required) — cadence between checks, e.g. `5`
- `[what to monitor]` (optional) — free-text description to focus the skill, e.g. `"the production deploy"`. If omitted, infer from recent conversation.
- `--continue` (internal) — passed by whatever re-invokes this skill on the next cycle; the user never types it

If `$ARGUMENTS` is empty or non-numeric, print usage (`$monitor <minutes> [what]`) and exit.

## State File

`.claude/monitor-state.md` holds the loop's working state so each wake-up can pick up where the last left off:

```markdown
# monitor state
started: 2026-04-21T10:30:00Z
cadence_minutes: 5
max_cycles: 10
current_cycle: 3
forge: github
description: fix production deploy for auth service
check_method: gh run list --branch main --limit 1 --json status,conclusion
last_status: failing (null_pointer in auth.go:142)
fix_attempts:
  - cycle 1: re-ran failed job (fix: transient)
  - cycle 2: reverted c3d4e5f, force-pushed
original_context: |
  User fixed null pointer in auth service. PR #482 merged.
  Waiting on CI + production deploy to complete.
  Success criteria: main branch CI green + deploy webhook confirms.
```

Create on first run. Read on every wake. Delete on final state (success, stuck, cap).

## Workflow

### Mode A: First invocation (no state file, no `--continue`)

#### Step 1: Parse and validate

Extract `$1` as minutes. Reject if not an integer >0.
Extract `$2+` as optional description.

#### Step 2: Read recent context

Look at the last ~20 turns of conversation (or the whole session if short). Determine:

- **What just happened** — the bug/feature/deploy that's in flight
- **What we're waiting on** — CI, deploy webhook, PR merge, test run, external API
- **How to check it** — the concrete command/URL/tool that reveals the current state
- **Success criteria** — what "done" looks like
- **Known fix patterns** — things the conversation already tried or mentioned

If the conversation is too ambiguous, ask one clarifying question before proposing. Don't guess wildly.

#### Step 3: Propose the plan

Output a single message using this exact shape (adapt content to the situation):

```
I noticed we were <≤50 words on the issue>.
I attempted to fix by <≤50 words on the fix>.
Deployment status: <≤50 words on what's deployed + where>.

Check method: <command or URL I'll use each cycle>
Success criteria: <what "done" looks like>
Cadence: every <N> minutes, max 10 cycles (<N*10> min cap)

Would you like me to make sure this gets completely fixed and deployed?
I'll fix any issues that come up and notify you when it's in a fixed deployed state.
```

Wait for user confirmation (yes/no/modify). On no, exit without starting the loop. On modify, accept their corrections and re-propose.

#### Step 4: Write state file

On yes, create `.claude/monitor-state.md` with cycle=0 and the agreed plan. Then fall through to Step 5.

### Mode B: Wake-up (state file exists or `--continue` was passed)

Skip Steps 1-4. Go straight to Step 5.

#### Step 5: Run one cycle

1. Read `.claude/monitor-state.md`. Increment `current_cycle`.
2. Run the check method. Capture output.
3. Classify the result:
   - **Succeeded** — success criteria met → go to Step 6 (final: success)
   - **Failed** — a concrete failure detected → attempt aggressive fix, then loop on
   - **In progress** — still running, nothing actionable → loop on
   - **Unknown** — check produced ambiguous output → loop on, note in state file

4. **If failed, fix aggressively (user chose "Aggressive fix"):**
   - Try the most likely fix based on failure signal (re-run a flaky job, revert a bad commit, push a one-line fix for a known pattern, retry a deploy, bump a dependency)
   - Record the attempt in `fix_attempts` in state file
   - Don't wait for user approval — that's the point of aggressive mode
   - Don't try the same fix twice in a row if it didn't work; try something different

5. **If cycle >= max_cycles (10):** go to Step 6 (final: cap hit)

6. Otherwise, update the state file and hand the next cycle back to the user.

   <!-- codex-port: Claude Code self-schedules with ScheduleWakeup. Codex has no
        in-session scheduling tool, so this skill cannot wake itself. Rewritten
        to a user-created trigger; see references/codex-format.md. -->

   **Codex cannot schedule its own next cycle** — there is no `ScheduleWakeup`
   equivalent, and inventing a call wastes the turn. On the FIRST cycle only,
   print one line telling the user how to make it recur:

   ```
   monitoring every <N>m — Codex cannot wake itself. To keep cycles running:
     ChatGPT desktop/web → Scheduled → Automation firing "$monitor --continue"
     every <N> minutes, or an OS cron job running `codex exec` with that prompt.
   ```

   Then end the cycle. The state file holds everything, so the next invocation —
   scheduled or typed by hand — resumes at cycle N+1 exactly as before.

Do not output anything to the user in this step unless Step 6 is triggered.

### Step 6: Final notification

When exiting the loop (success, stuck, or cap), print a clear summary and delete `.claude/monitor-state.md`:

```
Monitor complete: <SUCCESS | STUCK | CAP HIT> (<N> cycles, <total minutes>)

Issue: <original description, 1 line>
Final state: <one sentence>
Fixes applied:
  - cycle 1: <what>
  - cycle 3: <what>
Check output at exit:
  <last few lines of check output>
```

Delete state file. Do not schedule another wake.

## Host Detection (GitHub vs GitLab)

Before running any PR/CI commands, detect which forge is in use. The repo might live on GitHub (use `gh`) or GitLab (use `glab`). Do this once at the start of Mode A, record the result in state, and reuse it on every wake.

Detection order:

1. Check remote: `git remote get-url origin 2>/dev/null` — if it contains `gitlab`, use `glab`; if it contains `github`, use `gh`.
2. Fall back to CLI availability: try `command -v glab` first on GitLab-looking remotes, `command -v gh` first on GitHub-looking remotes.
3. If neither CLI is installed, degrade gracefully — skip PR/CI context, ask the user for the check command.

Store the detected forge in state as `forge: github` or `forge: gitlab`. Every command below is shown in both forms — pick the matching one.

### Command equivalents

| Purpose | GitHub (`gh`) | GitLab (`glab`) |
|---|---|---|
| List own PRs/MRs | `gh pr list --author @me --state all --limit 5` | `glab mr list --mine --all --per-page 5` |
| List CI runs | `gh run list --limit 5 --json name,status,conclusion,headBranch` | `glab ci list --per-page 5` |
| Latest run on branch | `gh run list --branch <branch> --limit 1 --json status,conclusion` | `glab ci list --branch <branch> --per-page 1` |
| View a run | `gh run view <run_id> --json status,conclusion` | `glab ci view <pipeline_id>` |
| Re-run failed jobs | `gh run rerun <id> --failed` | `glab ci retry <pipeline_id>` |

When the skill text below references a `gh` command, substitute the `glab` equivalent if `forge: gitlab`.

## Dynamic Context Injection

On the first invocation, gather context via preprocessing. Each command auto-detects the forge and silently no-ops if the required CLI isn't installed. Every line ends with `|| true` so the preprocessing never aborts on a missing tool:

```
!`git log --oneline -20 2>/dev/null | head -20 || true`
!`if command -v gh >/dev/null 2>&1; then gh pr list --author @me --state all --limit 5 2>/dev/null; elif command -v glab >/dev/null 2>&1; then glab mr list --mine --all --per-page 5 2>/dev/null; fi || true`
!`if command -v gh >/dev/null 2>&1; then gh run list --limit 5 --json name,status,conclusion,headBranch 2>/dev/null; elif command -v glab >/dev/null 2>&1; then glab ci list --per-page 5 2>/dev/null; fi || true`
```

These give a quick read on recent git activity, PRs/MRs, and CI status without the model having to ask. They inform the "what just happened" inference. Absence of output is itself a signal — no forge CLI available means the skill should ask the user for the check command instead of guessing.

## Check Method Inference

Match the situation to the right check. Use the forge-appropriate command.

| Situation signal | GitHub | GitLab |
|---|---|---|
| PR/MR merged, CI running | `gh run list --branch <branch> --limit 1 --json status,conclusion` | `glab ci list --branch <branch> --per-page 1` |
| Production deploy triggered | `curl -sf <health_url>` + `gh run list` | `curl -sf <health_url>` + `glab ci list` |
| Long-running test suite | `gh run view <run_id> --json status,conclusion` | `glab ci view <pipeline_id>` |
| Local background job | `ps -p <pid>` or check output file | same |
| External webhook/API | `curl -s <url>` | same |
| Nothing automated | Ask user for the check command | same |

## Aggressive Fix Patterns

Apply these without asking. Record each attempt in state.

| Failure signal | Fix to try |
|---|---|
| Flaky test in logs | Re-run the failing job: `gh run rerun <id> --failed` (GitHub) or `glab ci retry <pipeline_id>` (GitLab) |
| "timeout" / "rate limit" | Wait one extra cycle then retry |
| Deploy webhook 5xx | Re-trigger deploy |
| Compile error, simple typo | Fix and push |
| Dependency install fail | Bump lockfile, push |
| Test expects stale fixture | Update fixture, push |
| Merge conflict blocking deploy | Rebase onto main, push |

Never try a fix twice in a row. If fix N didn't clear it, try fix N+1 next cycle.

If no reasonable fix is available (the failure is novel, the root cause is unclear, or it requires a design decision), treat it as **stuck** and trigger Step 6 with status `STUCK` instead of looping further. Don't burn cycles on a failure you can't fix.

## Cadence Guidance (advisory, not enforced)

- `<minutes>` < 5: stays within Anthropic prompt cache TTL, cheap
- 5-20 minutes: cache misses each cycle, more expensive
- 20+ minutes: amortize the cache miss, also cheap
- The "worst of both" zone is 5-7 minutes. If user picks 5, respect it — but if they say "every few minutes, whatever works," suggest 4 or 20.

## Guardrails

- **Never run $monitor without explicit user confirmation** on first invocation. `--continue` only ever arrives from the recurring trigger the user set up, never from a first invocation.
- **Never push to main or force-push without user approval** even in aggressive-fix mode. Limit pushes to feature branches.
- **Never auto-rollback production** — deploy reverts need human signoff. "Revert a commit and push to a feature branch" is fine; "rollback prod" is not.
- **Respect the cap.** 10 cycles is the hard ceiling. Don't extend it silently.
- **If the conversation context is thin** (fresh session, no recent activity), don't guess. Ask one clarifying question before proposing.
- **State file is authoritative** during a loop. If it's malformed, delete it and start Mode A again — don't try to recover mid-loop.
- **One monitor at a time per project.** If `.claude/monitor-state.md` exists when user invokes `$monitor <N>` (not `--continue`), warn them and ask whether to cancel the existing one.

## What NOT to Do

- Don't post status updates between cycles — user chose "final state only"
- Don't ask for approval on fixes — user chose "aggressive fix"
- Don't exceed 10 cycles even if it seems close to passing
- Don't infer deploy success from "CI green" alone if the conversation indicates a separate deploy step
- Don't `rm` the state file until the final notification goes out
- On reaching a final state, say so plainly and tell the user to pause the Automation they created — this skill cannot cancel a trigger it never armed
