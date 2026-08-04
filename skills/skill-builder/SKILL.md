---
name: skill-builder
description: Use when creating new skills, optimizing existing skills, or auditing skill quality. Guides skill development following Claude Code official best practices.
---

## What This Skill Does

Guides the creation and optimization of Claude Code skills using official best practices. Use this whenever:

- Building a new skill from scratch
- Optimizing or auditing an existing skill
- Deciding on advanced features (subagent execution, hooks, dynamic context, etc.)
- Troubleshooting a skill that isn't working correctly

For the complete technical reference on all frontmatter fields, advanced patterns, and troubleshooting, see [reference.md](reference.md).

## Skill Naming & Families

Skills are organized into **families** by naming convention. This is how related skills stay legible in a flat directory. Apply it to every new skill and every audit.

**Core fact:** for personal/project skills the **directory name IS the `/command`**. Frontmatter `name:` is only a display label and must be lowercase-hyphen with **no leading hyphen** — so for orphan skills the dir and `name:` intentionally differ.

**The four roles:**

| Role | Pattern | Example | `disable-model-invocation` | Notes |
|---|---|---|---|---|
| **Meta** (orchestrator) | plain name | `importer`, `categorize`, `organizer` | unset (auto-invocable) | Owns a pipeline/family; coordinates children via the Skill tool. |
| **Child** | `<meta>-<child>` | `categorize-venue`, `importer-audit` | **unset — leave it OFF** | Worker owned by a meta. MUST stay model-invocable or its meta can't delegate to it via the Skill tool. Add `context: fork` if it must run isolated. Control triggering with a narrow description, not a flag. |
| **Sub-family** | `<meta>-<meta>-<child>` | `importer-audit-schema` | **unset — leave it OFF** | Use only when a child is itself a meta with its own workers. Prefix chains left→right, most-general first. Cap at 3 tiers — deeper means it should become its own top-level meta. |
| **Orphan** | `-<child>` (leading hyphen) | `-renamer` → `/-renamer` | usually unset | "Hasn't found a home / baggage." A standalone or not-yet-classified worker. Dir leads with `-`; `name:` stays hyphen-free (`name: renamer`). Set `true` ONLY if nothing delegates to it and it's user-only (destructive/expensive). |

**Rules:**

1. **A family prefix requires an existing meta of that exact dir name.** `categorize-*` is only valid if a `categorize` meta exists.
2. **Do NOT set `disable-model-invocation: true` on a child (or any skill a meta delegates to).** That flag blocks the **Skill tool entirely** — not just auto-triggering — so a meta calling `Skill(categorize-linker)` fails with *"cannot be used with Skill tool due to disable-model-invocation."* There is no flag for "delegatable but won't auto-fire," because Skill-tool delegation *is* model invocation. Keep the flag OFF and control unwanted triggering with a **narrow, distinct description**. Reserve `disable-model-invocation: true` for user-only standalones that no meta delegates to (typically destructive/expensive, meant to be typed by the user).
3. **Orphans keep a normal (hyphen-free) `name:` field.** Accept the dir≠name mismatch — that mismatch is the *signal* the skill is unhomed. When an orphan is adopted, rename `-renamer` → `<meta>-renamer` (see rule 6).
4. **Grouping is naming-only.** Discovery is a flat scan of each `.claude/skills/` root; prefixes do NOT nest folders and a child folder inside a meta folder is NOT discovered.
5. **Descriptions carry routing.** Meta description = whole-goal triggers; child description = narrow sub-task with distinct terms so it won't collide with its meta.
6. **Renames use `git mv`** (preserve history) and rewire every `/command` reference — skill bodies, `CLAUDE.md`, `oracle.md` — in one commit. Leave historical logs (e.g. `.claude/iterate/archive/`) untouched.

