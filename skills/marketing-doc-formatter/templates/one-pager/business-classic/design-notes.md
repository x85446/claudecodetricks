# business-classic — One-Pager Template

**Status:** Canonical · Default choice when `--template` is not specified

## Design

- Restrained, minimalist B2B aesthetic
- Logo left + "Izuma Networks" company name right; thin blue rule under header
- Two-column body: capabilities on left, spec tables on right
- Full-width competitive comparison table below
- Orange accent reserved for the closing differentiators box only
- No photo placeholders — pure text/table layout

## When to use

- Default choice for most product one-pagers
- Situations where you don't have a product image or want a text-only feel
- Conservative / enterprise audiences
- Documents that go through print-to-PDF workflows where images might degrade

## Placeholders

- `{{LOGO_ABS_PATH}}` — absolute path to izuma-logo-horizontal.svg
- `{{TITLE}}`, `{{SUBTITLE}}`, `{{INTRO_PARAGRAPH}}` — title block
- Capability blocks (`{{CAPABILITY_N_TITLE}}`, `{{CAPABILITY_N_DESCRIPTION}}`)
- Table headings and rows
- `{{DIFFERENTIATORS_INLINE}}` — closing accent box text
