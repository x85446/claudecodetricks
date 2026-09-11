---
name: accounts
description: "**Always invoke for any connectivity or account-access question.** Use whenever the user asks about reaching a machine, logging into a service, getting credentials, or any flavor of \"do I have access to X\" — SSH hosts, machines, GitHub/GitLab orgs and repos, API tokens, cloud accounts (AWS/GCP/Cloudflare), URLs/dashboards. Trigger phrases include \"can I reach X\", \"how do I connect to Y\", \"log into github/gitlab\", \"do we have AWS creds\", \"what's the staging URL\", \"what hosts can I reach in this project\", \"what does this project use\", \"set up access to <thing>\". Also use when the user *shares* new access info (\"you can ssh to xyz\", \"the AWS account is 12345\", \"the stripe key is at ~/.ssh/foo.key\", \"the staging URL is …\"). Also use for auth status for gh/glab/cloudflare, or `/accounts`. **Use IMMEDIATELY on any SSH auth failure** — \"permission denied (publickey)\", \"too many authentication failures\", \"load key ... invalid format\", \"could not open a connection to your authentication agent\", agent has no identities, ssh hangs on password prompt, can't ssh into a host that worked yesterday. The dominant cause on this setup is KeePassXC being locked (which empties ssh-agent) — this skill checks for that BEFORE any config diagnosis."
argument-hint: <show|where|remember|check> [args]
---

# accounts

Knowledge base of what access Claude has. Read it to figure out what's possible. Write to it when the user shares new access info. Two stores:

| Store | Path | Scope |
|---|---|---|
| **Project** | `<project>/.claude/data/accounts.md` | Project-specific (this repo's deploy targets, this client's hosts, etc.) |
| **Global** | `~/.claude/skills/accounts/known.md` | Personal / cross-project (your machines, your everyday accounts) |

Project store wins on conflict (more specific). Read both, merge.

## When to invoke

Auto-invoke whenever:

- The user **asks** about connectivity or access: "can I ssh to X", "how do I connect to Y", "what's the token for Z", "do we have AWS creds here", "what's the staging URL", "log into github", "what hosts/services does this project use", "what access do I have for this project". **Any flavor of "do I have access to X" should fire this skill.**
- The user **shares** new access info: "you can ssh to xyz as foo", "the staging URL is X", "use the cloudflare token at Y", "we deploy to AWS account 12345"
- A command in another skill fails with auth and the calling skill needs to know what creds exist
- **An SSH command fails for any auth-related reason** — `Permission denied (publickey)`, `Too many authentication failures`, `Load key ... invalid format`, `Connection closed by remote host` mid-handshake, `Could not open a connection to your authentication agent`, agent has no identities, ssh hangs on a password prompt, or a previously-working host suddenly stops working. See the "SSH auth failures" section below — KeePassXC is the canonical first suspect, not the config.
- The user types `/accounts`

## Subcommands

| Form | Purpose |
|---|---|
| `/accounts show` (default) | Read both stores, print what's known |
| `/accounts where <thing>` | Find which store a piece of info lives in |
| `/accounts remember <details>` | Record new info (asks where to store) |
| `/accounts check [service]` | Read-only auth state (gh/glab/cloudflare/ssh) |

If no subcommand: assume `show`. If a service name alone is given: assume `check <service>`.

## Read flow

The skill always **inventories every relevant source**, not just the two store files. Some access info lives in `~/.ssh/config.d/*`, in `gh auth status`, in `glab auth status`, in `CLOUDFLARE_API_TOKEN`, etc. — the stores are for what you couldn't otherwise discover, plus for tagging "this project frequently uses X".

1. Determine project root: nearest git root upward from cwd. If not in a git repo, project is "none".
2. Read `<project-root>/.claude/data/accounts.md` if it exists.
3. Read `~/.claude/skills/accounts/known.md` if it exists.
4. **Inventory global sources** (read-only — do not modify):
   - SSH hosts: list `~/.ssh/config.d/*` (one file per site; each `Host <name>` block is one node). Cross-reference the matching site/banner in the [[ssh-config]] skill if needed.
   - CLI auth state on demand: `gh auth status`, `glab auth status`, `curl ... cloudflare verify` — only when the question touches these.
   - Token files: list `ls -1 ~/.ssh/*.key 2>/dev/null` (just names, never contents).
