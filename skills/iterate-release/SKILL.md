---
name: iterate-release
description: "Plans a production release of this project — version, per-platform build/sign/notarize/package, clean-machine install checks, tag, publish — as an iterate plan with mode: release that /iterate executes. The only plan that may notarize, staple, push a version tag or publish. Triggers: \"/iterate-release\", \"plan the release\", \"cut a release\", \"ship v1.2\", \"production release\"."
argument-hint: "[version]  (omit to have one proposed from the commits since the last tag)"
version: 5.21.0
---

<!-- version: FAMILY version, shared by every iterate skill — never bump this file alone. `skillctl family iterate set X.Y.Z` stamps all members at once; drift between them is a defect, not a state. -->

# /iterate-release — plan a production release

A production release is the one moment the project's work reaches other people's machines, so it is the one plan that touches the release credentials: Apple's notary service, Authenticode, the package-signing key, the version tag, the publish target. Every other plan builds and signs at most (`/iterate-planner` §5.11). This skill gathers what the release needs from the project itself, settles the version with the user, and hands a release brief to `/iterate-planner`, which writes the plan. **It never executes.** The user types `/iterate <name>` to run it, and that typed command is the authority for the tag push and the publish — the nightly tick and the conductor never run a release plan.

## Operation router — parse `$1`

0. **version** — `$1` is exactly "version": answer as every family skill does — the family version off disk (`grep -m1 '^version:' ~/.claude/skills/iterate/SKILL.md`) plus `iterate-run version`. Stop.
1. **default** — `$1` empty or a version (`1.4.0`, `v1.4.0`): plan a release, Steps 1–7.

## Steps

### 1. Preconditions

Check, and stop with one line naming the first that fails:

- On the default branch (`git rev-parse --abbrev-ref HEAD` is `main`/`master`), up to date with its remote (`git fetch` then `git status -sb` shows no `behind`).
- No tracked changes outside `.claude/` (`git status --porcelain -- . ':!.claude'`).
- No plan in `./.claude/iterate/plans/` is `phase: executing`, and no other `mode: release` plan is open. One release at a time: a second would race the first for the tag.
- No release-candidate item is `open` or `planned` in `./.claude/iterate/release-candidate.md` (absent file passes). Fails → `release candidate: N items open — /ip rc plan, then /iterate <name>`. Those items are proofs the suite stands in for, and a version ships only after they ran against the real thing (`/iterate-planner` op 8.8).

The test gate is the plan's own first real step, not a planning-time run — planning stays cheap and the gate runs against the exact commit being released.

### 2. Detect the platforms

The release ships to whatever platforms the project already ships to. Read the evidence; never assume a platform the project does not build for.

| Evidence | Platform |
|---|---|
| `docs/engineering.md` Deploy table, `docs/architecture.md` platform statement | authoritative when present |
| `*.xcodeproj`, `project.yml` (XcodeGen), `Package.swift` executables, `pkgbuild`/`productbuild`/`hdiutil`/`codesign`/`notarytool` in the Makefile or `scripts/` | macOS |
| `*.wxs` (WiX), `*.iss` (Inno Setup), `*.nsi`, `Package.appxmanifest`, `signtool` | Windows |
| `debian/`, `*.spec`, `nfpm.yaml`, AppImage/Flatpak manifests, `dpkg-deb`/`rpmbuild` | Linux |
| `.goreleaser.yaml`, `cargo-dist` config, `electron-builder` config | read their target lists |

Evidence that is absent or disagrees becomes one `AskUserQuestion` — multi-select, one option per candidate platform, the evidence for each in its description.

### 3. Propose the version

1. Last release: `git describe --tags --abbrev=0 --match 'v[0-9]*'`. No tag → the first release: propose `1.0.0` when `docs/roadmap.md` marks v1 complete, else `0.1.0`.
2. Commits since it (`git log <tag>..HEAD --format=%s`), read as Conventional Commits: a `!` or `BREAKING CHANGE` → major (minor while below 1.0), any `feat` → minor, otherwise patch.
3. `$1` gave a version → validate it is higher than the last tag and use it as the proposal.
4. `AskUserQuestion`: the proposal first, marked `(Recommended)` with the commit counts that produced it, then the other two bumps. The answer is the version. Nothing is tagged at planning time.

### 4. Credentials, per platform

Ask `/accounts` for each (its "Held entitlements" and "Apple signing and notarization" sections). Each held credential becomes an `Access:` constraint the planner verifies at the front of the plan.

| Platform | Needs | Held → | Not held → |
|---|---|---|---|
| macOS | `Developer ID Application` (+ `Developer ID Installer` for a `.pkg`) for the team, notarytool profile **`notary`** | `Cost: held — Apple Developer Program (<team>), per /accounts` | the membership is paid: a decision for the user now (enroll, or ship unsigned with the Gatekeeper override documented) |
| Windows | an Authenticode code-signing certificate | `Cost: held — <cert>, per /accounts` | paid: a decision for the user now (buy, or ship unsigned with the SmartScreen warning documented) |
| Linux | a package-signing GPG key | fingerprint in the plan | a decision for the user now (generate a release key, or ship unsigned packages) |