**Decide the role for any skill:** Does it orchestrate others? → **meta** (plain name). Is it owned by/triggered as a stage of a meta? → **child** (`<meta>-<child>`, `disable-model-invocation` left OFF so the meta can delegate). Is that child big enough to own its own workers? → **sub-family** (`<meta>-<meta>-<child>`). None of the above / undecided? → **orphan** (`-<child>`) until it finds a home.

## Quick Start: What Is a Skill?

A skill is a reusable set of instructions that tells Claude Code how to handle a specific task. Skills live in `.claude/skills/[skill-name]/SKILL.md` inside your project. When you type `/skill-name` or describe what you need in natural language, Claude loads the skill's instructions and follows them.

Think of skills as SOPs for Claude. Instead of re-explaining a workflow every conversation, you write it once and invoke it forever.

**How they work under the hood:**
- Your project's `CLAUDE.md` instructions are always loaded, every conversation
- Skill *descriptions* (from frontmatter) are always loaded so Claude knows what's available
- The full skill content only loads when the skill is actually invoked
- Once loaded, Claude follows the skill's instructions while still respecting your CLAUDE.md rules

---

## Mode 1: Build a New Skill

When building a new skill, run the **Discovery Interview** first. Do NOT start writing files until discovery is complete.

### Discovery Interview

Ask questions using AskUserQuestion, one round at a time. Each round covers one topic. Move to the next round only after the user answers. Keep going until you're 95% confident you understand the skill well enough to build it without further clarification.

**Round 1: Goal & Name**
*Why this matters: A clear goal prevents scope creep. The name becomes the `/slash-command`, so it needs to be memorable and specific.*

- What does this skill do? What problem does it solve or what workflow does it automate?
- What should we call it? (Suggest a name based on their answer -- lowercase, hyphens, max 64 chars)
- **Which family/role?** Apply "Skill Naming & Families" above: is this a **meta** (plain name), a **child** of an existing meta (`<meta>-<child>`), a **sub-family** (`<meta>-<meta>-<child>`), or an **orphan** with no home yet (`-<child>`)? If it's a child, confirm the parent meta exists. Default to **orphan** (`-<name>`) when the home is unclear — don't invent a meta prefix that doesn't exist.

**Round 2: Trigger**
*Why this matters: The `description` field is how Claude decides whether to load your skill. Bad trigger words mean Claude never uses it. Too broad means Claude fires it when you don't want it.*

- What would someone say to trigger this? (Get 2-3 natural language phrases)
- Should it be user-only (`/slash-command`), Claude-auto-invocable, or both?
- Does it accept arguments? If so, what? (e.g., a topic, a URL, a file path)

**Round 3: Step-by-Step Process**
*Why this matters: Claude follows instructions literally. Vague steps produce vague results. Specific steps produce consistent output every time.*

- Walk me through exactly what should happen from trigger to output. What's step 1? Step 2? Keep going.
- For each step: Does Claude do it directly, or delegate to a subagent/script?
- Does this need to be conversational (back-and-forth with the user) or is it a fire-and-forget task?

**Round 4: Inputs, Outputs & Dependencies**
*Why this matters: Skills that don't specify where to find inputs or where to put outputs produce inconsistent results. Nailing this down makes the skill reliable.*

- What inputs does the skill need? (Files, API responses, user arguments, live data)
- What does it produce? (Files, text output, structured data) Where do outputs go?
- Does it need external APIs, scripts, or tools? Which ones?
- Does it need reference files, style guides, templates, or examples?

**Round 5: Guardrails & Edge Cases**
*Why this matters: Skills without guardrails can produce unexpected behavior -- wrong outputs, unnecessary API costs, or actions you didn't intend.*

- What could go wrong? What are the common failure modes?
- What should this skill NOT do? Any hard boundaries?
- Are there cost concerns? (API calls, AI image generation, etc.)
- Any ordering or dependency constraints? (e.g., "must check X before doing Y")

**Round 6: Confirmation**
*Why this matters: Misunderstandings caught here save you from rebuilding the skill later.*

After all rounds, summarize your understanding back to the user in this format:

```
## Skill Summary: [name]

**Goal:** [one sentence]
**Trigger:** `/name` + [natural language phrases]
**Arguments:** [what it accepts, or "none"]

**Process:**
1. [step]
2. [step]
...

**Inputs:** [what it reads/needs]
**Outputs:** [what it produces + where]
**Dependencies:** [APIs, scripts, agents, reference files]
**Guardrails:** [what can go wrong, what to avoid]
```

Ask: "Does this capture it? Anything to add or change?" Only proceed to building once the user confirms.

**Skipping rounds:** If the user provides enough context upfront (e.g., they describe the full workflow in their first message), skip rounds that are already answered. Don't re-ask what you already know.

### Build Phase

Once discovery is complete, build the skill following these steps:

**Step 1: Choose the skill type**

- **Task skills** (most common) give step-by-step instructions for a specific action. Invoked with `/name` or natural language. Examples: generate a report, summarize a PR, deploy code.
- **Reference skills** add knowledge Claude applies to current work without performing an action. Examples: coding conventions, API patterns, style guides.

**Step 2: Configure frontmatter**

Set these fields based on what you learned in discovery:

- `name` -- Matches the directory name. Lowercase, hyphens, max 64 chars. **Exception:** orphan skills whose directory leads with a hyphen (`-renamer`) keep a hyphen-free `name:` (`renamer`) — YAML names can't lead with `-`, and the dir≠name mismatch is the intended "unhomed" signal. Family skills use the full prefixed dir name (`categorize-venue`) as both dir and `name`.
- `description` -- Written as: "Use when someone asks to [action], [action], or [action]." Include natural keywords from the trigger phrases. For **children**, keep it narrowly scoped with distinct terms so it won't collide with its meta.
- `disable-model-invocation: true` -- Set ONLY for user-only standalones that no meta delegates to (destructive/expensive skills meant to be typed by the user). **Never set it on a child/worker a meta delegates to** — it blocks the Skill tool and the meta's `Skill(child)` call will fail. Control child triggering with a narrow description instead. Metas leave this unset.
- `argument-hint` -- Set if the skill accepts arguments. Shows in the `/` menu autocomplete.
- `context: fork` + `agent` -- Set if the skill is self-contained and doesn't need conversation history.
- `model` -- Set if a specific model capability is needed.
- `allowed-tools` -- Set if the skill should have restricted tool access.

Only set fields you actually need. Don't add frontmatter just because you can.

For the full field reference and invocation control matrix, see [reference.md](reference.md).

**Step 3: Write the skill content**

Structure task skills as:
1. **Context** -- Files to read, APIs to call, reference material to load
2. **Step-by-step workflow** -- Numbered steps. Each step tells Claude exactly what to do.
3. **Output format** -- What the result looks like. Include templates, file paths, structured formats.
4. **Notes** -- Edge cases, constraints, what to delegate, what NOT to do.

Content rules:
- Keep SKILL.md under 500 lines. Move detailed reference material to supporting files.
- Use `$ARGUMENTS` / `$N` for dynamic input from arguments.
- Use `!`command`` for dynamic context injection (preprocessing).
- Be specific about agent delegation -- include exact prompt text.
- Specify all file paths (inputs, outputs, scripts, references).

**Step 4: Add supporting files (if needed)**

If your skill needs detailed reference docs, examples, or scripts, add them alongside SKILL.md in the same directory. Reference them from SKILL.md so Claude knows they exist. Supporting files are NOT loaded automatically -- they load only when Claude needs them. See [reference.md](reference.md) for the full pattern.

**Step 5: Deploy, Backup & Register in Installer**

All skills are backed up to the central skill repository and deployed to their target project.

**Backup location (always):** `~/workspace/x85446/claudecodetricks/skills/[skill-name]/`
This is the canonical copy. The installer script deploys from here.

**Deploy location:** The target project's `.claude/skills/` directory. Ask the user which project to deploy to using AskUserQuestion. There is no default -- the user must specify.

