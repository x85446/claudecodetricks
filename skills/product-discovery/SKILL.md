---
name: product-discovery
description: "Use when someone asks to research a product, discover features from a website, crawl and analyze a product, do product discovery, extract product capabilities from a URL, or build product knowledge from a website."
argument-hint: "<url> <product-name>"
disable-model-invocation: true
---

## What This Skill Does

Deep-crawls a website to build comprehensive product knowledge, then generates marketing documents from that knowledge. Uses a team of specialized agents to research, extract, and write.

## Step-by-Step Workflow

### Step 1: Parse Arguments

- Extract the URL and product name from the arguments.
- If either is missing, ask the user for them using AskUserQuestion.
- Normalize the product name to lowercase with hyphens for the directory path.

### Step 2: Check Existing State

Check if `products/[product]/research.md` already exists.

If it does, ask the user:

> "research.md already exists for this product. Should I:
> 1. **Replace** — discard and start fresh
> 2. **Update** — read the existing file and incorporate new findings
> 3. **Skip research** — keep existing research, just generate documents from it"

If it doesn't exist, proceed directly to research.

### Step 3: Read Existing Product Files

Read all existing files in `products/[product]/` for context. These may include:
- `features.md`
- `one-pager.md`
- `multi-pager.md`
- `whitepaper.md`
- `roadmap.md`
- `research.md` (if updating)

Feed this context to the research agents so they know what's already documented.

### Step 4: Launch Research Team

Launch three agents in parallel using the Agent tool. Each agent receives:
- The target URL
- The product name
- Any existing product context from Step 3
- The domain-scoping rules (see Crawl Rules below)

**Agent 1: Site Crawler**

```
Prompt: "You are a thorough website crawler for marketing research.

TARGET URL: [url]
PRODUCT: [product-name]

Your job:
1. Fetch the target URL.
2. Extract all meaningful content (text, headings, structure).
3. Find all internal links and links to sibling domains (see domain rules below).
4. Follow those links and extract their content too.
5. Go deep — follow links from linked pages as well. Aim for comprehensive coverage.
6. Pace yourself: wait briefly between fetches. Do not hammer the site.
7. Skip: images, videos, PDFs, login pages, form submissions.

DOMAIN RULES:
- The target domain and any sibling domains are in-scope.
  e.g., if target is izumanetworks.com, then izuma.com is also in-scope.
- Do NOT follow links to unrelated third-party sites (github.com, kubernetes.io, etc.).
- For well-known external references (Kubernetes, Docker, etc.), note what was referenced but use your own knowledge instead of crawling.

OUTPUT: Return a structured dump of ALL content found, organized by page URL. Include:
- Page title and URL
- Full text content
- Links found (with notes on which were followed)
- Site structure / navigation map

Be thorough. This content feeds into a marketing analysis pipeline."
```

**Agent 2: Feature Extractor**

```
Prompt: "You are a product feature analyst for marketing research.

TARGET URL: [url]
PRODUCT: [product-name]
EXISTING CONTEXT: [paste any existing features.md or research.md content]

Your job:
1. Fetch the target URL and follow links within the same domain realm.
2. Identify and catalog every product feature, capability, and technical specification.
3. For each feature, extract:
   - Feature name
   - Description (what it does)
   - Technical details (how it works, if available)
   - Which product tier/edition it belongs to (if mentioned)
4. Look for: feature pages, documentation, API references, changelog/release notes, comparison pages.
5. Organize features into logical categories.

DOMAIN RULES: Stay within the target domain and sibling domains. Use common knowledge for external tech references.

OUTPUT: Return a structured list of all discovered features, categorized and detailed. Include the source URL for each feature."
```

**Agent 3: Benefits & Messaging Analyst**

```
Prompt: "You are a marketing messaging analyst.

TARGET URL: [url]
PRODUCT: [product-name]
EXISTING CONTEXT: [paste any existing one-pager.md or whitepaper.md content]

Your job:
1. Fetch the target URL and follow links within the same domain realm.
2. Extract all marketing messaging, value propositions, and positioning:
   - Customer benefits (not features — the outcomes customers care about)
   - Target personas / audiences (who is this product for?)
   - Use cases and scenarios
   - Customer testimonials or case studies
   - Competitive positioning / differentiation claims
   - Pricing model (if public)
   - Call-to-action messaging
3. Look for: landing pages, about pages, case studies, blog posts, press releases, pricing pages.
4. Note the tone and voice used in existing marketing copy.

DOMAIN RULES: Stay within the target domain and sibling domains. Use common knowledge for external references.

OUTPUT: Return a structured analysis of all marketing messaging found. Organize by: value propositions, target personas, use cases, competitive positioning, tone/voice analysis. Include source URLs."
```

