# photo-ready — One-Pager Template

**Status:** Alternate template · Opt in with `--template photo-ready`

## Design

- Reserves a photo/image placeholder zone in the top-right (roughly 2:1.3 aspect, ~180×120px)
- Intro and title flow in the top-left column beside the image
- Body below is a standard two-column layout identical to `business-classic`
- Orange-dashed border marks the placeholder zone — remove border once image is in place
- Everything else matches brand conventions (colors, fonts, table style)

## When to use

- Products that have strong visuals: actual hardware shots, architecture diagrams, dashboard screenshots, topology diagrams
- Trade-show collateral where a hero image draws the eye
- Executive-audience handouts where a visual helps orient non-technical readers
- Any situation where text-only feels too dry

## Placeholders

Same as `business-classic`, plus:
- The image placeholder is currently styled dashed-border empty box. Replace with `<img src="...">` pointing to the product image (absolute path or data-URL).
- Remove `border: 2px dashed #FF9C2B;` from `.photo-placeholder` once an actual image is inserted.

## Notes

- If no image is supplied, the placeholder still renders (dashed box with "[ PRODUCT IMAGE OR ARCHITECTURE DIAGRAM ]" text) — so the document remains usable as a draft
- Image file size affects PDF size; prefer SVG or optimized PNG/JPG under 500KB