Known deploy targets (for reference):

| Target | Path |
|---|---|
| Izuma Marketing | `~/workspace/izuma/marketing/.claude/skills/` |
| Travis Taxes (Google Drive) | `~/Library/CloudStorage/GoogleDrive-travis.mccollum@gmail.com/My Drive/TRAVIS_Taxes/.claude/skills/` |
| ClaudeCodeTricks (finance pipeline) | `~/workspace/x85446/claudecodetricks/temp/.claude/skills/` |

Then:
1. Write the skill files (SKILL.md + any supporting files) to the **backup location** first: `~/workspace/x85446/claudecodetricks/skills/[skill-name]/`
2. Copy the same files to the **deploy location**: `[deploy-path]/.claude/skills/[skill-name]/`
3. **Register in the installer** — add a case entry to `~/workspace/x85446/claudecodetricks/skills/skillinstall.sh`:

```bash
# In the do_install() function, add:
<skill-name>)  install_skill <skill-name> "$TARGET_VAR" ;;
```

Where `$TARGET_VAR` is one of the predefined deploy target variables at the top of `skillinstall.sh`:
- `$IMARKETING` — Izuma Marketing
- `$TAXES` — Travis Taxes (Google Drive)
- `$CCTRICKS` — ClaudeCodeTricks finance pipeline

If the deploy target doesn't have a variable yet, add one to the "Deploy targets" section of `skillinstall.sh`.

4. Confirm all three steps succeeded (backup, deploy, installer registration).

If the user declines a location (skips deploy), still write to backup and register in the installer.

**Step 6: Document in CLAUDE.md**

Your project's `CLAUDE.md` file is where Claude loads project-wide instructions every conversation. After creating a skill, add a brief entry so you (and your team) know what's available:

- Skill name and `/slash-command`
- Trigger phrases
- Brief description of what it does
- Output location (if it produces files)

This isn't required for the skill to work, but it keeps your project organized and helps Claude understand how skills fit into your broader workflow.

**Step 7: Test**

Test both invocation methods:

1. **Natural language** -- Say something matching the description. Does Claude load the skill?
   - If not, revise the `description` field to include the keywords you used
   - Try 2-3 different phrasings to verify it triggers reliably
2. **Direct invocation** -- Run `/skill-name` with test arguments
   - Verify `$ARGUMENTS` / `$N` are substituting correctly
   - Check that outputs go where expected
3. **Edge cases** -- Try invoking with missing arguments, unusual input, or empty input
4. **Character budget** -- If you have many skills, run `/context` to confirm your skill's description is being loaded. If it's not, your total descriptions may exceed the budget (see [reference.md](reference.md) for details).

If issues arise, see Troubleshooting in [reference.md](reference.md).

### Complete Example

Here's a minimal but complete skill you can use as a starting template:

**File:** `.claude/skills/meeting-notes/SKILL.md`

```yaml
---
name: meeting-notes
description: Use when someone asks to summarize meeting notes, recap a meeting, or format meeting minutes.
argument-hint: [topic or date]
---

## What This Skill Does

Takes raw meeting notes and produces a structured summary with action items.

## Steps

1. Ask the user to paste their raw meeting notes (or provide a file path).
2. Extract the following from the notes:
   - **Attendees** -- Who was in the meeting
   - **Key decisions** -- What was decided
   - **Action items** -- Who owes what, with deadlines if mentioned
   - **Open questions** -- Anything unresolved
3. Format the output using the template below.
4. If $ARGUMENTS is provided, use it as the meeting title. Otherwise, infer a title from the content.

## Output Template

# Meeting: [title]
**Date:** [date if mentioned, otherwise "Not specified"]
**Attendees:** [comma-separated list]

## Key Decisions
- [decision]

## Action Items
- [ ] [person]: [task] (due: [date or "TBD"])

## Open Questions
- [question]

## Notes

- Keep summaries concise. Don't add commentary or embellish.
- If notes are too vague to extract action items, flag that to the user instead of making them up.
```

