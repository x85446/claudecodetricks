# Smug Judge — 20-Category Scoring Framework

This framework defines all 20 audit categories organized into 3 tiers. Sub-dimensions adapt based on detected tech stack.

---

## Tech Stack Adaptation Rules

Before assigning sub-dimensions, classify the project:

| Signal | Classification |
|--------|---------------|
| `go.mod`, `*.go` | Go backend |
| `package.json` + `*.tsx`/`*.jsx` | JavaScript/TypeScript frontend |
| `package.json` + `express`/`fastify`/`nest` | Node.js backend |
| `requirements.txt`/`pyproject.toml` + `*.py` | Python |
| `Cargo.toml` + `*.rs` | Rust |
| Both `go.mod` AND `package.json` | Full-stack (split sub-dimensions) |
| `Dockerfile` + `helm/` + CI config | Has infrastructure layer |

When a sub-dimension references a technology not present in the stack, replace it with the stack-appropriate equivalent. For example:
- Go project: "React component composition" becomes "Package composition and interface design"
- Python project: "TypeScript strict mode" becomes "Type hint coverage (mypy/pyright)"
- Frontend project: "SQL injection prevention" becomes "XSS and DOM injection prevention"

If a category is genuinely not applicable (e.g., "OAuth" for a CLI tool with no auth), score it at the project's average and note "N/A — Not applicable to project type" in the report.

---

## Tier 1: Core Quality (9 categories, 55% weight)

### 01 — Security Posture (10%)

Audits the project's defense against common attack vectors.

| # | Sub-dimension | Weight | What to Audit |
|---|---------------|--------|---------------|
| a | Authentication & credential handling | 25% | Password/token/key lifecycle, secret storage, credential transmission |
| b | Input validation & injection prevention | 25% | Schema validation, sanitization, SQL/XSS/command injection, parameterized queries |
| c | Authorization & access control | 20% | RBAC/ABAC, middleware coverage, privilege escalation, endpoint protection |
| d | Secret management | 15% | Hardcoded secrets, env vars, .gitignore coverage, secret rotation |
| e | Transport security | 15% | HTTPS enforcement, security headers, cookie attributes, CORS |

### 02 — Error Handling & Resilience (8%)

Audits how gracefully the system handles failure.

| # | Sub-dimension | Weight | What to Audit |
|---|---------------|--------|---------------|
| a | Error propagation patterns | 25% | Consistent error types, error wrapping, stack traces, panic recovery |
| b | User-facing error messages | 25% | Generic messages, no info leakage, helpful without exposing internals |
| c | Graceful degradation | 20% | Fallbacks, circuit breakers, timeout handling, retry logic |
| d | Error boundaries & recovery | 15% | Error boundaries (React), recover() (Go), try/except (Python), process isolation |
| e | Logging vs display separation | 15% | Detailed server logs, generic client messages, structured logging |

### 03 — Data Integrity & Validation (7%)

Audits data correctness throughout the system.

| # | Sub-dimension | Weight | What to Audit |
|---|---------------|--------|---------------|
| a | Input schema validation | 25% | Validation library usage, schema completeness, type coercion |
| b | Data model correctness | 25% | Database constraints, foreign keys, migrations, data types |
| c | API contract enforcement | 20% | Request/response validation, versioning, backwards compatibility |
| d | State management correctness | 15% | Race conditions, stale data, optimistic updates, cache invalidation |
| e | Boundary validation | 15% | Null checks, array bounds, integer overflow, string length limits |

### 04 — Architecture & Design (7%)

Audits structural quality and design decisions.

| # | Sub-dimension | Weight | What to Audit |
|---|---------------|--------|---------------|
| a | Separation of concerns | 25% | Layer boundaries, business logic isolation, UI/logic separation |
| b | Dependency management | 25% | Dependency direction, circular imports, coupling, DI patterns |
| c | Component/module composition | 20% | File sizes, single responsibility, reusability, interface design |
| d | Directory organization | 15% | Clear structure, convention adherence, co-location patterns |
| e | Scalability design | 15% | Statelessness, horizontal scaling readiness, bottleneck identification |

### 05 — Testing & Quality Assurance (6%)

Audits test presence, quality, and coverage.

