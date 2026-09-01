---
name: classify-incoming
description: Classify a filed slug (category, priority, repo, confidence) using Haiku.
when_to_use: When a slug.json with GHLSTATE=filed is found in any to-chopper/.
---

# classify-incoming

Reads a slug JSON, sends `description + title` to Haiku, returns a structured
`{category, priority, repo, confidence}` payload. Errors when
`confidence < limits.dedupe_classifier_min_confidence`. For multi-repo cases
(`§17`), populates `multi_repo` with a per-surface breakdown.

## Invocation

```
classify-incoming --slug-path <path> --repo-root <path>
```

## Output

JSON on stdout matching `ClassifyResult { category, priority, repo, confidence, multi_repo? }`.
