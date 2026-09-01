# Smug Judge — Report Templates

All auditors and the lead PM use these templates for consistent output.

---

## Category Report Template

Each auditor writes one file per category: `{NN}-{category-slug}.md`

File naming convention:
- `01-security-posture.md`
- `02-error-handling-resilience.md`
- `03-data-integrity-validation.md`
- `04-architecture-design.md`
- `05-testing-quality-assurance.md`
- `06-api-design-contracts.md`
- `07-dependency-health.md`
- `08-concurrency-resource-safety.md`
- `09-configuration-environment.md`
- `10-cicd-build-pipeline.md`
- `11-documentation-discoverability.md`
- `12-performance-efficiency.md`
- `13-observability-monitoring.md`
- `14-deployment-infrastructure.md`
- `15-data-privacy.md`
- `16-type-safety-language-idioms.md`
- `17-code-quality-standards.md`
- `18-accessibility.md`
- `19-i18n-readiness.md`
- `20-ai-friendly-development.md`

```markdown
# {NN} {Category Name} Audit — Round {N}

Date: {date} | Source: {project-name}

---

## Grade: {Letter} ({Score}/100)

| Sub-dimension | Weight | Score (0-10) | Weighted | Evidence |
|---------------|--------|-------------|----------|----------|
| {name} | {W}% | {N} | {N.NN} | {file:line reference + brief explanation} |
| {name} | {W}% | {N} | {N.NN} | {file:line reference} |
| {name} | {W}% | {N} | {N.NN} | {file:line reference} |
| {name} | {W}% | {N} | {N.NN} | {file:line reference} |
| {name} | {W}% | {N} | {N.NN} | {file:line reference} |
| **Category Total** | **100%** | | **{sum} x 10 = {score}** | |

{If P0 cap applied: "P0 cap applied: score capped at 70, raw score was {raw}."}
{If P1 cap applied: "P1 cap applied: score capped at 85, raw score was {raw}."}

## Summary
{2-3 paragraph narrative describing what was found, what's good, what's bad.
Professional tone — no smugness in category reports. Evidence-based.}

## P0 Findings (Critical)

| # | Finding | File(s) | OWASP/CWE | Impact | Effort |
|---|---------|---------|-----------|--------|--------|
| P0-01 | **{Bold finding title}** — {detailed description with code reference} | `{file:line}` | {reference if applicable} | {impact description} | {effort} |

{If none: "None."}

## P1 Findings (Warning)

| # | Finding | File(s) | Impact | Effort |
|---|---------|---------|--------|--------|
| P1-01 | {description} | `{file:line}` | {impact} | {effort} |

{If none: "None."}

## P2 Findings (Info)

| # | Finding | File(s) | Impact | Effort |
|---|---------|---------|--------|--------|
| P2-01 | {description} | `{file:line}` | {impact} | {effort} |

{If none: "None."}

## Compliant Items
{Numbered list of things done right — give genuine credit.}

## Recommendations

| Priority | Recommendation | Effort | Expected Score Impact |
|----------|---------------|--------|----------------------|
| P0 | {what to do} | {effort} | +{N} points |
| P1 | {what to do} | {effort} | +{N} points |

## Scoring Rules
- P0 cap: If ANY P0 exists, category score cannot exceed 70
- P1 cap: If >3 P1s exist, category score cannot exceed 85
- Score = (weighted sub-dimension sum) x 10, capped by P0/P1 rules
```

### Delta Columns (Round 2+)

When ROUND > 1, add these columns to the score table:

```markdown
| Sub-dimension | Weight | R{N-1} | R{N} | Delta | Weighted | Evidence |
```

And add these sections:
- **Items Resolved Since Round {N-1}**
- **Regressions Detected**

---

## Executive Summary Template

Written by the lead PM. File: `00-executive-summary.md`

