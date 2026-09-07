<!-- ADAPTED for the codeconverter pipeline from codeplanner/phase03-tests.md -->

> **Stage mapping preamble — read first.** This playbook was adapted from the
> legacy "codeplanner" process, which numbered its phases differently. In this
> pipeline you are executing **Stage 04-test-baseline**. When the text below says "Phase N",
> translate with this table:
>
> | Legacy phase | codeconverter stage |
> |---|---|
> | (service profile interview) | 01-service-profile |
> | Phase 1 | 02-codebase-analysis |
> | Phase 2 | 03-dependency-discovery |
> | Phase 3 | 04-test-baseline |
> | Phase 4 | 05-api-surface |
> | Phase 5 | 06-domain-analysis |
> | Phase 6 | 07-target-codebase |
> | Phase 7 | 08-gap-validation |
> | Phase 8 (bad actors) | 09-dependency-audit |
> | Phase 9 | 10-service-alignment |
> | Phase 10 (also titled "Phase 8 — Migration Plan") | 11-migration-plan |
>
> All file paths in this document have been rewritten to the
> `docs/codeconverter/` layout. There is no journey.md/journal in this pipeline —
> ignore any journaling instructions. Where this document conflicts with the
> stage's SKILL.md output contract (uniform headers, MANIFEST.md, output
> directory), **SKILL.md wins**.

---

# AI Coder Instructions — Phase 3: Get Every Runnable Test Suite to 100% Pass Rate

## Mission (read carefully)

The goal of this phase is to establish a verified, passing test baseline against the existing Java implementation before any Go rewrite begins. A passing test suite is your primary contract enforcement mechanism. If you cannot prove the Java service passes all its tests today, you have no ground truth to verify the Go replacement against tomorrow.

You will find every test suite, run it, fix failures that are in tests or environment, escalate failures that reveal real service bugs, and lock the results. You will not touch production service code to make a test pass. You will document everything.

You will **not** stop early. Every runnable test suite must reach 0 failures, 0 errors before this phase is complete.

---

## Prerequisites

Before starting, confirm the following are available:

```bash
# Java and Maven
java -version   # must be Java 11 or 17 (check pom.xml for source/target level)
mvn -version

# Docker (for integration test dependencies)
docker ps
docker compose version   # or 'docker-compose version'

# Python virtualenv (for system tests)
ls /workspace/recode/IAM/auth/pelion-venv/bin/pytest
ls /workspace/recode/IAM/auth/pelion-venv/bin/clitest

# gh CLI (for syncing test repo branches)
gh auth status

# The branch name the human has specified for all test repos
# Placeholder: {BRANCH_NAME} — the human will specify this (e.g., '2026Q1_update')
```

If any prerequisite is missing, stop and report it to the human before proceeding.

---

## Step 0 — Read Existing Test Documentation

Read both of these files in full before doing anything else:

- `docs/codeconverter/03-dependency-discovery/references.md` — the output of Phase 2; lists all test suite repos and their local paths
- `docs/codeconverter/04-test-baseline/tests.md` — the existing test inventory; contains run commands, counts, and last-known status

From these files, construct a working inventory table. Add a column for your current run status. Example:

| Suite | Location | Run command | Last known status | Your baseline run | Notes |
|---|---|---|---|---|---|
| Unit tests | `iam-*/src/test/` | `mvn test -pl ...` | 314 pass | TBD | |
| Integration tests | `test-integration/` | `mvn test -pl test-integration` | 2191 pass | TBD | |
| Flow tests | `test-flow/` | `mvn test -pl test-flow` | 396 pass | TBD | |
| System tests (pytest) | `/workspace/recode/IAM/pelion-system-tests/` | see docs/codeconverter/04-test-baseline/tests.md | 13/13 pass | TBD | |
| CLI system tests | `/workspace/recode/IAM/mbed-clitest-systemtest/` | see docs/codeconverter/04-test-baseline/tests.md | 18/18 pass | TBD | |

You may discover additional test suites from `docs/codeconverter/03-dependency-discovery/references.md` that are not yet listed in `docs/codeconverter/04-test-baseline/tests.md`. Add them to the table.

