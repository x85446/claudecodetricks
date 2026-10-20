# Qualifying questions — the stage 9 decision tree

Only the operator edits this file. `/product-environment-qualify` walks it from Q1, answers each question from the evidence its line names, follows the branch, and establishes every obligation in the block the leaf names, in every environment. The AI never adds, removes or rewords a question here.

Grammar: `### Q<n> — <question>` with `- evidence:`, `- yes →` and `- no →` (a `Q<m>`, an `OBL-<x>` or `end`); `### OBL-<x> — <title>` with one `- [ ] <obligation> — probe: <what proves it>` per line.

### Q1 — Are we compiling a Mac app?
- evidence: `docs/architecture.md` platform statement; `docs/engineering.md` Stack and Deploy (a macOS SDK or Xcode target, a `.app` bundle, a launchd agent, a `.pkg`); the `Makefile` (`codesign`, `pkgbuild`, `productbuild`, `xcodebuild`).
- yes → Q2
- no → end

### Q2 — Do we sign with Apple's official identity (Developer ID, Apple Developer Program)?
- evidence: `docs/engineering.md` Deploy (the signing identity and team the build uses); `/accounts` (Apple Developer Program held, team id); `security find-identity -v -p codesigning` on the owner's machine.
- yes → OBL-A
- no → OBL-B

### OBL-A — Signed everywhere, permissions pre-set, headless
Every environment — build, test, release and any other the deploy table names — is established to use the signatures and rebuild with them, and every OS permission is already set, before any other plan runs.
- [ ] Every environment holds the Developer ID identity, or receives binaries the owner's machine signed with it — probe: in each environment `security find-identity -v -p codesigning` lists the identity, or `codesign -dv --verbose=2 <binary>` on the delivered binary shows the team and identifier.
- [ ] Every build in every environment rebuilds signed: the default build target signs, no ad-hoc identity, and no unsigned binary reaches a test that touches an OS permission — probe: after a fresh `make build`, `codesign --verify --deep --strict <binary>` exits 0 and `codesign -dv --verbose=2 <binary>` shows `Authority=Developer ID Application`.
- [ ] Every OS permission the product needs (the permission-holder list in `docs/engineering.md`: Full Disk Access, Accessibility, Screen Recording, Input Monitoring, Automation, …) is already granted to the stable designated requirement in every guest's golden image and baked into its `clean` snapshot, and granted once on the owner's real Mac — probe: the permission-holder reads a protected path in each guest (the launchd daemon lists `~/Documents`; a TCC database row proves nothing); the real Mac's grant is one operator act, listed once.
- [ ] `.iz` carries every guest with the provisioning that installs the signed binary and the grants, so `izmachine up <key>` reproduces a pre-authorized guest from scratch — probe: `izmachine validate --format json` and `izmachine up <key> --dry-run --format json` exit 0 and the `[[provision]]` list names the signing and grant steps.
- [ ] Headless: no later plan prompts for a signature or a permission — probe: the stage's smoke proof runs in each guest over SSH with no console act, and `/iterate` logs no `blocked-on-operator` on a permission or a signing dialog.

### OBL-B — SIP off, grants scripted, headless
No official identity: every build and run environment is configured so the AI grants itself every allowance by command, for the whole lifetime of every future build and run, with no human act.
- [ ] System Integrity Protection is disabled in every guest's golden image and `clean` snapshot — probe: `csrutil status` in each guest reads disabled, or `unknown (Custom Configuration)` with `nvram csr-active-config` showing the protections off. The owner's real Mac is not an environment this branch changes: it keeps SIP on and its grant is one operator act, listed once.
- [ ] The AI has a proven command path that grants every OS permission the product needs to the current build's binary in each guest, with no human act — probe: the project's make target (`make tcc-grant <binary>` or its equivalent writing the TCC store) exits 0 and the permission-holder then reads a protected path.
- [ ] The grant path runs after every rebuild, because an ad-hoc identity changes with each build — probe: the build or test target calls the grant step, or the guest's provisioning does, and a rebuilt binary reads the protected path with no prompt.
- [ ] `.iz` carries the SIP-off state and the grant step in every guest's provisioning — probe: `izmachine validate --format json` and `izmachine up <key> --dry-run --format json` exit 0 and the `[[provision]]` list names them.
- [ ] Headless: no later plan prompts for a permission — probe: the stage's smoke proof runs in each guest over SSH with no console act, and `/iterate` logs no `blocked-on-operator` on a permission.