| # | Sub-dimension | Weight | What to Audit |
|---|---------------|--------|---------------|
| a | Unit test coverage | 25% | Test file presence, critical path coverage, assertion quality |
| b | Integration tests | 20% | Cross-component tests, API tests, database tests |
| c | E2E / acceptance tests | 20% | Full-flow tests, browser tests, CLI tests |
| d | Test infrastructure | 15% | Test runner config, fixtures, mocks, CI integration |
| e | Coverage metrics & enforcement | 20% | Coverage thresholds, coverage reporting, branch coverage |

### 06 — API Design & Contracts (5%)

Audits API quality, consistency, and documentation.

| # | Sub-dimension | Weight | What to Audit |
|---|---------------|--------|---------------|
| a | Endpoint design consistency | 25% | RESTful conventions, naming, HTTP method usage, status codes |
| b | Request/response schemas | 25% | Typed contracts, OpenAPI/Swagger, protobuf definitions |
| c | Versioning strategy | 20% | URL vs header versioning, backwards compatibility, deprecation |
| d | Error response format | 15% | Consistent error shape, error codes, machine-readable errors |
| e | Documentation completeness | 15% | API docs, examples, authentication docs |

### 07 — Dependency Health (5%)

Audits third-party dependency management.

| # | Sub-dimension | Weight | What to Audit |
|---|---------------|--------|---------------|
| a | Vulnerability scanning | 25% | Known CVEs, audit tools (npm audit, govulncheck, safety) |
| b | Version pinning & lockfiles | 25% | Lock file presence, exact vs range versions, reproducible builds |
| c | Dependency freshness | 20% | Outdated packages, major version lag, abandoned dependencies |
| d | Dependency count & bloat | 15% | Unused dependencies, bundle size impact, transitive deps |
| e | License compliance | 15% | License compatibility, copyleft risks, license audit |

### 08 — Concurrency & Resource Safety (4%)

Audits thread safety, resource management, and race conditions.

| # | Sub-dimension | Weight | What to Audit |
|---|---------------|--------|---------------|
| a | Thread/goroutine safety | 25% | Mutex usage, channel patterns, async/await correctness |
| b | Resource lifecycle | 25% | File handles, DB connections, socket cleanup, defer/finally |
| c | Race condition prevention | 20% | Data races, TOCTOU, atomic operations, -race flag |
| d | Context propagation & cancellation | 15% | Context passing, timeout propagation, cancellation handling |
| e | Memory management | 15% | Leak prevention, buffer management, GC pressure, pool usage |

### 09 — Configuration & Environment (3%)

Audits configuration management and environment handling.

| # | Sub-dimension | Weight | What to Audit |
|---|---------------|--------|---------------|
| a | Config validation | 25% | Startup validation, required field checks, type safety |
| b | Environment separation | 25% | Dev/staging/prod configs, env var management, .env handling |
| c | Secret injection patterns | 20% | No hardcoded secrets, secure config loading, secret rotation |
| d | Default value safety | 15% | Safe defaults, fail-closed, no debug mode in production |
| e | Config documentation | 15% | README config section, .env.example, required vars documented |

---

## Tier 2: Production Readiness (6 categories, 28% weight)

### 10 — CI/CD & Build Pipeline (6%)

Audits build, test, and deployment automation.

| # | Sub-dimension | Weight | What to Audit |
|---|---------------|--------|---------------|
| a | Build configuration | 25% | Build strictness, reproducible builds, build warnings as errors |
| b | CI pipeline completeness | 25% | Lint, type-check, test, build stages; PR checks |
| c | Deployment automation | 20% | Deploy scripts, rollback strategy, blue/green, canary |
| d | Container security | 15% | Multi-stage builds, non-root user, minimal base image, no secrets |
| e | Pipeline performance | 15% | Build times, caching, parallelization, artifact management |

### 11 — Documentation & Discoverability (5%)

Audits how easy it is for new developers to understand the project.

| # | Sub-dimension | Weight | What to Audit |
|---|---------------|--------|---------------|
| a | README completeness | 25% | Setup instructions, architecture overview, contribution guide |
| b | Code documentation | 25% | Doc comments (GoDoc/JSDoc/docstrings), complex logic explained |
| c | API documentation | 20% | Endpoint docs, request/response examples, auth docs |
| d | Architecture docs | 15% | ADRs, system diagrams, data flow docs |
| e | Onboarding experience | 15% | Time to first build, dev environment setup, CLAUDE.md |

