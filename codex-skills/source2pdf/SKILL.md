---
name: source2pdf
description: Use when someone asks to build a master/super PDF from a _sources folder, consolidate or merge source PDFs into one file in order, assemble monthly statements/bills into a single mortgage.pdf or water.pdf, or run source2pdf. Concatenates every PDF in a _sources/ folder chronologically into one master PDF.
---


## What This Skill Does

## Usage

Argument: [path to _sources folder or its parent]. `$1` is its first word; `$ARGUMENTS` is the whole thing.

<!-- codex-port: `argument-hint` has no Codex frontmatter home; folded into this Usage section. Argument substitution is documented for Codex custom prompts but not for skills, so the meaning is stated in prose rather than left to the token alone. -->

Concatenates every PDF inside a `_sources/` folder — in filename order — into **one** master PDF, placed next to `_sources/` and named after the parent folder. This is how per-account "super-PDFs" are assembled (e.g. all monthly mortgage statements → `mortgage.pdf`, all water bills → `water.pdf`).

It is lossless and safe:
- Encrypted inputs are decrypted with `qpdf --decrypt` (common for bank statements).
- Damaged inputs are auto-repaired (`qpdf` rebuilds the cross-reference table).
- The output page count is verified against the sum of the inputs.
- `_sources/` is **never** deleted.

## Convention

```
mortgage/                        <- parent folder
├── mortgage.pdf                 <- OUTPUT (named after parent), created here
└── _sources/                    <- INPUT folder (kept intact)
    ├── 20030201-... .pdf        <- merged in filename (date-prefixed) order
    ├── 20090305 ... .pdf
    └── ...
```

Files are ordered by **basename**, so the `YYMMDD`/`YYYYMMDD` date prefix produces chronological order. This pairs with the `pdf2name` skill, which produces those date-prefixed names in the first place.

## Steps

1. Resolve the target from `$ARGUMENTS`. It may be a `_sources/` folder directly, or a parent folder that contains one. If no argument is given, ask for the path. If `$ARGUMENTS` is empty and the current working directory (or an obvious subfolder) contains a `_sources/`, offer that.

2. Run the merge script:
   ```bash
   python3 ~/.agents/skills/source2pdf/source2pdf.py "$ARGUMENTS"
   ```
   - Add `--name <file>.pdf` only if the user wants a name other than the parent-folder default.
   - Add `--force` only if the user confirms overwriting an existing output.

3. Read the script's report to the user: the ordered file list, the output path, the page count, and the `OK` / `MISMATCH` verdict.

4. If the script reports a **PAGE-COUNT MISMATCH**, do NOT present the output as trustworthy — investigate the offending input (a truncated or multi-loan file) before proceeding.

5. Never delete `_sources/`. If the user later says the master looks correct, they can remove `_sources/` themselves (or ask you to).

## Prerequisites

Needs `pdfinfo`, `pdfunite` (poppler) and `qpdf`. If the script reports a missing tool, install with:
```bash
brew install poppler qpdf
```

## Notes

- **Order matters and comes from filenames.** If the `_sources` files are not date-prefixed, the order is alphabetical, which may be wrong. Run `pdf2name` on them first, or tell the user the order is unverified.
- **One account per `_sources` folder.** This skill assumes every PDF in `_sources/` belongs to the same account/type. It does not de-duplicate or split by account — that grouping is the caller's responsibility (see the `pdf2name` skill for correct per-account naming).
- **Unique one-off documents do not belong in `_sources/`.** Closing statements, payoff letters, and other non-recurring docs should stay as separate files outside `_sources/`, not merged in.
- Escrow analyses, when kept separate, get their own `_sources/` → `escrow.pdf` in a sibling `escrow/` folder, using this same skill.