The profile is always `notary` — one per team, shared by every project. A plan that names a per-project profile is wrong.

A missing credential that the user already pays for (a lapsed agreement, a notary profile not yet stored) is an access step, not a cost question: name the one act that clears it (`/accounts` knows it) in the brief.

### 5. Release targets

Every release action is a make target (`/dev-makefiles`). Read `make help` and the Makefile and record which exist:

| Target | Does |
|---|---|
| `release-build` | release-configuration build for every platform in Step 2 |
| `release-sign-<os>` | sign each artifact (inside-out for bundles, hardened runtime on macOS) |
| `release-notarize` | macOS: `xcrun notarytool submit --keychain-profile notary --wait`, `xcrun stapler staple`, `xcrun stapler validate` |
| `release-package` | `.dmg`/`.pkg`, `.msi`/installer, `.deb`/`.rpm`, checksums |
| `release-verify` | local checks: `codesign --verify --deep --strict`, `spctl --assess`, `signtool verify /pa`, `gpg --verify` |
| `release-publish` | push artifacts to the publish target |
| `release` | build → sign → notarize → package → verify |

A missing target becomes a step that builds it, ahead of the step that runs it. The version every target embeds comes from the tag (`git describe --exact-match`), so the plan tags locally before it builds.

**Publish target:** an existing `release-publish` target, a goreleaser `release:` block, an update-manifest channel, or a Homebrew tap. None → `AskUserQuestion`: GitHub Release on the repo's remote (Recommended), the project's own host, or artifacts in `dist/` only.

### 6. Write the brief and hand it to the planner

Invoke the Skill tool with `iterate-planner` and this brief as its argument:

```
release brief
version: <X.Y.Z>   previous: <tag or none>
platforms: <macos, windows, linux — those in scope>
credentials: <per platform: held credential and its Access: probe, or the decision the user made>
targets: <each release-* target: exists | build it>
publish: <target>
verify-on: <per platform: a clean guest of the two most recent majors (macOS), the supported distros (Linux), Windows 11>
decisions: <every answer the user gave in Steps 2–5, quoted>
```

The planner writes it per its op 8.7. The release plan's steps, in order:

1. Access preflight for every credential above (the planner's Step 5).
2. Test gate: `testmaster suite run` fast + standard on the release commit, all green.
3. Build each missing release target (per `/dev-makefiles`).
4. Version bump wherever the project records its version, `RELEASES.md` and `CHANGELOG.md` sections for `vX.Y.Z`, the `/user-docs` sync and the Makefile maintenance — committed as the release commit.
5. Tag the release commit locally: `git tag -a vX.Y.Z -m "<title>"`.
6. `make release`. A notarization rejection is executor work: read `xcrun notarytool log <id> --keychain-profile notary`, fix the cause (an unsigned nested binary, a missing hardened runtime, a disallowed entitlement), rebuild, resubmit.
7. Install and run on a clean guest per platform (`/izmachine`): the quarantined download passes Gatekeeper as notarized Developer ID, the Windows installer verifies, the Linux package verifies against the published key. A failure moves the local tag after the fix (`git tag -f`) — it has not been pushed.
8. Push the tag: `git push origin vX.Y.Z`.
9. `make release-publish`.
10. Verify what was published: download each artifact from the publish target, check its checksum and signature, and `xcrun stapler validate` the macOS one.

The planner keeps its standing finishers but runs them inside step 4, before the tag, because artifacts carry the docs and tests must gate the build rather than follow the publish. `/iterate` merges the release branch on all-green; the tag points at the release commit, which main then contains.

### 7. Report

Print the planner's plan presentation, then one line:

`release plan <name> — v<X.Y.Z> for <platforms>; run it with /iterate <name> (the nightly tick and the conductor never launch a release)`

## Rules

1. **Only a release plan notarizes, staples, pushes a version tag or publishes.** Every other plan stops at signing.
2. **Never executes.** Planning only; the typed `/iterate <name>` is the release authorization, and it covers the tag push and the publish the plan names.
3. **The platforms come from the project.** A platform with no build in the repo is not in the release; add it with an ordinary plan first.
4. **The version is the user's to confirm.** Propose it with its reasoning; never tag a number nobody approved.
5. **Held is not paid.** Credentials go through `/accounts` before any cost question (CLAUDE.md "Free and permissive").
6. **One notary profile, `notary`.** Never a per-project profile.
7. **One release plan at a time per project.**
8. **Never staged.** `/ip stage` refuses a `mode: release` plan, the conductor skips it, and the nightly tick does not count it as startable.
