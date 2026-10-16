---
name: product-environment
description: "PRODUCT child (invoked via /product): the environment the build runs in — every guest (OS × version × arch), toolchain, third-party credential and live-only proof the roadmap needs, each proven by a real probe, substituted by a local server, tracked for the release candidate, or reduced to one operator act on one list, so the milestone chain runs headless."
version: 1.5.0
---

# /product-environment — stage 9: tear down the human gates

The milestone chain runs unattended from M1-01 to M3-10, and a plan that meets a wall nobody listed parks there with everything behind it. This stage finds every wall before the first plan launches: every machine the build, test, install and release tiers need, every toolchain on them, every third-party account and credential, and every proof only the real thing gives. Each one ends up **ready** (proven by a real probe), **substituted** (a local server stands in), **tracked** (on the release-candidate ledger), or **one operator act** on one list. That list is the whole human agenda for the roadmap, and the gate is whether it is empty.

The worst case this stage exists for: several operating systems at several versions, two architectures, and a Google account, an Azure tenant, a Cloudflare zone and a signing identity, each discovered by a different plan on a different night. Here they are one table, worked through once.

## Steps

1. **Read**, in this order:
   - `docs/engineering.md` — Stack (every toolchain and its pinned version), Machines (every `[machine.<key>]` need), Build, Test strategy (the tiers that touch a platform), Deploy (every environment and what it runs on).
   - `docs/architecture.md` — the platform statement (every target OS, version and architecture) and every external service an ADR adopts.
   - `docs/requirements.md` and `docs/features.md` — every vendor, protocol or service a requirement names, with its release.
   - `docs/roadmap.md` — each release's end-to-end check and the integrations it needs.
   - `docs/milestones.md` — which milestone first touches each need.
   - `./.iz`, `.claude/data/accounts.md` and `/accounts show` — what the project already has.

2. **Inventory the needs** into three classes. Every row carries its release and the first milestone that uses it.

   | Class | One row per | Established by |
   |---|---|---|
   | Machines | kind × OS × version × arch that any tier needs: a build box, a guest per target OS for the install, integration and UI tiers, a clean box for the install check, a server to run against. The toolchains the pinned stack needs on it are part of the row. | `/izmachine` |
   | Access | account, credential, API token, OAuth client, signing identity, forge, registry or deploy target — after the substitution test | `/accounts` |
   | Live-only proofs | what no local substitute proves: a real consent screen, a real notarization, a real tenant | `/ip release candidate task to track:` |

   **The substitution test runs first on every access candidate** (`/iterate-planner` Step 5). An open standard with a free implementation is a local server and a creation step — IMAP is Dovecot, S3 is MinIO, OIDC is Keycloak — and lands in the Access table as `substituted`, with no account behind it. Only a vendor that is itself the deliverable is an access need.

   **Every release is inventoried; v1 is proven.** The chain runs all thirty milestones unattended, so v2 and v3 needs are listed under `## Later releases` with the status `later — v2`. They are proven when that release's milestones are re-cut in maintain mode.