### Step 5: Compile Research Document

After all three agents return, compile their findings into `products/[product]/research.md` with this structure:

```markdown
# [Product Name] — Product Research

**Source:** [url]
**Research Date:** [today's date]

## Site Map
[From Site Crawler — key pages discovered]

## Features & Capabilities
[From Feature Extractor — categorized feature list]

### [Category 1]
- **[Feature Name]:** [description] *(source: [url])*

### [Category 2]
...

## Marketing Messaging & Positioning

### Value Propositions
[From Benefits Analyst]

### Target Personas
[From Benefits Analyst]

### Use Cases
[From Benefits Analyst]

### Competitive Positioning
[From Benefits Analyst]

### Tone & Voice
[From Benefits Analyst]

## Raw Content Index
[From Site Crawler — summary of all pages crawled with key content]
```

If updating (not replacing), merge new findings with existing content. Flag what's new vs. what was already documented.

Write the file to `products/[product]/research.md`.

### Step 6: Ask Which Documents to Generate

Present the user with options:

> "Research complete. Which documents should I generate?
> 1. **features.md** — structured feature/capability list
> 2. **one-pager.md** — single-page product summary
> 3. **multi-pager.md** — detailed product overview
> 4. **whitepaper.md** — comprehensive technical/business document
> 5. **All of the above**
>
> Enter numbers (e.g., 1,2,4) or 5 for all:"

### Step 7: Launch Final Writer

For each selected document, launch a writer agent. The writer receives `research.md` plus any existing version of the target document.

**Writer Agent Prompt Template:**

```
Prompt: "You are a marketing document writer for [product-name].

Read the research file at products/[product]/research.md for comprehensive product knowledge.

[If existing file]: Also read the existing products/[product]/[doc-type].md for reference on current structure and content.

Write a [doc-type] for this product:

DOCUMENT TYPES:
- features.md: Structured feature list with categories, descriptions, and technical details. Organized for a technical audience evaluating the product.
- one-pager.md: Concise single-page product summary. Lead with the value proposition, core capabilities, key differentiators, and a comparison table if competitors are known. Must be dense but readable.
- multi-pager.md: Detailed product overview covering all major features, use cases, architecture, and benefits. 4-8 pages equivalent. Suitable for prospects doing deep evaluation.
- whitepaper.md: Comprehensive technical and business document. Covers problem statement, solution architecture, features in depth, use cases, competitive landscape, and deployment. 10+ pages equivalent.

RULES:
- Write in clean markdown.
- Be factual — only include information found in research.md or that you can verify from the crawled sources.
- Do not invent features or capabilities.
- Do not add brand styling or CSS — the marketing-doc-formatter skill handles that separately.
- Write for a professional audience.
- Use clear section structure with headers.

Write the document to products/[product]/[doc-type].md."
```

Launch writers for selected documents. They may run in parallel if multiple documents are selected.

### Step 8: Report

Tell the user:
- What was crawled (number of pages, key sections found)
- Files generated and their locations
- Any gaps — areas where information was sparse or missing
- Suggest next steps (e.g., "run /marketing-doc-formatter to produce PDFs")

## Crawl Rules

These rules apply to all research agents:

- **In-scope:** The target domain and sibling/related domains (same organization). e.g., `izumanetworks.com` and `izuma.com` are both in-scope.
- **Out-of-scope:** Unrelated third-party sites (github.com, kubernetes.io, docker.com, etc.). For well-known technologies, use common knowledge instead of crawling.
- **Pacing:** Brief pauses between fetches. Do not send rapid-fire requests.
- **No interaction:** Do not submit forms, sign up for anything, or contact anyone.
- **Skip:** Login-gated content, binary downloads, video/audio content.

## Output Location

```
products/[product]/
  research.md      # comprehensive raw research
  features.md      # structured feature list (if selected)
  one-pager.md     # concise summary (if selected)
  multi-pager.md   # detailed overview (if selected)
  whitepaper.md    # comprehensive document (if selected)
```

## Notes

- This skill produces **content in markdown**. Use `/marketing-doc-formatter` separately to stylize and convert to PDF.
- The research.md file is the knowledge base. All other documents are derived from it.
- If a crawl fails or a site blocks access, report what was accessible and what wasn't. Don't silently skip content.
- Multiple agents may discover the same information — the compile step deduplicates.
- For very large sites, the crawler should prioritize: product pages, feature pages, docs, pricing, about/company pages. Blog posts and news are lower priority unless they contain product announcements.