---

## Step 1 — Run Each Suite, Document Results

For each suite in your inventory table, follow this exact process:

### 1a. Attempt to run the suite

Run the exact command from `docs/codeconverter/04-test-baseline/tests.md` for in-repo suites. For external repos, use the run command documented in `docs/codeconverter/03-dependency-discovery/references.md`. Capture full output to a log file:

```bash
mvn test -pl {MODULE} 2>&1 | tee /tmp/test-run-{SUITE_NAME}.log
```

Do not interpret results from memory. Read the actual log.

### 1b. Record the outcome immediately

After each run, record:
- Total tests run
- Passed count
- Failed count
- Error count
- Skipped count
- Wall clock time
- Exit code of the run command

### 1c. For each failure or error, diagnose the root cause

For every failing test or error, you must classify it into exactly one of these four categories:

#### Category A: Test bug
The test itself contains incorrect logic, incorrect expectations, wrong hardcoded values, or incorrect setup/teardown. The service behavior is correct.

Evidence required: show the assertion that fails, show the actual service response, explain why the actual response is correct per the service's contract.

**Fix**: Fix the test. Do not change the service.

#### Category B: Missing config or environment issue
The test requires a config value, environment variable, file, Docker network, or service endpoint that is not present in this environment.

Evidence required: show the exact error message (e.g., `Connection refused on localhost:5432`, `Missing env var SMTP_HOST`), and show where the test reads that config.

**Fix**: Set up the missing config or environment dependency. Do not change the service.

#### Category C: Test framework or tooling issue
The test runner itself, a dependency of the test, or a test utility class has a bug or version incompatibility.

Evidence required: show the stack trace or error message from the framework, not from the service.

**Fix**: Fix or upgrade the tooling. Do not change the service.

#### Category D: Real service bug
The test correctly describes expected behavior, the expected behavior is in the service's contract (documented in phase01 output or the API spec), and the service produces the wrong result.

Evidence required: show the test expectation, show the service response, show where in the phase01 or API documentation this behavior is specified.

**Fix**: Stop. Document the bug clearly and escalate to the human. Do not fix the service yourself during Phase 3. Do not disable the test to hide the bug.

---

## Step 2 — Suite-Specific Guidance

### Unit Tests (in-repo: `iam-policies-engine`, `iam-federation`, `iam-common`, `iam-identity`, `iam-access`, `iam-policies`)

```bash
mvn test -pl iam-policies-engine,iam-federation,iam-common,iam-identity,iam-access,iam-policies
```

These tests have no external dependencies. If they fail, the cause is almost always a Category A (test bug) or a recent code change that broke something. Check recent git commits before diagnosing:

```bash
git log --oneline -10
git diff HEAD~5..HEAD -- iam-policies-engine/ iam-federation/ iam-common/
```

### Integration Tests (`test-integration/`)

These tests require Docker to be running with all service containers. Before running, verify:

```bash
# Check that the docker-compose stack is up
docker ps | grep -E 'postgres|redis|rabbitmq|s3mock|iam'

# If not running, start it
docker compose -f docker/docker-compose.yml up -d
# Wait for healthy status
docker compose -f docker/docker-compose.yml ps
```

Important notes:
- Integration tests spin up **GreenMail** as an SMTP mock. No external mail server is needed or expected. If tests fail with SMTP connection errors, the issue is that GreenMail did not start — check the GreenMail server port in the test's `BaseIntegrationTest` class.
- The test database is created fresh for each test run (or each test class). If you see leftover state from a previous run causing conflicts, truncate the test DB tables or restart the Postgres container.
- `@Disabled` tests must NOT be enabled. Document each one with the reason it was disabled (read the `@Disabled` annotation value or nearby comments). These are explicitly excluded from the pass target.
- `Assume.*` guards (e.g., `Assume.longMode()`) indicate tests that require a special flag. Document which flag each one needs. Do not force-enable them. They are correctly skipped in standard CI.

### Flow Tests (`test-flow/`)