3. **Establish each need** — one subagent per class, in parallel, each returning one line per row: `<class>\t<need>\t<status>\t<proof or act>`.
   - **Machines.** Write or reuse a `[machine.<key>]` definition in `./.iz` with a `needs` directive (kind, os, osversion, and arch when the row has one), and its toolchains in the `[[provision]]` list. A host appears only when the user's own words named one. `izmachine validate --format json`, then `izmachine up <key> --dry-run --format json`; `resolution.remote` is the placement. `izmachine up <key> --format json`, then the **smoke proof** inside the guest: brownfield, the project's own build and fast tier (`izmachine ssh <key> -- make check`); greenfield, the pinned stack's version command and a hello-world build in its language. A green proof is snapshotted as the clean base (`izmachine direct snapshot create <guest> clean --provider <p> --format json`) and the guest is stopped; plans bring it up. No registered host can place the row → `operator:` with the one act (register a host that can, add the image, or drop the version from the platform statement). A toolchain that needs a vendor account to download is an Access row too.
   - **Access.** `/accounts` first: held or not, and where the credential lives — a file path, an env var name, a keychain item, a profile name, never a value. Then the real end-to-end probe of the capability the milestones use: `gh auth status`, the vendor API's whoami call, `security find-identity -v -p codesigning`, `xcrun notarytool history --keychain-profile notary`, the project's own `make probe-<vendor>`. Green → `ready` with the probe and its date. Not green and `/accounts` can self-heal (an agent emptied by a locked KeePassXC, a refreshable token) → heal, re-probe. Otherwise → `operator: <the one act>`: create the OAuth client, publish it to production, store the notary profile, authorize a key. A paid need nobody holds is the free-and-permissive question, asked at this gate as a choice (free path, held, or the user's words authorizing it) and never left for a plan. Each ready credential's location is recorded as a pointer under `## Used by this project` in `.claude/data/accounts.md`; the gate is the confirmation `/accounts` asks for.
   - **Substituted** rows: stand the local server up through the project's make target or fixture (`/dev-makefiles` adds the target when there is none), run the probe, record it.
   - **Live-only proofs**: `/ip release candidate task to track: <what only the real thing proves>` for each row; record the `RC-NN`.

4. **Write `docs/environment.md`** (format below), then **analyze** — the meta's Step 4 plus:
   - **Coverage** — every OS, version and architecture in the platform statement has a Machines row; every `[machine.<key>]` in engineering's Machines section has one; every vendor named in requirements, features or architecture has an Access row; every Deploy environment has the machine or access it runs on.
   - **Placement** — no row names a host the user did not name; every definition in `./.iz` carries `needs` or a user-named `remote`.
   - **Secrets** — no value anywhere in the document: a token, a password, a key body, a client secret. Locations only.
   - **Acts** — every `operator:` row is exactly one act and names its first milestone; `## Operator acts` matches the rows, ordered by that milestone.
   - **Proof** — every `ready` row has a probe command and a date. A row with neither is `unproven`, never `ready`.

5. **The gate is readiness.** Present the three tables' counts and `## Operator acts`. The stage is `done` when that list is empty. The user may approve with acts open; the meta records `override: N acts`, and the chain then parks on each act when a plan reaches it, which is the user's call. Re-running `/product environment` re-probes every row, and acts the operator did drop off the list by themselves.

## Output — `docs/environment.md`

```markdown
# <Product> — Environment

**Date:** <YYYY-MM-DD>  ·  **Proven through:** v1  ·  **Operator acts open:** <N>

## Machines
| Key | Need | Serves | First | Placement | Proof | Status |
|---|---|---|---|---|---|---|
| mac-26 | vm · macos · 26 · arm64 | install, UI | M1-01 | needs → <remote> | `izmachine ssh mac-26 -- make check` · 0 · <date> | ready |
| win-10 | vm · windows · 10 · amd64 | install, UI | M1-01 | needs → — | — | operator: add a Windows 10 image to a registered host, or drop 10 from the platform statement |

## Access
| Need | Used by | Disposition | Lives at | Proof | Status |
|---|---|---|---|---|---|
| Google OAuth client, Gmail scope | R-12 · M1-04 | vendor is the deliverable | `~/.<product>-dev.env` (client id) · keychain item `<product>-dev` (refresh token) | `make probe-google` · 0 · <date> | operator: publish the OAuth client to "In production" (a Testing client's tokens expire in 7 days) |
| IMAP | R-03 · M1-02 | substituted — Dovecot via `make fixture-imap` | — | `make fixture-imap` · 0 · <date> | ready |
| Apple Developer ID | release | held — Apple Developer Program (<team>), per /accounts | keychain identity | `security find-identity -v -p codesigning` lists it · <date> | ready |

## Live-only proofs
| RC | Only the real thing proves | Stand-in | First |
|---|---|---|---|
| RC-01 | the real token revoke | stub in `test/integration/…` | M1-04 |

## Operator acts
Ordered by the first milestone that needs them. One act each; `/product environment` after any of them re-probes and drops what cleared.
1. [ ] Add a Windows 10 image to a registered izmachine host, or drop 10 from the platform statement — M1-01
2. [ ] Publish the Google OAuth client to "In production" — Cloud Console → OAuth consent screen — M1-04

## Later releases
| Need | Release | Class | Note |
|---|---|---|---|
| Azure AD tenant | v2 | access | proven when v2 is re-cut |

## Calls made
- <every judgment: a substitution chosen, a version taken from the platform statement, a paid option routed to the free path>

## Deferred validations
- <what only the product running proves>
```

## Rules

1. **A row is `ready` only with a probe and a date.** A need nobody ran a real command against is `unproven`. Reachability is not the probe; the capability the milestones use is.
2. **Never a secret value.** This document and `.claude/data/accounts.md` hold locations: a path, an env var name, a keychain item, a profile name.
3. **Placement is izmachine's.** Every machine is a `needs` definition in `./.iz`; a host appears only when the user named it. Hardware at hand is never a reason to drop an OS, a version or an architecture.
4. **The substitution test precedes every access row.** A standard with a free implementation is a local server, never an account.
5. **Free and permissive holds here, at the gate.** A paid need nobody holds is one choice put to the user before `done`; it never reaches a plan as a question.
6. **One act per operator row, one list, ordered by first milestone.** The list is the whole human agenda for the roadmap. A plan that parks on something absent from it is this stage's defect, and the row is added.
7. **Every release is inventoried, v1 is proven.** Later releases' rows are proven when their milestones are re-cut.
8. **Guests are deliverables, not scratch.** Each is snapshotted `clean` and stopped; plans bring them up with `izmachine up <key>` and roll back to `clean` for an install check. The stage leaves no guest running.
9. **It writes `docs/environment.md`, `./.iz`, pointers in `.claude/data/accounts.md` and the release-candidate ledger, and nothing else.** No product code, no plan, no edit to a locked document: a gap in `docs/engineering.md` (an unnamed OS version, a missing deploy environment) is reported for the user to unlock.
10. **Re-running re-probes.** Credentials expire and guests get destroyed, so every invocation runs every probe again, and maintain mode reports each row whose probe stopped passing as drift, with its act back on the list.
