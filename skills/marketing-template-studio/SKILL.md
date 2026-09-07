---
name: marketing-template-studio
description: "Use when someone asks to experiment with marketing document templates, try new one-pager designs, iterate on branded layouts, generate template variants, refine a marketing template, or A/B test PDF designs."
argument-hint: "[variant-count] [doc-type]"
---

## What This Skill Does

Interactive design studio for the `marketing-doc-formatter` templates. Produces N template variants, renders each using the `device-management` product content as the sample, shows the user where to view them, takes feedback, and iterates until a winning design emerges. When the user is happy, the skill swaps the winner into the canonical template location.

This skill is **conversational**. Each invocation does one turn:
- **First turn:** generate initial variants
- **Middle turns:** refine, add, or combine variants based on user feedback
- **Final turn:** promote a chosen variant — either as a **new slot** (`--template <new-name>`) or by **replacing an existing slot**

## Pre-Flight Checks

1. **Check marketing-doc-formatter skill is present:** Verify `.claude/skills/marketing-doc-formatter/templates/` exists.
   - If missing: "The marketing-doc-formatter skill must be installed first. Run `~/workspace/x85446/claudecodetricks/skills/skillinstall.sh marketing-doc-formatter`."
   - **Hard stop.**

2. **Check assets symlink:** Verify `./assets/Logo-Branding/izuma-logo-horizontal.svg` is accessible.
   - If missing, direct user to `./assets/README.md`.
   - **Hard stop.**

3. **Check sample source exists:** Verify `products/device-management/one-pager.md` exists (the sample content source).
   - If missing: "The device-management sample content is not present. Run `/product-discovery` for device-management first, or pick a different sample product."
   - **Hard stop.**

4. **Check tools:** `wkhtmltopdf`, `pdfinfo`, `pdftoppm` all available on PATH.
   - If missing: `brew install wkhtmltopdf poppler`
   - **Hard stop.**

## Directory Layout

All experiment artifacts live under the marketing-doc-formatter skill's templates directory:

```
.claude/skills/marketing-doc-formatter/templates/
├── one-pager/                   # canonical one-pager templates (named slots)
│   ├── business-classic/
│   │   ├── template.html
│   │   └── design-notes.md
│   ├── photo-ready/
│   │   ├── template.html
│   │   └── design-notes.md
│   └── [future-slot]/           # added via "promote as new slot"
├── multi-page.css               # shared canonical CSS
├── multi-page-body.html         # shared canonical logo snippet
└── experiments/
    ├── [variant-slug-1]/
    │   ├── design-notes.md      # description of this variant's approach
    │   ├── one-pager.html       # rendered from canonical layout with variant-specific edits
    │   └── one-pager.pdf        # sample render using device-management content
    ├── [variant-slug-2]/
    │   └── ...
    └── _archive/                # previous "promoted" templates, for rollback
```

Variant slugs are short, lowercase, hyphenated (e.g., `classic-corporate`, `bold-banner`, `sidebar-accent`, `photo-hero`).

**Canonical template slots** (`templates/one-pager/[slot-name]/`) are the templates `marketing-doc-formatter` picks from when its user passes `--template [slot-name]`. A variant in `experiments/` has no effect on production output until it's promoted into a slot.

## Invocation Modes

The skill auto-detects which mode to run based on the current state of `templates/experiments/`:

- **Mode A — New session:** `experiments/` is empty (or user asks to start fresh). Generate initial variants.
- **Mode B — Iteration:** `experiments/` contains variants. User wants to refine, add, combine, or delete.
- **Mode C — Promote:** User picks a winner to become the canonical template.

## Step-by-Step Workflow

### Step 1: Determine Mode

Check `templates/experiments/` contents.

- If empty → **Mode A**.
- If non-empty and user said "start over" / "clean slate" / "new variants from scratch" → archive existing to `experiments/_archive/[timestamp]/` and run **Mode A**.
- If user said "promote X" / "use variant X" / "pick X" → **Mode C**.
- Otherwise → **Mode B**.

### Step 2: Mode A — Generate Initial Variants

**Interview (if count not in arguments):**

Ask the user using AskUserQuestion:

> "How many variants? (default 3)
> Any specific design themes or directions to try? (e.g., 'classic corporate', 'bold with orange accents', 'sidebar design', 'photo-hero' — or 'surprise me' for AI's picks)"

**Generate variants:**

For each variant, produce:
1. A short slug (kebab-case) naming the approach
2. A `design-notes.md` file describing the variant's concept in 3–5 bullet points
3. A one-pager HTML file based on an existing slot template (default: `templates/one-pager/business-classic/template.html`) with variant-specific CSS/structure edits
4. Use the device-management sample content to fill the template (see Sample Content Substitution below)
5. Render to PDF using the `marketing-doc-formatter` one-pager command
6. Save under `templates/experiments/[slug]/`

**Variant design discipline:**
- All variants MUST comply with the Page Layout Rules in `marketing-doc-formatter/SKILL.md` (symmetric margins, no position:fixed footers, 9–10pt body font, CLI-only margins, etc.).
- Variants differ in *visual design* (color blocks, header style, accent placement, typography scale, column arrangement) — not in layout rules.
- Each variant must fit on one US Letter page at 9–10pt body font.

**Example initial variant set (use as inspiration when user says "surprise me"):**

| Slug | Concept |
|------|---------|
| `classic-corporate` | Minimalist, logo left + company-name right, subtle orange accent under footer |
| `bold-banner` | Full-width cobalt header band with centered white logo, orange underline accent |
| `sidebar-accent` | Thin cobalt left-accent bar, stats strip, orange callout box |
| `hero-callout` | Large hero paragraph at top in blue box, dense tables below |
| `photo-ready` | Reserves a placeholder image area top-right (replace later with product photo) |

### Step 3: Mode B — Iterate Based on Feedback

Read the existing `experiments/` directory and the user's feedback. Possible actions:

- **Refine a variant:** apply specific changes the user requested (e.g., "make classic-corporate's header smaller", "swap bold-banner's colors"). Edit the variant's HTML, re-render the PDF.
- **Combine variants:** create a new variant blending elements from two existing ones (e.g., `classic-corporate`'s header + `sidebar-accent`'s stats strip).
- **Add new variant:** generate an additional variant with the approach the user described.
- **Delete a variant:** remove its subdirectory from `experiments/`.
- **Duplicate and tweak:** copy an existing variant to a new slug, then modify.

For any action that creates or modifies a variant, re-render the PDF and update `design-notes.md` to reflect the change.

### Step 4: Mode C — Promote a Winner

When the user picks a winner, **ask them whether to add a new slot or replace an existing slot** using AskUserQuestion. List the existing slots so they can decide:

> "Promote `[winner-slug]` — add a new template slot or replace an existing one?
> 1. **Add new slot** — give the slot a name (default: use the variant's slug). `marketing-doc-formatter --template [new-slot-name]` will become a new option.
> 2. **Replace existing slot** — overwrite one of the current slots. Current slots:
>    - (list `ls templates/one-pager/` at runtime — e.g., `business-classic`, `photo-ready`)"

**Flow A — Add new slot:**

1. Ask the user for the slot name (default: the winner's variant slug). Validate: lowercase, hyphens only, no spaces, not already taken.
2. Create the slot:
   ```
   mkdir -p templates/one-pager/[new-slot-name]
   cp experiments/[winner-slug]/one-pager.html templates/one-pager/[new-slot-name]/template.html
   cp experiments/[winner-slug]/design-notes.md templates/one-pager/[new-slot-name]/design-notes.md
   ```
3. Update the new slot's `design-notes.md` to include a "Status: Alternate template" line and "Opt in with `--template [new-slot-name]`".
4. Sanity-check render using `marketing-doc-formatter` against device-management at `--template [new-slot-name]`. Verify 1 page and Page Layout Rules compliance.
5. Sync to backup repo (see Backup Sync below).
6. Confirm: tell the user the new slot is available via `/marketing-doc-formatter ... --template [new-slot-name]`.

**Flow B — Replace existing slot:**

1. Ask which slot to replace (AskUserQuestion; list slots present under `templates/one-pager/`).
2. Archive the outgoing slot's files:
   ```
   cp templates/one-pager/[slot]/template.html \
      experiments/_archive/[slot]-template-[YYYYMMDD-HHMM].html
   cp templates/one-pager/[slot]/design-notes.md \
      experiments/_archive/[slot]-design-notes-[YYYYMMDD-HHMM].md
   ```
3. Overwrite the slot:
   ```
   cp experiments/[winner-slug]/one-pager.html templates/one-pager/[slot]/template.html
   cp experiments/[winner-slug]/design-notes.md templates/one-pager/[slot]/design-notes.md
   ```
4. Preserve the slot's Status line. E.g., if replacing `business-classic`, keep its "Status: Canonical · Default" header in design-notes.md.
5. Sanity-check render using `marketing-doc-formatter --template [slot]` against device-management. Verify 1 page.
6. Sync to backup repo.
7. Confirm: tell the user the slot is updated, the previous version is archived, and any future `/marketing-doc-formatter --template [slot]` uses the new design.

**Backup Sync (both flows):**
```
cp -r <deploy-path>/templates/one-pager/ \
      ~/workspace/x85446/claudecodetricks/skills/marketing-doc-formatter/templates/one-pager/
```

### Step 5: Report to User

After every invocation, end with a summary that includes:

- **List of current variants** with file paths to their PDFs (so user can open them directly).
- **For new session (Mode A):** brief description of each variant's design direction.
- **For iteration (Mode B):** what changed this turn.
- **For promote (Mode C):** confirmation + archive location.
- **Next step prompt:** ask the user how to proceed (e.g., "Which one do you want to iterate on, or are you ready to promote a winner?").

**Example report format:**

```
## Current variants

| Slug | PDF Location | Design |
|------|--------------|--------|
| classic-corporate | templates/experiments/classic-corporate/one-pager.pdf | Minimalist, logo left, orange footer accent |
| bold-banner | templates/experiments/bold-banner/one-pager.pdf | Full cobalt header band, centered logo |
| sidebar-accent | templates/experiments/sidebar-accent/one-pager.pdf | Left accent bar, stats strip, orange callout |

Open each PDF in your viewer and compare.

**Next:** What's your feedback? Options:
- "refine [slug]: [change]"
- "combine [slug-A] + [slug-B]"
- "add a [style] variant"
- "delete [slug]"
- "promote [slug]" (when you've picked a winner)
```

## Sample Content Substitution

When rendering a variant, fill the template with device-management content:

1. Read `products/device-management/one-pager.md` for the source marketing copy.
2. Extract logical content blocks:
   - Title: "Izuma Device Management"
   - Subtitle: "Chip-to-Cloud IoT at Scale"
   - Intro paragraph: first paragraph after the title
   - Core Capabilities: 4–5 h3 blocks
   - Device Spectrum table
   - Deployment Options table
   - Why Not Build It Yourself? comparison table
   - Key Differentiators: final accent box
3. Map these into the variant's HTML structure.
4. Resolve `{{LOGO_ABS_PATH}}` to the absolute path of `./assets/Logo-Branding/izuma-logo-horizontal.svg` (use `realpath`).

Content stays identical across variants so designs can be compared apples-to-apples. Don't re-write the copy between variants — only the layout and styling differ.

## Render Command

Use the marketing-doc-formatter one-pager command for every render:

```bash
wkhtmltopdf --enable-local-file-access --page-size Letter \
  --margin-top 12mm --margin-bottom 12mm --margin-left 15mm --margin-right 15mm \
  --disable-smart-shrinking \
  templates/experiments/[slug]/one-pager.html \
  templates/experiments/[slug]/one-pager.pdf
```

If the variant doesn't fit one page at 9pt body font, reduce it via the abbreviation procedure from the `marketing-doc-formatter` SKILL — but note this in `design-notes.md`. Do not save an abbreviated variant as canonical-replacement material without flagging it.

## Notes

- This skill **does not modify** `products/device-management/one-pager.md` or any product markdown. Content comes from there read-only.
- This skill **does modify** files under `.claude/skills/marketing-doc-formatter/templates/experiments/` freely and, in promote mode, one of the canonical slot directories under `templates/one-pager/[slot-name]/`.
- Slots under `templates/one-pager/` are the source of truth for `marketing-doc-formatter`. Until a variant is promoted into a slot, it has no effect on real marketing output.
- Archive previous slot contents in `experiments/_archive/` (prefixed with slot name + timestamp) before overwriting — never delete without a backup.
- Variants that violate the Page Layout Rules (asymmetric margins, position:fixed footers, etc.) must NOT be produced. If the user asks for something that violates rules, explain the constraint and propose a compliant alternative.
- If the user wants to extend this workflow to `multi-pager.md`, `whitepaper.md`, or a new `features.md` template, the skill supports it — use the corresponding canonical files (`multi-page.css` + `multi-page-body.html`). In that case, experiment artifacts are stored per-doc-type: `experiments/[slug]/multi-pager.{html,pdf}` etc. For simplicity, the default flow focuses on one-pagers.
- Maximum practical variant count per session: ~6. Beyond that, comparisons get unwieldy.
- `design-notes.md` inside each variant directory is the concept memo. Keep it short (3–5 bullets). Update it when the variant changes.