### 12 — Performance & Efficiency (5%)

Audits runtime performance characteristics.

| # | Sub-dimension | Weight | What to Audit |
|---|---------------|--------|---------------|
| a | Algorithmic efficiency | 25% | Big-O awareness, unnecessary iterations, N+1 queries |
| b | Resource utilization | 25% | Memory usage, connection pooling, caching strategy |
| c | Bundle/binary size | 20% | Tree shaking, dead code elimination, binary stripping |
| d | Rendering/response performance | 15% | Core Web Vitals, API latency, SSR/SSG, lazy loading |
| e | Profiling & benchmarks | 15% | Benchmark presence, profiling infrastructure, perf regression tests |

### 13 — Observability & Monitoring (4%)

Audits production visibility.

| # | Sub-dimension | Weight | What to Audit |
|---|---------------|--------|---------------|
| a | Structured logging | 25% | Log format, log levels, correlation IDs, JSON logging |
| b | Metrics & instrumentation | 25% | Prometheus/StatsD/custom metrics, SLI tracking |
| c | Health checks & probes | 20% | Liveness, readiness, startup probes, dependency health |
| d | Distributed tracing | 15% | Trace propagation, span creation, trace sampling |
| e | Alerting readiness | 15% | Alert rules, runbooks, on-call documentation |

### 14 — Deployment & Infrastructure (4%)

Audits infrastructure-as-code and deployment readiness.

| # | Sub-dimension | Weight | What to Audit |
|---|---------------|--------|---------------|
| a | Infrastructure definition | 25% | IaC (Terraform/Pulumi/Helm), reproducible environments |
| b | Environment parity | 25% | Dev/staging/prod consistency, feature flags, config drift |
| c | Scaling configuration | 20% | HPA, resource limits, auto-scaling policies |
| d | Disaster recovery | 15% | Backup strategy, restore procedures, RTO/RPO |
| e | Security hardening | 15% | Network policies, RBAC, image scanning, secrets management |

### 15 — Data & Privacy (4%)

Audits data handling and privacy compliance.

| # | Sub-dimension | Weight | What to Audit |
|---|---------------|--------|---------------|
| a | PII handling | 25% | Data classification, encryption at rest/transit, masking |
| b | Data lifecycle | 25% | Retention policies, deletion capabilities, archival |
| c | Privacy compliance | 20% | GDPR/CCPA readiness, consent management, data export |
| d | Audit trail | 15% | Action logging, immutable logs, user activity tracking |
| e | Data minimization | 15% | Collect only what's needed, no unnecessary PII storage |

---

## Tier 3: Code Excellence (5 categories, 17% weight)

### 16 — Type Safety & Language Idioms (5%)

Audits language-specific best practices.

| # | Sub-dimension | Weight | What to Audit |
|---|---------------|--------|---------------|
| a | Type system utilization | 25% | Strict mode, type coverage, no `any`/`interface{}` abuse |
| b | Idiomatic patterns | 25% | Language conventions, standard library usage, community patterns |
| c | Static analysis compliance | 20% | Linter config, zero warnings policy, custom rules |
| d | Error/null handling patterns | 15% | Optional types, nil checks, exhaustive matching |
| e | Build-time enforcement | 15% | Compiler strictness, CI type checking, no suppressed errors |

### 17 — Code Quality & Standards (4%)

Audits code cleanliness and consistency.

| # | Sub-dimension | Weight | What to Audit |
|---|---------------|--------|---------------|
| a | Naming conventions | 25% | Consistent casing, descriptive names, no abbreviations |
| b | DRY compliance | 20% | Code duplication, shared utilities, abstraction appropriateness |
| c | Dead code elimination | 20% | Unused imports, commented-out code, unreachable branches |
| d | Formatting consistency | 20% | Auto-formatter config, consistent style, editor config |
| e | Code complexity | 15% | Cyclomatic complexity, function length, nesting depth |

### 18 — Accessibility (3%)

Audits accessibility compliance (primarily for projects with UI).

