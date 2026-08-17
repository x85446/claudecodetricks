<!-- ADAPTED for the codeconverter pipeline from codeplanner/phase02-references.md -->

> **Stage mapping preamble — read first.** This playbook was adapted from the
> legacy "codeplanner" process, which numbered its phases differently. In this
> pipeline you are executing **Stage 03-dependency-discovery**. When the text below says "Phase N",
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

# AI Coder Instructions — Phase 2: Discover All Dependent Repositories

## Mission (read carefully)

Before you rewrite a single line of this service in Go, you must know every external system that calls it, tests it, borrows code from it, or deploys it. Hidden dependencies discovered after a rewrite are catastrophic. This phase eliminates that risk.

You will scan all GitHub organizations for repos that have any relationship to this codebase. You will iterate until no "unknown" repos remain. You will produce a structured, verified reference document.

You will **not** stop early. You will iterate at least 3 full rounds of discovery. Each round must expand the search based on what the previous round found.

---

## Prerequisites

You need the following before starting:

- GitHub CLI (`gh`) authenticated and working. Verify with: `gh auth status`
- The names of the GitHub organizations to scan. The human will provide these. In all instructions below, substitute the actual org names wherever you see `{ORG1}` and `{ORG2}` (add more as needed for each org provided).
- The name of this service. Check `pom.xml` for `<artifactId>` and `<groupId>`. Note the Java package root (e.g., `com.arm.mbed.cloud.iam`). You will grep for these strings in all discovered repos.

---

## Step 0 — Establish Baseline Identity of This Repo

Before scanning other repos, collect the identity markers of this codebase. You will use these as search terms.

Run the following and record all results in a working notes section (not the final output yet):

```bash
# Get the repo name and org
gh repo view --json name,owner,description,url

# Get the Java package root and artifact identifiers from pom.xml
grep -r '<groupId>\|<artifactId>' pom.xml | head -30

# Get all public class names that might be borrowed
find . -name '*.java' | xargs grep '^public class ' | sed 's/.*public class //' | sed 's/ .*//' | sort -u > /tmp/class_names.txt
wc -l /tmp/class_names.txt

# Get all REST resource paths declared in this repo
grep -r '@Path\|@GET\|@POST\|@PUT\|@DELETE\|@PATCH' --include='*.java' -h | grep -oP '"/[^"]*"' | sort -u

# Get the service name as it appears in configs, Docker, and deployment manifests
grep -r 'service.*name\|container.*name\|app.*name' docker/ k8s/ helm/ deploy/ --include='*.yaml' --include='*.yml' --include='*.json' -h 2>/dev/null | head -40
```

Record:
- `THIS_REPO_NAME`: the GitHub repo name (e.g., `iam`)
- `THIS_ORG`: the GitHub org that owns it (e.g., `PelionIoT`)
- `THIS_PACKAGE_ROOT`: the Java package root (e.g., `com.arm.mbed.cloud.iam`)
- `THIS_ARTIFACT_IDS`: all `<artifactId>` values from the root and module poms
- `THIS_SERVICE_URL_PATHS`: the top-level API paths this service serves (e.g., `/v3/`, `/auth/`, `/internal/v1/`)
- `THIS_SERVICE_HOSTNAMES`: any service hostnames/ports referenced in configs (e.g., `iam-service`, `localhost:8080`)

You will grep for ALL of these in every discovered repo.

---

## Round 1 — Full Organization Scan

### Step 1.1 — List all repos in each org

```bash
# List all repos in ORG1 (paginate — do not stop at 30)
gh repo list {ORG1} --limit 1000 --json name,description,language,url,isArchived,isFork,updatedAt \
  > /tmp/org1_repos.json

# Repeat for ORG2
gh repo list {ORG2} --limit 1000 --json name,description,language,url,isArchived,isFork,updatedAt \
  > /tmp/org2_repos.json

# Count
jq 'length' /tmp/org1_repos.json
jq 'length' /tmp/org2_repos.json
```

If a repo count seems too low (e.g., you expect 200 repos but get 30), increase `--limit` or check for pagination. Use `gh api` with cursor-based pagination if needed:

```bash
gh api --paginate '/orgs/{ORG1}/repos?per_page=100' \
  --jq '.[] | {name: .name, description: .description, language: .language, url: .html_url, archived: .archived}' \
  > /tmp/org1_repos_full.json
```

### Step 1.2 — For each repo, determine relationship

For each repo in the combined list, you must check all five relationship types below. Do NOT rely on repo names or descriptions alone — they are often misleading. Check the actual code.