5. Merge: project entries override global-store entries on the same key. Global-store entries override raw global-source data on the same key (so a curated entry beats a discovered one).
6. Answer the user's question. For single-item lookups, label the source: `from project`, `from global`, or `from <source-file>` (e.g. `from ~/.ssh/config.d/df-austin`).

## Write flow — `remember <details>`

User shares something new. Steps:

1. **Parse** the detail into the right section (see schema below). Examples:
   - "you can ssh to web-01.foo.com as deploy with key ~/.ssh/foo_deploy" → SSH Hosts
   - "the stripe key for companyX is at ~/.ssh/companyX.stripe.key" → API Tokens / Credentials
   - "staging is at https://staging.foo.com" → URLs / Dashboards
   - "we deploy to AWS account 12345 (foo-prod)" → Services / Accounts
2. **Refuse to write actual secret values.** Only file paths, env var names, public identifiers, and URLs. If the user pastes a literal token/password/key, respond: "I'll record the file path or env var name only — paste the value into a file (chmod 600) and tell me where." Never let a raw secret value land in either store.
3. **Check** both stores AND raw global sources (`~/.ssh/config.d/*`, gh/glab/cloudflare state) for an existing entry on the same key.
   - **If it already exists globally (in a store or a raw source)**: don't duplicate. Instead offer the **project-pointer pattern** — see "Project-local relevance" below. Ask: "I already know about `<thing>` from `<source>`. Want me to note in this project's `accounts.md` that this project frequently uses it? (Pointer only — no duplication.)"
   - **If it exists with different details**: ask whether to update, duplicate (different context), or skip.
   - **If it's genuinely new**: proceed to step 4.
4. **Ask where to store**, using AskUserQuestion. Options offered depend on context:
   - **Project** (`<project-root>/.claude/data/accounts.md`) — recommended when the info is project-scoped
   - **Global** (`~/.claude/skills/accounts/known.md`) — recommended when the info is yours across projects
   - **Project pointer to global** — when the info already exists globally and the user wants to flag it as project-relevant (writes a one-line entry under "Used by this project" referencing the global source — no value duplication)
   - If cwd isn't inside a project, only Global is offered.
5. **Create the file** if missing. Project file: `mkdir -p <project-root>/.claude/data/` first. Initialize with the section headers from the schema below.
6. **Append** to the right section. Sort entries alphabetically by name within the section after the insert. Preserve any unmanaged sections, comments, or notes already in the file.
7. **Confirm** what was written and where, in one line: `Recorded SSH host 'web-01.foo.com' to project store (.claude/data/accounts.md).`

## File schema

Both stores share these sections; the project store also has an optional `## Used by this project` section for pointers.

```markdown
# Known Access

## Used by this project   <!-- project store only — pointers to global sources -->

- **<name>** — kind: <ssh|api|service|url>, source: <e.g. ~/.ssh/config.d/df-austin or "global store">, role: <one-line description of why this project uses it>

## SSH Hosts

- **<name>** — host `<hostname>`, user `<user>`, key `<key-path>`[, via `<jump-host>`][, notes: <free-form>]

## API Tokens / Credentials

- **<service> (<context>)** — file `<path>` (chmod 600), env var `<VAR_NAME>` (no value stored)

## Services / Accounts

- **<service>** — id `<id>`, name `<friendly-name>`[, notes: <free-form>]

## URLs / Dashboards

- **<name>** — `<url>`[, notes: <free-form>]
```

Bracketed `[...]` fields are optional. Empty sections may be omitted. New sections can be added if a fact doesn't fit — note new section names in `## Other` at the bottom so future writes find them.

## Project-local relevance — pointers, not duplication