---

## Mode 2: Audit an Existing Skill

Use this checklist to audit any existing skill. Read the skill file first before running through the checklist. Fix issues before marking the audit complete.

### Frontmatter Audit

- [ ] `name` matches the directory name — **except orphans** (`-name` dir keeps `name: name`, no leading hyphen)
- [ ] **Naming role is correct** (see "Skill Naming & Families"): meta = plain name; child = `<meta>-<child>` and the parent meta exists; sub-family = `<meta>-<meta>-<child>` (≤3 tiers); orphan = `-<child>` when unhomed
- [ ] **`disable-model-invocation: true` is NOT set on any child/worker a meta delegates to** (it would break `Skill(child)` delegation) — reserve it for user-only standalones
- [ ] `description` uses natural keywords someone would actually say when they need this skill
- [ ] `description` is specific enough to avoid false triggers but broad enough to catch real requests
- [ ] `disable-model-invocation: true` is set if the skill has side effects (generates files, calls APIs, sends messages, costs money) AND no meta delegates to it (if a meta delegates to it, leave the flag OFF — see the child rule above)
- [ ] `argument-hint` is set if the skill accepts arguments via `/name`
- [ ] `allowed-tools` is set if the skill should NOT have access to all tools
- [ ] `context: fork` is used if the skill is self-contained and produces verbose output
- [ ] `model` is set only if a specific model capability is needed
- [ ] No unnecessary fields are set (don't add frontmatter just because you can)

### Content Audit

- [ ] Total SKILL.md is under 500 lines (detailed reference moved to supporting files)
- [ ] Clear step-by-step workflow with numbered steps (for task skills)
- [ ] Output format is specified with templates or examples
- [ ] All file paths and locations are documented
- [ ] Agent delegation instructions include the actual prompt text to send
- [ ] Notes section covers edge cases, constraints, and what NOT to do
- [ ] No vague instructions -- every step tells Claude exactly what to do
- [ ] String substitutions (`$ARGUMENTS`, `$N`) are used where the skill takes input

### Integration Audit

- [ ] Skill is documented in CLAUDE.md (recommended, not required)
- [ ] Supporting files (if any) are referenced from SKILL.md, not orphaned
- [ ] Scripts (if any) have correct file paths and are executable
- [ ] API keys (if any) are stored in environment variables, never hardcoded

### Quality Audit

- [ ] A beginner could follow the instructions without prior context
- [ ] Instructions are actionable, not abstract
- [ ] Delegates to subagents when appropriate to keep main context clean
- [ ] Doesn't duplicate information that lives elsewhere (CLAUDE.md, other skills)
- [ ] Output paths follow a predictable convention

### Optimization Opportunities

After running the audit, check [reference.md](reference.md) for advanced features that could improve the skill: `context: fork`, `allowed-tools`, dynamic context injection, hooks, and supporting files.

---

## Recommended Conventions

Adapt these to fit your project:

- Skills live in `.claude/skills/[skill-name]/SKILL.md`
- **Name by family** (see "Skill Naming & Families"): meta = plain name, child = `<meta>-<child>`, 3-tier = `<meta>-<meta>-<child>`, orphan/unhomed = `-<child>`. Leave `disable-model-invocation` OFF on children (metas must be able to delegate to them); renames use `git mv`.
- Output files go in a predictable location (e.g., `output/[skill-name]/`)
- API keys go in environment variables, never hardcoded in skill files
- Document all active skills in your project's CLAUDE.md
- Frontmatter `description` is written as: "Use when someone asks to [action], [action], or [action]."

## Important Notes

- Always read an existing skill before optimizing it. Never propose changes to a skill you haven't read.
- When building a new skill, check if a similar skill already exists that could be extended instead.
- For advanced patterns (subagent execution, hooks, permissions), see [reference.md](reference.md).
