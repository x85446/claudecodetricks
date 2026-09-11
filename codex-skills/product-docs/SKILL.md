---
name: product-docs
description: Invoked as iterate plans' standing final task, or directly ("update the product docs", "sync the user docs", "document the new features"). Developer/internal docs are out of scope.
---


# $product-docs — keep the user docs true to the product

**Version:** 1.0.0

<!-- codex-port: Codex frontmatter permits only name and description, so the
     version lives here in the body. Read it from this line when stamping a
     plan's planner-version / executor-version. -->


End-user documentation only: what the product does and how to operate it, written for someone who uses it and has never seen the code. Internal/dev docs (architecture, build, contributing) are out of scope.

## What this skill does

<!-- codex-port: moved out of the startup description, which is charged against Codex's manifest budget in every session. This text is documentation, not routing signal, so it belongs at the body level where it loads on trigger. No trigger phrase was moved. -->

Maintains the END-USER product documentation of the current project — each invocation brings the docs to match the product as it is right now: documents new features and how to operate them, updates changed behavior, and DELETES documentation for removed features.

## Usage

Argument: <optional scope hint — default: everything that changed since docs were last true>. `$1` is its first word; `$ARGUMENTS` is the whole thing.

<!-- codex-port: `argument-hint` has no Codex frontmatter home; folded into this Usage section. Argument substitution is documented for Codex custom prompts but not for skills, so the meaning is stated in prose rather than left to the token alone. -->

## Where the docs live

1. If the project already has end-user docs (a `docs/` user guide, `USERGUIDE.md`, a docs site source, a README "Usage" section that clearly serves end users) — **that's the home; follow its existing structure and voice.** Never start a parallel doc tree beside a live one.
2. Otherwise create `docs/product/USERGUIDE.md` as the home (one page until it earns splitting; feature pages as `docs/product/<feature>.md` when a section outgrows the guide).

## Steps

1. **Determine what changed.** In order of preference: the current iterate plan's `## Changelog draft` + checked-off steps (when invoked as a plan's standing docs task); else the branch diff vs the default branch; else `CHANGELOG.md`'s entries newer than the docs' last commit; else `$1`'s hint. The unit of interest is user-visible behavior — internal refactors produce no doc changes.
2. **Three passes over the docs, every invocation:**
   - **Add** — every new user-visible feature gets documented: what it is, how to operate it (the actual commands/clicks/inputs, with a realistic example), and any limits. Verify each operating instruction against the real product (run the command, load the page) before writing it — docs describe what IS, not what the plan intended.
   - **Update** — changed behavior: rewrite the affected sections to the current truth. No "previously"/"as of this release" narration — active voice, present tense, the system as it is right now.
   - **Delete** — features removed from the code lose their documentation entirely: sections, references, examples, screenshots, index/TOC entries. Verify removal in the code (the flag/endpoint/UI is gone), then cut without a tombstone. A doc describing a dead feature is worse than no doc.
3. **Sweep for internal consistency**: TOC matches sections, cross-references resolve, examples use current syntax end to end.
4. **Report**: `docs synced: N features added, M updated, K removed` + one line each. When run as an iterate step, this edit lands on the plan's branch like any other change and rides the same PR (add a `[changed] user docs synced` line to the plan's Changelog draft).

## Rules

1. **The product is the source of truth, not the plan.** Document what you can demonstrate in the final tree; a planned-but-cut feature gets nothing.
2. **Deletion is mandatory, not optional** — stale docs for removed features are the primary failure this skill exists to prevent.
3. **End-user voice throughout**: benefit-first, no jargon, no file paths into the source, no change-history narration. If nothing user-visible changed, say `docs already true — no changes` and touch nothing.