#### Check 1: Is it a test suite that calls this service?

```bash
# Clone a sample of repos and grep for this service's API paths
# Focus on repos with names containing: test, system-test, e2e, integration, qa, validation
for REPO in $(jq -r '.[] | select(.name | test("test|e2e|system|qa|validation"; "i")) | .name' /tmp/org1_repos.json); do
  echo "=== $REPO ===" >> /tmp/test_candidates.txt
  gh api /repos/{ORG1}/$REPO/contents --jq '.[].name' 2>/dev/null >> /tmp/test_candidates.txt
done
```

For each test candidate, clone it locally and grep:

```bash
git clone git@github.com:{ORG1}/$REPO.git /tmp/discovered/$REPO
grep -r "/v3/\|/auth/\|/internal/v1/" /tmp/discovered/$REPO --include='*.py' --include='*.js' --include='*.java' --include='*.go' -l
grep -r "$THIS_SERVICE_HOSTNAME" /tmp/discovered/$REPO -l
```

#### Check 2: Is it a consumer (runtime dependency)?

```bash
# Search for the service's hostname, API paths, or artifact ID in config, env, and source files
grep -r "$THIS_SERVICE_HOSTNAME\|$THIS_ARTIFACT_ID" /tmp/discovered/$REPO \
  --include='*.yaml' --include='*.yml' --include='*.json' --include='*.go' \
  --include='*.py' --include='*.java' --include='*.ts' -l
```

#### Check 3: Is it a borrower (copied source code)?

```bash
# Search for this repo's package names and key class names
grep -r "$THIS_PACKAGE_ROOT" /tmp/discovered/$REPO --include='*.java' -l
# Also search for distinctive utility class names
while read CLASS; do
  grep -r "$CLASS" /tmp/discovered/$REPO --include='*.java' -l 2>/dev/null
done < /tmp/class_names.txt
```

#### Check 4: Is it a CI/deployment repo?

```bash
# Search for this service's name in deployment configs
grep -r "$THIS_REPO_NAME\|$THIS_ARTIFACT_ID" /tmp/discovered/$REPO \
  --include='*.yaml' --include='*.yml' --include='Dockerfile*' --include='*.tf' \
  --include='*.json' -l
```

#### Check 5: Is it a shared library this service uses?

Check this service's `pom.xml` dependencies against all discovered repos:

```bash
grep -r '<artifactId>' pom.xml modules/*/pom.xml | grep -v '^Binary' | sort -u > /tmp/this_deps.txt
# For each dep, search org repos for a matching artifactId
```

---

## Round 2 — Recursive Discovery

After Round 1, you have a set of discovered repos. For each repo that is a test suite, consumer, or borrower:

1. Clone it locally (or search via `gh api search/code`).
2. Repeat all 5 checks from Round 1 against its dependencies and imports.
3. Any newly discovered repo that was not in Round 1 is a Round 2 discovery. Add it to the tracking list with `discovered_in_round: 2`.

The key insight: a test repo may depend on a test library repo. That test library repo must also be documented.

```bash
# GitHub code search across orgs (requires gh auth with read access)
gh api "search/code?q=$THIS_PACKAGE_ROOT+org:{ORG1}" --jq '.items[].repository.full_name' | sort -u
gh api "search/code?q=$THIS_PACKAGE_ROOT+org:{ORG2}" --jq '.items[].repository.full_name' | sort -u
```

---

## Round 3 — Verification Pass

For every repo in the combined Round 1 + Round 2 list:

1. Verify the relationship classification is correct. If a repo was flagged as a test suite, confirm by running `grep -r 'assert\|expect\|assertEqual\|def test_' /tmp/discovered/$REPO -l`.
2. Verify the clone status: is it locally available? If not, attempt to clone it now. Record any clone failures.
3. Verify the branch: what is the default branch? What branch was the code examined from?
4. For test repos: identify whether the test suite can run locally (does it require external services, credentials, or prod URLs that would fail locally?).

After Round 3, no repo in your tracking list should have `relationship: unknown`.

---

## Output Format

After all 3 rounds, produce `docs/codeconverter/03-dependency-discovery/references.md` using this exact structure:

```markdown
# IAM Service — External Repository References

_Last updated: {DATE}_
_Rounds of discovery: 3_
_Total repos scanned: {N}_
_Total repos with relationship: {M}_

---

## This Repo

| Field | Value |
|---|---|
| Name | {THIS_REPO_NAME} |
| Org | {THIS_ORG} |
| GitHub URL | {URL} |
| Primary language | Java |
| Package root | {THIS_PACKAGE_ROOT} |
| API paths served | {THIS_SERVICE_URL_PATHS} |
| Artifact IDs | {THIS_ARTIFACT_IDS} |

---

## Test Suites

Repos that contain test code exercising this service's API.

| Repo | Org | Language | Test framework | # test files | Can run locally | Local path | Branch | Notes |
|---|---|---|---|---|---|---|---|---|
| ... | ... | ... | ... | ... | YES/NO | /tmp/discovered/... | main | ... |

For each test suite repo, add a sub-section:

### {REPO_NAME}

- **Relationship**: Test suite
- **What it tests**: {describe which endpoints or features}
- **How it calls the service**: direct HTTP to `{hostname}:{port}` or via gateway on `{port}`
- **Config file location**: `{path to config file that sets the host/port}`
- **Run command**: `{exact command}`
- **Can run locally**: YES / NO / PARTIAL
- **If NO, why**: {reason — needs prod credentials, needs external IdP, etc.}
- **Local clone path**: {path}
- **Current branch**: {branch}
- **Discovery round**: 1 / 2 / 3

---

## CI Infrastructure

Repos that build, deploy, or monitor this service in CI/CD pipelines.

| Repo | Org | Language | Role | Local clone | Branch | Notes |
|---|---|---|---|---|---|---|
| ... | ... | ... | ... | ... | ... | ... |

For each CI repo, add a sub-section:

### {REPO_NAME}

- **Relationship**: CI / Deployment
- **Role**: {e.g., Helm chart, Terraform, GitHub Actions workflow, Jenkins pipeline}
- **Deployment target**: {e.g., Kubernetes cluster, Docker Swarm}
- **References this service as**: `{service name or image name as used in the CI config}`
- **Config file(s)**: `{paths}`
- **Local clone path**: {path}

---

## IAM Consumers

Services that depend on this service at runtime (call its APIs as part of their normal operation).

| Repo | Org | Language | What it calls | Call type | Notes |
|---|---|---|---|---|---|
| ... | ... | ... | ... | REST / gRPC / SDK | ... |

For each consumer repo, add a sub-section:

### {REPO_NAME}

- **Relationship**: Consumer (runtime dependency)
- **What it calls**: {list of endpoints}
- **Auth method used**: {API key / access token / internal JWT / mTLS}
- **How discovered**: grep for `{search term found}` in `{file}`
- **Local clone path**: {path}

---

## Shared Libraries

Repos that provide library code used by this service, or repos that use library code copied from this service.

| Repo | Org | Language | Direction | What is shared | Notes |
|---|---|---|---|---|---|
| ... | ... | ... | provides / borrows | ... | ... |

---

## Cloned / Local Status

Summary of all repos and their local availability for Phase 3.

| Repo | Cloned | Local path | Clone date | Errors |
|---|---|---|---|---|
| ... | YES/NO | ... | {date} | none / {error} |

---

## Repos Scanned But No Relationship Found

List every repo that was examined and ruled out, with a one-line reason.

| Repo | Reason ruled out |
|---|---|
| ... | No references to IAM package root, service hostname, or API paths |
```

---

## Invariants and Rules

- Every repo in the output must have been actively checked (cloned or searched via `gh api`). Do NOT list repos based on names or descriptions alone.
- Every claim of a relationship must include the search term found and the file it was found in.
- If a repo cannot be cloned (private, no access, archived), document it in the Cloned/Local Status table with the reason. Do not omit it.
- If a repo's relationship is ambiguous after all checks, classify it as `uncertain` and document what would resolve the uncertainty.

---

## Exit Criteria

You may declare Phase 2 complete only when ALL of the following are true:

1. Every repo in `{ORG1}` and `{ORG2}` appears in either the "has relationship" sections or the "no relationship" table. No repo is missing from the output.
2. Every repo in the "has relationship" sections has been cloned locally OR has a documented reason why it cannot be.
3. Every relationship claim has at least one piece of evidence: a search term found in a specific file in the repo.
4. The `docs/codeconverter/03-dependency-discovery/references.md` file is committed to the current working branch.
5. No repo has `relationship: unknown` remaining in any tracking list.
6. At least 3 rounds of discovery have been completed and documented (even if rounds 2 and 3 produce zero new repos — that is a valid result, but you must confirm it explicitly).

If any of these conditions fails, do not move to Phase 3.
