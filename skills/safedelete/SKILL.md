---
name: safedelete
description: Use when someone wants to safely delete redundant source PDFs, verify that a merged/consolidated PDF fully contains its originals before deleting, confirm _original-bundles or _sources are represented page-for-page, or run safedelete. Proves every original page's content exists in the kept files (rendered-image md5, not eyeballing), then Trashes the originals only if SAFE.
disable-model-invocation: true
argument-hint: [folder containing _original-bundles, or --originals X --against Y]
allowed-tools: Bash, Read, Glob
---

## What This Skill Does

Before deleting source PDFs, it **proves** nothing is lost: for every page of the ORIGINALS, it checks that the same page's *content* exists somewhere in the KEEPERS. The match is a content hash, not a filename or byte compare:

- **Primary:** md5 of each page **rendered to a grayscale image** (`pdftoppm -gray`). Identical visual pages match even after `qpdf`/`pdfseparate`/`pdfunite` re-encode the bytes.
- **Fallback:** md5 of the page's **normalized text**. Flags "image differs but text identical" (e.g. a re-scan) so you can judge it.

If — and only if — every original page is matched, and you confirm, it moves the originals to the **macOS Trash** (recoverable). It never hard-deletes and never deletes on an unmatched page.

This is the safe counterpart to `source2pdf` (which builds `mortgage.pdf` from a `_sources/` folder) — run `safedelete` to confirm the build/extraction captured everything before removing `_original-bundles/`.

## Steps

1. **Verify first (never delete yet).** Run:
   ```bash
   python3 ~/.claude/skills/safedelete/safedelete.py "$ARGUMENTS"
   ```
   - Folder form: `safedelete.py <folder>` — originals = `<folder>/_original-bundles/`, keepers = every other PDF under `<folder>` (e.g. `mortgage.pdf`, `escrow.pdf`, `_sources/*`). Override the delete-candidate subfolder with `--bundle-name NAME`.
   - Explicit form: `safedelete.py --originals <file|dir> --against <file|dir> [...]`.

2. **Read the report to the user.** It lists each original page as `✓ image-match`, `~ text-match (image differs)`, or `✗ UNMATCHED`, then a verdict line: **SAFE TO DELETE** or **NOT SAFE**.

3. **If NOT SAFE:** do not delete. Show the `✗ UNMATCHED` pages and explain each is a page whose content isn't in any keeper — either a genuinely missing page, or a duplicate scan that didn't match pixel-for-pixel. Let the user decide (extract the missing page into the keepers, or accept the loss and re-run).

4. **If SAFE:** tell the user it's verified, then **ask for confirmation** before deleting. On confirmation, re-run with `--trash`:
   ```bash
   python3 ~/.claude/skills/safedelete/safedelete.py "$ARGUMENTS" --trash
   ```
   The script re-verifies and refuses if anything is UNMATCHED, then moves the originals to Trash and prints the location.

5. **Report** what was moved to Trash (and that it's recoverable from there).

## Output (verify report)

```
Verifying 2 original PDF(s) against 4 keeper(s) @ 100dpi

  _original-bundles/....4pp....pdf  (4pp)
     p1: ✓ image-match  -> mortgage.pdf p4
     ...
============================================================
pages: 11  |  image-match 10  |  text-match 0  |  UNMATCHED 1
NOT SAFE — some original pages are not represented (see UNMATCHED)
```

## Notes

- **Content, not bytes.** Never compare raw PDF md5 to decide safety — merging/decrypting rewrites the bytes, so identical pages would falsely mismatch. This skill renders and hashes the *page*, which is what actually matters.
- **A `~ text-match` is a soft pass.** The text is identical but the raster differs (rotation, a different scan, DPI baked into the source). Surface these; don't silently treat them as perfect.
- **Blank pages** hash to a uniform image and will match any other blank page of the same size — harmless, but mention it if a bundle has many blanks.
- **Trash, not rm.** Default and `--trash` both use the macOS Trash (via `trash` CLI → Finder `osascript` → `~/.Trash` fallback). Only offer a hard `rm` if the user explicitly insists.
- **Never delete on NOT SAFE**, and never delete without the user's confirmation, even when SAFE.
- **Raise the DPI** with `--dpi 150` if two genuinely different pages are colliding at 100dpi (rare; low-detail pages).
- Requires `poppler` (`pdftoppm`, `pdftotext`, `pdfinfo`); `qpdf` optional for encrypted inputs. Install: `brew install poppler qpdf`.