Many things the user has access to are *globally discoverable* — `~/.ssh/config.d/*` defines hosts; `gh auth status` shows GitHub login; `CLOUDFLARE_API_TOKEN` is exported from the shell profile. Claude doesn't need a project store to *find* these; it just reads the global source.

What the project store adds is **relevance tagging**: "in this project, the following globally-known access is salient." Without that, a future Claude session sees 40 SSH hosts in `~/.ssh/config.d/` and doesn't know which 3 matter here.

The pattern: when the user mentions an access detail that already exists globally, **do not copy the details into the project store**. Write a one-line pointer under `## Used by this project`:

```markdown
## Used by this project

- **P64** — kind: ssh, source: ~/.ssh/config.d/df-austin, role: kube-master access for the izprod cluster
- **GitHub `companyX/web`** — kind: service, source: gh auth (global), role: this repo
- **Cloudflare zone `companyX.com`** — kind: service, source: $CLOUDFLARE_API_TOKEN (global), role: DNS + Workers for the production site
```

This means:

- The details of how to reach P64 live in `~/.ssh/config.d/df-austin` — single source of truth, never copied
- The project tag tells future-Claude *why* P64 matters here, so it surfaces only the relevant subset
- Pointers don't drift — if the SSH config changes, the pointer still resolves correctly