| # | Sub-dimension | Weight | What to Audit |
|---|---------------|--------|---------------|
| a | Semantic markup | 25% | Proper HTML elements, ARIA roles, landmarks |
| b | Form accessibility | 25% | Labels, error announcements, focus management |
| c | Keyboard navigation | 20% | Tab order, focus trapping, keyboard shortcuts |
| d | Screen reader support | 15% | Live regions, alt text, aria-describedby |
| e | Color & contrast | 15% | WCAG AA contrast, not relying on color alone |

**For non-UI projects:** Replace with "CLI Usability" — help text quality, exit codes, stdin/stdout conventions, error output formatting, man page/docs.

### 19 — Internationalization Readiness (3%)

Audits i18n/l10n preparedness.

| # | Sub-dimension | Weight | What to Audit |
|---|---------------|--------|---------------|
| a | String externalization | 30% | Hardcoded strings, translation file structure, key naming |
| b | Locale-aware formatting | 20% | Dates, numbers, currency, phone numbers |
| c | RTL/bidirectional support | 15% | Logical properties, text direction handling |
| d | Error message localization | 20% | Translatable errors, parameterized messages |
| e | Content pluralization | 15% | Plural rules, ICU message format, gender handling |

**For non-user-facing projects:** Replace with "Extensibility" — plugin architecture, hook points, configuration extensibility, API evolution readiness, backward compatibility.

### 20 — AI-Friendly Development (2%)

Audits how well the codebase works with AI coding assistants.

| # | Sub-dimension | Weight | What to Audit |
|---|---------------|--------|---------------|
| a | Code discoverability | 20% | Clear file naming, CLAUDE.md, entry points obvious |
| b | Intent clarity | 25% | Self-documenting code, no clever tricks, readable control flow |
| c | Safe editability | 20% | Changes in one file don't break distant files, loose coupling |
| d | Pattern consistency | 20% | Same patterns everywhere, predictable structure |
| e | Test guardrails | 15% | Tests catch regressions from AI-generated changes |

---

## Scoring Methodology

### Per-Category Scoring

1. Score each sub-dimension 0-10
2. Raw Score = SUM(sub_score x sub_weight) x 10 → yields 0-100
3. Apply caps:
   - Any P0 finding → category capped at 70
   - More than 3 P1 findings → category capped at 85
4. Final Category Score = min(Raw Score, applicable cap)

### Health Score

```
Health Score = SUM(Category_Score x Category_Weight) across all 20 categories
```

### Grade Scale

| Grade | Range | Grade | Range |
|-------|-------|-------|-------|
| A+ | 97-100 | C+ | 76-79 |
| A | 93-96 | C | 73-75 |
| A- | 90-92 | C- | 70-72 |
| B+ | 87-89 | D+ | 65-69 |
| B | 83-86 | D | 60-64 |
| B- | 80-82 | F+ | 50-59 |
| | | F | 0-49 |

### Priority Definitions

| Priority | Label | Definition | Score Impact |
|----------|-------|-----------|--------------|
| P0 | Critical | Exploitable vulnerability, complete feature failure, data loss risk. Must fix before production. | Caps category at 70 |
| P1 | Warning | Security weakness, significant gap, reliability risk. Fix within 1-2 sprints. | -1 to -3 per finding on sub-dimension |
| P2 | Info | Best-practice gap, defense-in-depth item, improvement opportunity. | -0.5 to -1 per finding |

### Effort Scale (1 cycle ~ 2-4 hours)

| Label | Cycles |
|-------|--------|
| Trivial | 0.1-0.25 |
| Quick | 0.25-0.5 |
| Standard | 1-2 |
| Large | 3-5 |
| Epic | 5+ |

---

## Auditor Category Assignments

| Agent | Categories | Focus |
|-------|-----------|-------|
| auditor-1 | 01, 02, 03, 04 | Security, Error Handling, Data Integrity, Architecture |
| auditor-2 | 05, 06, 07, 08 | Testing, API Design, Dependencies, Concurrency |
| auditor-3 | 09, 10, 11, 12 | Config, CI/CD, Documentation, Performance |
| auditor-4 | 13, 14, 15, 16 | Observability, Deployment, Privacy, Type Safety |
| auditor-5 | 17, 18, 19, 20 | Code Quality, Accessibility, i18n, AI-Friendly |

Each auditor writes ONLY their 4 assigned categories. No cross-writing.