Flow tests talk **directly to services**, bypassing the API gateway. This means:
- Check the `env.json` or equivalent config file used by the flow test runner. It must point to the direct service ports, not port 8080 (the gateway).
- The config file location is typically `test-flow/src/test/resources/env.json` or set via a system property. Read the `BaseFlowTest` class to find where host/port are read from.
- All 3 IAM service containers must be running before flow tests can execute.
- The skipped tests (`@Disabled` and `Assume.*` guards) documented in `docs/codeconverter/04-test-baseline/tests.md` are intentionally skipped. Do not enable `FederatedUserTest.federatedSignupTest` — it requires a live SAML IdP that is not available locally. Do not enable `ExternalRestApiFuzzyTest` — it requires `apigwMode=true`.

```bash
# Confirm direct service ports before running
grep -r 'port\|host\|baseUrl' test-flow/src/test/resources/ 2>/dev/null
grep -r 'apigwMode\|port\|host' test-flow/src/test/java/*/BaseFlowTest.java 2>/dev/null
```

### System Tests (Python pytest — `pelion-system-tests`)

System tests go through the **API gateway**. The gateway must be running before these tests execute.

```bash
# Verify gateway is running
curl -s http://localhost:8080/v3/accounts/me -o /dev/null -w "%{http_code}"
# Must return 401 (not connection refused)

# Run the tests
cd /workspace/recode/IAM/pelion-system-tests
/workspace/recode/IAM/auth/pelion-venv/bin/pytest --config_path=configs/localhost.json -v test_cases/iam/test_suite_iam.py
/workspace/recode/IAM/auth/pelion-venv/bin/pytest --config_path=configs/localhost.json -v test_cases/iam/multi_factor_authentication.py
/workspace/recode/IAM/auth/pelion-venv/bin/pytest --config_path=configs/localhost.json -v test_cases/iam/free_account_to_commercial.py
```

If tests fail with connection errors to the gateway, check `docker/nginx-gateway.conf` for the upstream service ports and verify they match what the services are actually listening on.

### CLI System Tests (mbed-clitest — `mbed-clitest-systemtest`)

```bash
cd /workspace/recode/IAM/mbed-clitest-systemtest
/workspace/recode/IAM/auth/pelion-venv/bin/clitest \
  --suite suites/iam.json \
  --tcdir . \
  --tc_cfg configs/localhost.json \
  --ignore_invalid_params
```

If the clitest runner itself fails (not individual test cases), check that the `mbed-clitest` and `mbed-cloud-systemtest-library` packages are installed in the venv:

```bash
/workspace/recode/IAM/auth/pelion-venv/bin/pip show mbed-clitest mbed-cloud-systemtest-library
```

---

## Step 3 — Branch Sync for Test Repos

All external test repos (those discovered in Phase 2 and cloned locally) must be on the branch `{BRANCH_NAME}`. Confirm with the human which branch name to use before executing this step.

For each external test repo:

```bash
cd /tmp/discovered/{REPO_NAME}
git status
git branch --show-current
```

If the repo is not on `{BRANCH_NAME}`:

```bash
git fetch origin
git checkout -b {BRANCH_NAME} origin/{BRANCH_NAME}
# If the branch doesn't exist yet on origin, create it from main:
git checkout -b {BRANCH_NAME}
```

If you make any fixes to test code in an external repo (Category A fixes), commit and push them to `{BRANCH_NAME}`:

```bash
git add {CHANGED_FILES}
git commit -m "Phase 3: fix test failures — {brief description of what was fixed}"
git push origin {BRANCH_NAME}
```

After pushing, re-run the full suite for that repo to confirm 0 failures before moving on.

---

## Step 4 — The Invariant: Do Not Change Production Code

This rule is absolute. If you find yourself editing any of the following to make a test pass, stop immediately and escalate to the human:

- Any file under `iam-identity/src/main/`
- Any file under `iam-access/src/main/`
- Any file under `iam-policies/src/main/`
- Any file under `iam-policies-engine/src/main/`
- Any file under `iam-federation/src/main/`
- Any file under `iam-common/src/main/`
- Any REST resource class (files matching `*Resource.java`)
- Any service class (files matching `*ServiceImpl.java`)
- Any DAO or repository class
- Any migration script under `src/main/resources/db/`

