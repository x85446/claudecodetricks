---
name: dedupe-against-open
description: Jaccard/keyword similarity scan over open + closed bugs to detect duplicates or regressions.
when_to_use: After classify-incoming succeeds, before issuing BUG-NNNNNN.
---

# dedupe-against-open

Walks `bugs/`, `bugs/_blocked/`, `bugs/closed/`. Computes a similarity score
between the slug and every existing bug. An OPEN match short-circuits
acceptance (slug stays as a comment on the open bug). A CLOSED match becomes a
regression — caller fills `regression_of: BUG-OLD` on the new bug.

## Invocation

```
dedupe-against-open --slug-path <path> --repo-root <path>
```

## Output

JSON `DedupeResult { match_type: open|closed|none, match_id?: BUG-NNNNNN }`.