```markdown
# {Project Name} Code Audit — Executive Summary (Round {N})

Date: {date} | Source: {project-name} ({tech stack summary})
Round: {N} {("Baseline") if N=1}

---

## Health Score: {Grade} ({Score}/100)

{One-liner smug commentary on the overall score.}

| # | Category | Tier | Weight | R{N} Score | R{N} Grade | R{N} Weighted | P0s | P1s | P2s |
|---|----------|------|--------|------------|------------|---------------|-----|-----|-----|
| 01 | [Security Posture](./01-security-posture.md) | 1-Core | 10% | {score} | {grade} | {weighted} | {n} | {n} | {n} |
| ... | | | | | | | | | |
| 20 | [AI-Friendly Dev](./20-ai-friendly-development.md) | 3-Excellence | 2% | {score} | {grade} | {weighted} | {n} | {n} | {n} |

**Important:** Every category name in column 2 MUST be a relative markdown link to its report file. Use the format `[Category Name](./NN-category-slug.md)` for all 20 rows. The filenames are listed in the Category Report Template section above.
| | **Weighted Total** | | **100%** | | **{grade}** | **{total}** | **{n}** | **{n}** | **{n}** |

---

## Tier Summary

### Tier 1: Core Quality (9 categories, 55% weight) — Score: {X}/{55} ({pct}%)
{Smug narrative analysis. 2-3 sentences. What's working, what's catastrophic.}

### Tier 2: Production Readiness (6 categories, 28% weight) — Score: {X}/{28} ({pct}%)
{Smug narrative analysis.}

### Tier 3: Code Excellence (5 categories, 17% weight) — Score: {X}/{17} ({pct}%)
{Smug narrative analysis.}

---

## Consolidated P0 Findings (Critical — Must Fix Before Production)

{Deduplicate across all 20 reports. Group by root cause.}

| # | Root Cause | Reports | OWASP/CWE | Impact | Effort |
|---|-----------|---------|-----------|--------|--------|
| 1 | **{bold root cause}** | {cat numbers} | {ref} | {impact} | {effort} |

---

## Consolidated P1 Findings (Warning — Fix Within 1-2 Sprints)

{N} raw P1 findings deduplicate to approximately {M} unique issues. Top clusters:

1. {finding}
2. {finding}
...

---

## Strengths

{Give genuine credit, but smugly. Numbered list.}

1. **{Strength}** — {acknowledgment with a hint of surprise}
...

---

## Compiler Validation Notes

{Report compiler findings: math discrepancies, duplicate findings, validation failures.}

---

## Score Trajectory

| Round | Date | Health Score | Grade | P0s | P1s | P2s | Key Change |
|-------|------|-------------|-------|-----|-----|-----|------------|
| {N} | {date} | {score} | {grade} | {n} | {n} | {n} | {description} |

**Target: {next grade} ({score}+).** {What it would take to get there.}

---

## Audit Reports Index

| # | Report | Grade | Score | P0s | P1s | P2s |
|---|--------|-------|-------|-----|-----|-----|
| 01 | [{Category Name}](./{filename}) | {grade} | {score} | {n} | {n} | {n} |
...

---

_Generated by The Smug Judge — {N} categories, 3 tiers, {date}_
```

---

## Remediation Plan Template

Written by the lead PM. File: `99-remediation-plan.md`

```markdown
# {Project Name} Remediation Plan — Round {N}

Date: {date} | Baseline Score: {Grade} ({Score}/100)

---

## Sprint Summary

| Sprint | Focus | Effort | Expected Score Impact | Target Score |
|--------|-------|--------|----------------------|--------------|
| 1 | {quick wins} | {N} cycles | +{N} pts | ~{score} |
| ... | | | | |
| {last} | {final polish} | {N} cycles | +{N} pts | ~{score} |
| **Total** | | **~{N} cycles** | **+{N} pts** | **~{score} ({grade})** |

---

## Sprint {N}: {Title} ({effort} cycles)

**Rationale:** {Why this sprint first. Highest impact/effort ratio explanation.}

| # | Category | Finding | File(s) | What to Do | Effort |
|---|----------|---------|---------|-----------|--------|
| {N.N} | {NN Cat} | {finding} | `{file}` | {action} | {effort} |

**Expected impact:** Cat {NN} (+{N}), Cat {NN} (+{N}). Weighted: ~+{N} points.

{Repeat for each sprint...}

---

## File Ownership Map (for Parallel Remediation)

| File | Primary Changes | Suggested Owner |
|------|----------------|-----------------|
| `{file}` | {changes} | fixer-{N} |

---

## Expected Score Trajectory

| After Sprint | Estimated Score | Grade | Key Unlocks |
|-------------|----------------|-------|-------------|
| 0 (baseline) | {score} | {grade} | — |
| 1 | ~{score} | {grade} | {what improves} |
| ... | | | |

---

_Generated from R{N} audit findings — {date}_
```

---

## Score Tracker Template

Written by the lead PM. File: `audits/{project-name}/score-tracker.md` (NOT inside R{N}/)

```markdown
# {Project Name} — Score Tracker

Cross-round score history.

---

## Overall Health Score

| Round | Date | Health Score | Grade | P0s | P1s | P2s | Delta | Key Change |
|-------|------|-------------|-------|-----|-----|-----|-------|------------|
| {N} | {date} | {score} | {grade} | {n} | {n} | {n} | {delta or —} | {description} |

---

## Per-Category Scores

| # | Category | Weight | R1 Score | R1 Grade | {R2 Score | R2 Grade | R2 Delta |} |
|---|----------|--------|----------|----------|
| 01 | [Security Posture](./R{latest}/01-security-posture.md) | 10% | {score} | {grade} |
| ... | | | | |
| 20 | [AI-Friendly Dev](./R{latest}/20-ai-friendly-development.md) | 2% | {score} | {grade} |

**Important:** Every category name MUST be a relative markdown link to the report file in the latest round directory. Use `[Category Name](./R{latest}/NN-category-slug.md)` where `{latest}` is the most recent round number. List all 20 rows.

---

## Tier Summary

| Tier | Weight | R{N} Weighted | R{N} % of Max |
|------|--------|-------------|-------------|
| 1: Core Quality (01-09) | 55% | {weighted} | {pct}% |
| 2: Production Readiness (10-15) | 28% | {weighted} | {pct}% |
| 3: Code Excellence (16-20) | 17% | {weighted} | {pct}% |
| **Total** | **100%** | **{total}** | **{pct}%** |

---

## P0 Lifecycle

| P0 Root Cause | Opened | Closed | Rounds Open |
|--------------|--------|--------|-------------|
| {cause} | R{N} | {R{M} or —} | {count}+ |

---

_Updated: {date} — Round {N} {baseline or update}_
```
