# Oracle — claudecodetricks

Last updated: 2026-08-27T23:34:22Z

## Index
- autonoma

---

## autonoma

**Who:** Maintained by the Autonoma-AI org on GitHub (~188 stars, 47 forks). Not a Travis project — external prior art.

**What:** An agentic end-to-end testing platform. Its own tagline: "Open-source testing platform where AI agents navigate your app end-to-end and catch regressions on every PR. No test code required." It reads the codebase, generates natural-language tests, and drives a real browser against each PR's preview deployment, reporting failures with screenshots, video, and a suspected code culprit.

**When:** Reference it when comparing approaches to test generation and regression detection — especially against TESTMASTER. Relevant to `/testmaster-derive` (deriving cases from intent) and to the drift problem. Do NOT reach for it as a FOSS test-case-management option: see Why.

**Where:**
- Repo: https://github.com/autonoma-ai/autonoma
- Related skills: [[testmaster]], [[testmaster-derive]], [[testmaster-catalog]]

**Why:** It attacks the same problem TESTMASTER does from the opposite end. TESTMASTER indexes and validates the tests you already have (requirement → cases → covered code, with drift computed from `git diff ∩ covers`). Autonoma writes and runs tests for you and skips the index entirely. Useful contrast, not a substitute.

**Licensing gotcha — it is not FOSS.** Business Source License 1.1, converting to Apache 2.0 on **2028-03-23**. BUSL is source-available with a production-use restriction, not an OSI open-source licence, despite the repo describing itself as open source. If the requirement is "completely FOSS," this does not qualify today. [[kiwi-tcms]] does (GPLv2).

**How:**
```bash
git clone https://github.com/Autonoma-AI/autonoma.git
cd autonoma && pnpm install
docker compose up -d
pnpm db:generate && pnpm db:migrate
pnpm dev            # API :4000, UI :3000
```
Stack: Node 24, TypeScript, pnpm/Turborepo, React 19, Vite, Postgres + Prisma, Redis, Hono, tRPC, Playwright (web) / Appium (mobile), Temporal for orchestration, Gemini/Groq/OpenRouter as model backends. Self-hosting means standing up Postgres, Redis, Temporal, and supplying your own model API keys — not a light dependency.