Only write a **full entry** in the project store when the access is genuinely project-specific and not in any global source (e.g. a per-project Stripe key that lives in the project's secrets, not in `~/.ssh/`).

## `check [service]` — read-only CLI state

Useful for the "do I currently have working auth" question. Not for fixing broken auth — this skill doesn't guide token creation.

| Service | Check command | OK signal |
|---|---|---|
| `github` | `gh auth status` | exit 0 |
| `gitlab` | `glab auth status` | exit 0 |
| `cloudflare` | `curl -fsS -H "Authorization: Bearer $CLOUDFLARE_API_TOKEN" https://api.cloudflare.com/client/v4/user/tokens/verify` | `"success":true` |
| `ssh` | `ssh-add -l` | exit 0 with at least one key listed |

If `service` is omitted, run all four. Print one short table — never the token values, just OK / MISSING / EXPIRED / NO_CLI / AGENT_EMPTY / NO_AGENT per service.

When auth is broken: report the state, point to the relevant dashboard, and stop. Token creation belongs to a separate, future skill.

- GitHub: https://github.com/settings/tokens
- GitLab: https://gitlab.com/-/user_settings/personal_access_tokens
- Cloudflare: https://dash.cloudflare.com/profile/api-tokens

## SSH auth failures — KeePassXC first, everything else later

When SSH authentication fails (`Permission denied (publickey)`, `Too many authentication failures`, `Load key ... invalid format`, `Connection closed by remote host`, `Could not open a connection to your authentication agent`, agent has no identities, ssh hangs, or a previously-working host suddenly stops working), follow this diagnostic order **strictly**. Do NOT propose IdentityFile changes, key file restoration, ssh-config edits, control-master tricks, or any per-host fix until Step 1 has run and Step 2 has been honored.

### Step 1 — Check the agent (always first)

Run:

```bash
ssh-add -l
```

Three outcomes:

| `ssh-add -l` output | Meaning | Next |
|---|---|---|
| Lists one or more keys | Agent is populated | Go to Step 3 |
| `The agent has no identities.` | Agent is up but empty | **Almost certainly KeePassXC is locked** — go to Step 2 |
| `Could not open a connection to your authentication agent.` | No agent reachable | KeePassXC isn't running or `SSH_AUTH_SOCK` isn't set in this shell — go to Step 2 |

### Step 2 — Suspect KeePassXC, prompt to unlock, STOP

On this user's setup, **KeePassXC provides ssh-agent**. When KeePassXC locks (timeout, sleep/wake, restart, or user closed it), the agent loses its keys. **~99% of SSH auth failures on this setup are KeePassXC being locked — not key files, not config, not the host.**

Tell the user, in one line, and wait:

> "ssh-agent has no keys loaded. Almost always means KeePassXC is locked — please unlock it and tell me when done. I'll re-run `ssh-add -l` before trying anything else."

Then **stop**. Do not propose any other fix. Do not edit any file. Do not retry SSH. Do not offer alternatives. Just wait for the user.

When the user confirms unlock:

1. Re-run `ssh-add -l` to verify keys are loaded.
2. Retry the original SSH command exactly as it was.
3. If it works, the diagnosis is complete. If it still fails, *now* go to Step 3.

### Step 3 — Only after Step 1 shows keys (or Step 2 has been honored and re-checked)

If the agent is populated and SSH still fails, then look at:

- IdentityFile mismatch in `~/.ssh/config` or `~/.ssh/config.d/*` (delegate to `/ssh-config`)
- Wrong username for the target host (consult the access store for the host)
- Host-specific key the agent doesn't have (check `known.md` / project `accounts.md` for the host)
- fail2ban lockout (test from a different IP, or wait the cooldown — do NOT keep retrying)
- `IdentitiesOnly=yes` blocking agent keys (try `-o IdentitiesOnly=no` once as a test)

### Step 4 — Hard "do NOT suggest" list (before Step 1 has run)

Before Step 1 runs and Step 2 is honored, these suggestions are **forbidden** because they're either irreversible, time-wasting, or both:

- "Drop the real private key at `~/.ssh/<name>`"
- "Fix the `IdentityFile` line to point at a private key"
- "Use `IdentitiesOnly=no`" *as a fix* (it's a diagnostic step at Step 3, not a fix)
- "Re-open a control-master from a working terminal"
- "Restore the missing private key from backup"
- "Just run the checks yourself with `!` and paste output" (the user has a skill for this — use it)
- Any narrative about fail2ban risk that isn't immediately actionable
- Any multi-option "pick whichever is easiest" menu

The above are all fine *after* Step 1 confirms the agent has keys. Before that, they waste the user's time and bury the real fix.

## Guardrails

- **On SSH auth failure, run `ssh-add -l` first and assume KeePassXC is locked if the agent is empty.** Do NOT propose IdentityFile edits, key file restoration, `IdentitiesOnly`, control-master tricks, or any per-host fix before that. See "SSH auth failures" above for the full forbidden list.
- **Never print, store, or echo secret values.** Only paths, env var names, and public identifiers (account IDs, URLs, hostnames).
- **Never touch token files** (`~/.ssh/*.key`, etc.). This skill does not write them, rotate them, or read their contents. It only records *where* they live.
- **Never write to a global store from inside a project unless explicitly asked.** When ambiguous, ask via AskUserQuestion.
- **Always confirm before writing** with a short "about to record X to Y" summary. Skip the confirm only when the user explicitly used `/accounts remember <full unambiguous detail>` and the parse is unambiguous.
- **Don't auto-create the project file in a non-project directory.** If there's no git root, the only available store is global.
- **Project file (`<project>/.claude/data/accounts.md`) is committable by default** — it's metadata about access, not secret values. If you're about to write to a project that looks public (open-source, no .gitignore covering `.claude/data/`), warn the user before writing — internal hostnames or account IDs may leak.
- **Global file (`~/.claude/skills/accounts/known.md`) is NOT committable.** It's personal. Stays on this machine only.

## Notes

- Safe to auto-invoke for `show`, `where`, `check`. For `remember`, always confirm before writing.
- `known.md` lives next to this `SKILL.md` inside `~/.claude/skills/accounts/`. **Skill update flows must not overwrite this file** — it's user data, not skill content. The canonical source repo (`~/workspace/x85446/claudecodetricks/skills/accounts/`) intentionally does NOT contain a `known.md`, so any `cp -r` install from source preserves the user's local data.
- Per-project file path is fixed at `<project>/.claude/data/accounts.md` regardless of where this skill is installed.
- If a previous version of this skill referenced `~/.ssh/.accounts-registry`, that file is no longer used — migrate any entries into `known.md` and delete the registry.
