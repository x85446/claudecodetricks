# Configuration through the binary

The current schema comes from `iterate-run conductor config show --json`. Keep the full returned `data` object, edit only requested fields, and submit it through `config validate` and revision-checked `config apply`. CLI help and the binary validate names, references, executable arguments and supported fields. Use `templates --json` to discover candidate subscriptions; a template does not establish installed/authenticated readiness.

## Project automation

Resolve a registered project's exact canonical path and ID. Resolve launch sequences inside their owning subscriptions. Each named automation definition has a stable `id` and editable `name`. Each `automation_attachments` entry has a stable `id`, `definition_id`, `project_id`, local `enabled`, and numeric `priority` (lower first). One definition attaches at most once per project. Create attachments disabled unless activation is explicitly requested. Shared edits govern future admissions; active assignments retain their saved inputs. Duplicate a definition for independent rules. Detach project references before deleting a definition. Preserve IDs for repeated requests, including entry IDs while reordering them.

| Request | Mode and fields |
|---|---|
| Keep it working | `mode: "keep_working"`; ordered `entries` |
| Run on a schedule | `mode: "scheduled"`; ordered `entries`; `window_id` |
| Follow subscription overdrive | `mode: "overdrive"`; unordered sequence IDs in `participants`; `honor_engine_overdrive: true`; `only_during_overdrive: true` |

Each normal entry is `{"id":"existing-or-new-id","sequence_id":"resolved-id","max_band":"green"}`. Green permits Green; Yellow permits Green and Yellow; Red permits all three soft bands. The first eligible entry wins, so repeated sequence IDs with different maximum bands are meaningful. Hard limits, fresh telemetry, ownership and launch gates remain mandatory. Normal modes default both overdrive flags to false. Preserve an existing explicit opt-in; set it only when requested. Only-during requires honor.

Scheduled automations select `run_limit: "drain_queue"` (default) or `"one_plan"`; and `on_window_end: "finish_checkpoint"` (default), `"finish_plan"`, or `"interrupt"`. Finish-plan applies only to the plan claimed in that occurrence and retains all other gates. Interrupt may leave unfinished edits and stops only the owned process tree. Explain that consequence when the user selects it. Queue-empty rules remain enabled.

## Named Time Windows

Create or reuse a `time_windows` entry and refer to its ID. A weekly recurrence contains `days` (Monday=1 through Sunday=7), `start`, `end` in HH:MM, IANA `timezone`, `every_weeks` (default 1), and optional `anchor_date` for stored multiweek phase; an absent anchor uses a deterministic internal epoch. It repeats indefinitely. Enabled recurrences require selected days. Overnight windows belong to their start day; equal start/end covers a full day. The editable Weekdays and Weekends defaults use `all_day: true`, displayed as 00:00–23:59 and ending at the next civil midnight, including the final minute. Other explicit end times remain exclusive.

Example: weeknights uses `days: [1,2,3,4,5]`, `start: "22:00"`, `end: "06:00"`, `timezone: "America/Chicago"`, `every_weeks: 1`, and `start_mode: "weekly"`. The subscription's trusted reset supports `start_mode: "reset_relative"` with `hours_before_reset` instead of a guessed reset date. A project's `window_id` is an additional limit intersecting its automation windows; empty means Anytime.

## AI Subscriptions and Named Launch Sequences

A subscription owns `sequences`, `presets`, `overdrive_windows`, `typical_reset`, tier metadata and exactly one `power_band_id`. The editable sequence `name` is its suffix; derive the displayed prefix from the subscription name. Preserve IDs when either name changes. Literal CLI options are arrays of arguments, including separate flag values; never concatenate shell fragments. `sequence preview ID --subscription ACCOUNT --json` exposes the same compiler used by dispatch.

Power-band definitions are shared `power_bands` entries. Universal is editable and serves as the default. Changing one affects every subscription referencing its ID; list those subscriptions and their automations. Reassignment and deletion happen in one validated transaction. Usage, resets and overdrive remain subscription-scoped even when several subscriptions share the same definition. Actual shared allowance pools also share hard limits and concurrency.

## Overdrive example

For the request “Codex Saturday midnight to 100%, Monday midnight to 100%, Thursday 16:00 to 60%; Filemaster follows those windows,” resolve the intended Codex subscription and Filemaster canonical project before editing. Percentages mean **weekly used**, not remaining.

Within that subscription, reuse matching named windows and IDs:

| Name | Weekly start | Target used | End |
|---|---|---|---|
| Before Sunday reset | Saturday 00:00 | 100 | `next_reset` |
| Before planned Tuesday reset | Monday 00:00 | 100 | `explicit`, Tuesday (`end_day: 2`) 00:00 |
| Thursday capacity target | Thursday 16:00 | 60 | `next_reset` |

Each window has `id`, `name`, `enabled`, `revision`, `start_mode: "weekly"`, `recurrence`, `end_mode`, and `target_used_percent`. Use the requested timezone, not an assumed machine timezone. `recurrence.end` is a valid clock value; `end_mode` governs the overdrive closing boundary. `typical_reset` is estimated metadata, not permission to fabricate telemetry or redeem a reset token.

The project automation contains the resolved launch-sequence ID in `participants` and follows these subscription windows without duplicating their schedule or target. Enable the subscription overdrive switch, individual windows and project automation only when requested; do not silently enable a disabled project or global conductor. Planning examples stay disabled. Existing unrelated windows survive. Repeating the request retains three windows and one automation, not six windows and two automations.

While used is below the target, active overdrive bypasses soft power bands only. At target/end/reset, overdrive-only automations wait; ordinary honoring automations return to their normal policy. Highest active unmet target wins. Consumption occurs at bounded checkpoints, so a target is neither an exact consumption guarantee nor a reserved balance.

## Read-only verification

Use `explain --project ID --json` for live decisions. Use `simulate --file SAMPLE --json` for explicit samples or unsaved automation drafts; obtain the request shape from the installed binary contract. Sample overrides never make disabled subscriptions/windows available. Compare the committed config revision and IDs with `/api/conductor/config` on the single site at localhost:8420. Report configured behavior separately from a currently gated decision.