Edits are only permitted in:
- `*/src/test/java/` directories (test source)
- `*/src/test/resources/` directories (test config)
- External test repos cloned under `/tmp/discovered/`
- Docker compose files and environment config files, only if they were misconfigured (e.g., wrong port number, missing env var with no business logic impact)

---

## Step 5 — Update Documentation

After all suites pass, update `docs/codeconverter/04-test-baseline/tests.md` with the final counts from your runs. Use the actual numbers from your terminal output, not the numbers from the previous version of the file.

Add a section at the top of `docs/codeconverter/04-test-baseline/tests.md`:

```markdown
## Phase 3 Baseline Run — {DATE}

All suites run to 0 failures, 0 errors. See summary table below.
Branch: {BRANCH_NAME}
Runner: {your environment description}
```

Then update each suite entry with the new counts.

Also update `docs/codeconverter/STATE.md` (if it exists) with a summary of what Phase 3 accomplished and what Phase 4 should start with.

---

## Output: Required Summary Table

At the end of Phase 3, produce this table in your output (also include it in `docs/codeconverter/04-test-baseline/tests.md`):

| Suite | Location | Run command | Test count | Pass | Fail | Error | Skip | Branch | Notes |
|---|---|---|---|---|---|---|---|---|---|
| Unit | `iam-*/src/test/` | `mvn test -pl ...` | 314 | 314 | 0 | 0 | 0 | 2026Q1_update | |
| Integration | `test-integration/` | `mvn test -pl test-integration` | 2191 | 2191 | 0 | 0 | 39 | 2026Q1_update | 39 @Disabled |
| Flow | `test-flow/` | `mvn test -pl test-flow` | 396 | 396 | 0 | 0 | 5 | 2026Q1_update | 5 skipped: see tests.md |
| System (pytest) | `/workspace/recode/IAM/pelion-system-tests/` | `pytest ...` | 19 | 19 | 0 | 0 | 0 | {BRANCH_NAME} | |
| CLI system | `/workspace/recode/IAM/mbed-clitest-systemtest/` | `clitest ...` | 18 | 18 | 0 | 0 | 0 | {BRANCH_NAME} | |

Fill in actual numbers. Do not use the placeholder numbers above as final results.

---

## Skipped Test Documentation

For every skipped test (`@Disabled` or `Assume.*` guarded), document it in a table:

| Suite | Test class | Test method | Skip mechanism | Reason / required condition | Can it ever run locally? |
|---|---|---|---|---|---|
| Flow | `FederatedUserTest` | `federatedSignupTest` | `@Disabled` | Needs live SAML IdP | NO — requires external IdP |
| Flow | `ExternalRestApiFuzzyTest` | (whole class) | `@Disabled` | Needs `apigwMode=true` | YES, with gateway running |
| Flow | `FeatureSwitchTest` | (1 method) | `Assume.longMode()` | Needs `-Diam.test.long` | YES, add flag to mvn command |
| Integration | ... | ... | `@Disabled` | ... | ... |

For tests where `Can it ever run locally?` is YES but they are currently skipped, document the exact steps to enable them and note them as optional/out-of-scope for Phase 3. Do not enable them now unless the human explicitly approves.

---

## Exit Criteria

You may declare Phase 3 complete only when ALL of the following are true:

1. Every test suite in your inventory table has been run at least once since you began Phase 3 (not relying on previous run results).
2. Every suite shows 0 failures and 0 errors in its most recent run.
3. Every skipped test is documented in the skipped test table with a reason.
4. No Category D (real service bug) is unresolved — either it has been escalated and the human has given a disposition, or it does not exist.
5. All external test repos are on branch `{BRANCH_NAME}`. All fixes committed to that branch and pushed.
6. `docs/codeconverter/04-test-baseline/tests.md` is updated with the final counts and the Phase 3 baseline run header.
7. `docs/codeconverter/STATE.md` is updated.
8. The summary table above is complete and accurate.

If any suite produces a failure during Phase 3 exit verification, you are not done.
