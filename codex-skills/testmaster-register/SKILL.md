---
name: "testmaster-register"
description: "TESTMASTER child (invoked via $testmaster): puts tests into the runner's registry — any language, blocking and serial declared, catalog links checked — and holds the rules for writing a test the runner can run."
---

<!-- version: shared across the family; see the **Version:** line above. -->

# $testmaster-register — TESTMASTER registers its own suite

**Version:** 1.0.0

<!-- codex-port: Codex frontmatter permits only name and description, so the
     version lives here in the body. Read it from this line when stamping a
     plan's planner-version / executor-version. -->


Obey the shared contracts in `$testmaster`'s SKILL.md. The `testmaster` binary runs only what `registry.json` names, so a test nobody registered never runs and nothing reports it. This child is the one that registers. Every test the project owns gets into the registry with its blocking and serial properties decided, and every catalog case links to a registered test.

## Usage

Argument: <empty = reconcile the whole project | <file|test-id…> just written | blocking|serial <id…>>. `$1` is its first word; `$ARGUMENTS` is the whole thing.

<!-- codex-port: `argument-hint` has no Codex frontmatter home; folded into this Usage section. Argument substitution is documented for Codex custom prompts but not for skills, so the meaning is stated in prose rather than left to the token alone. -->

## Dependencies

Invoked with Codex's explicit `$name` syntax. Each must also exist under Codex's skill-discovery path or the call will not resolve:

- `$testmaster` — ported.
- `$testmaster-adopt` — ported.
- `$testmaster-maintain` — ported.
- `$testmaster-prune` — ported.
- `$testmaster-run` — ported.

| A test… | Gets in by |
|---|---|
| in Go, Rust or pytest, at a recorded or detected source | `testmaster suite discover`, which `$testmaster-run` calls before every run |
| in Go, Rust or pytest deeper than one level down | `testmaster suite discover --kind <kind> --dir <path>` once; the source is then recorded |
| in any other language | `testmaster test add --kind shell`: step 2 here |
| that must block or run alone | `--blocking` / `--serial`: step 3 here, never left to the default by omission |
| that backs a catalog case | the case's `test` field holding the registry id: step 4 here |

## Arguments: parse `$1`

- *(empty)*: reconcile the whole project (steps 1–5).
- `<file|test-id…>`: tests just written (by `$testmaster-maintain`, or by a plan step). Register those only, skipping the sweep.
- `blocking <id…>` / `serial <id…>` / `parallel <id…>`: re-decide one property with step 3's rules and apply it with `testmaster test set`.

## Steps

### 1. Find what is unregistered

```bash
testmaster suite discover
```

Act on each line:

- `discover <kind> <dir>: N known, M new, K missing`: the M new tests are now registered. Carry on.
- `MISSING <id>`: the toolchain no longer has that test. Hand it to `$testmaster-prune`; never re-add it.
- `UNLINKED <case> <test>`: a catalog case names a test the registry lacks. Either the test exists but is unregistered (register it in step 2), or the link is not a registry id (rewrite the case's `test` to the id `testmaster test list` shows), or the test is gone (hand the case to `$testmaster-prune`).
- `discover: no go, cargo or pytest source`: every test here is a shell test. That is normal.

On a whole-project reconcile, also sweep for tracked test files that no registry entry names by `file` or in its `cmd`:

```bash
reg=$(jq -r '.tests[] | (.file // empty), (.cmd // empty)' .claude/testmaster/registry.json)
git ls-files | grep -E '(^|/)(tests?/[^/]+|[^/]*[._-](test|spec|check)\.[a-z]+|test_[^/]+\.py|[^/]+\.bats)$' |
  while read -r f; do grep -qF "$f" <<<"$reg" || echo "$f"; done
```

Each file it prints is one of these:

- **An unregistered test**: register it in step 2.
- **Not a test**: a helper, a fixture, testdata, or a Go `_test.go` holding only helpers. Skip it.
- **Code another repo owns, or a tool generates**: a vendored, mirrored or generated tree. Skip it, because its tests run in its owner's suite. In claudecodetricks, a skill whose `skills/skillmap.tsv` owner is not `self` is a mirror, and `codex-skills/` is generated.

### 2. Register each one

```bash
testmaster test add tutorial.box-test --kind shell --cmd 'make tutorial-box-test' --file skills/tutorial/tests/box-test.sh
testmaster test add web.login --kind shell --cmd 'npx vitest run src/login.test.ts -t login' --file src/login.test.ts
testmaster test add build --kind shell --cmd 'make build' --blocking
```

- **Id**: `<area>.<name>`, where area is the subsystem or directory and name is the test. History and catalog links key on the id, so choose it once and never rename it.
- **Command**: the narrowest invocation that runs exactly this test, from the project root (or `--dir`). Use the project's own entry point when it has one (a `make` target, an `npm` script). Common selectors: jest `npx jest <file> -t '<name>'`, vitest `npx vitest run <file> -t '<name>'`, bats `bats <file> -f '<name>'`, a test script `bash <file>`. Register one entry per test when the framework can select a single test, and one per file when the file is a single script.
- **Always pass `--file`**: the sweep, `$testmaster-adopt`'s covers pass and whoever fixes a failure all find the test through it.
- **When the bare toolchain cannot build** (cgo, build tags, a venv), record the project's real invocation once with `testmaster runner set <kind> -- <argv…>`, then run discover again.

### 3. Declare blocking and serial

Decide both properties for every new test. Most tests are neither, and the defaults (not blocking, parallel) are right for them.

**Blocking**: only when the test's failure makes every other result meaningless:

- the build or compile check every other test needs
- the fixture, server or database the other tests use coming up
- the toolchain or environment preflight

Never mark a test blocking because it is important. A blocking failure stops the run and every other test reports `not run`, so a wrongly blocking test hides the whole suite. Blocking tests run one at a time before anything else, so keep them fast. Most projects have zero or one.

**Serial** (`--serial`): when running beside another test could change this test's result:

- it binds a fixed port, writes a fixed path outside a temp dir, or uses `$HOME`, a shared database or a shared cache
- it changes global process state: environment variables, the working directory, a system service, a VM or container with a fixed name
- it is heavy enough to starve its neighbours into timeouts

When unsure, choose serial. Serial costs wall-clock time; a wrong parallel costs a flaky red. The test's header `parallel=` is the declaration, and `testmaster test set <id> --serial|--parallel` carries it into the registry.

### 4. Link and prove

- **Link**: a catalog case's `test` is the registry id, or a list of ids when several tests together back one case. The test's header carries the case id: `# TESTMASTER: id=<case id> tier=? parallel=<yes|no>`.
- **Prove**: `testmaster suite run <id>…` once. A test that has never run through the binary is not registered yet, and this run is its first measurement. A `FAIL` here is reported, never hidden: either the test or the product is wrong, and the caller fixes it.

### 5. Report

One line per test, then nothing else:

```
+ tutorial.box-test	shell	-	pass 4.4s
+ build	shell	blocking	pass 12.1s
```

With nothing to register: `register: nothing unregistered`.

## Writing a test the runner can run

`$testmaster-maintain`, and any plan step that writes a test, follows these rules. They make a test cheap to run and cheap to fix without anyone reading it.

1. **It runs unattended.** It shows no prompt and gets no terminal (stdin is not a TTY; a test that needs one opens its own pty). It reaches no network or account it does not stand up itself.
2. **The exit code is the verdict.** Exit 0 only when it checked something and that held. Exit 77 when it cannot run here (a missing tool, no VM), because that is a skip, never a pass. Any other exit is a failure. A script that prints `SKIP` and exits 0 is recorded as a pass.
3. **The failure output is the bug report.** The log is the first and often only thing the fixer reads. Print the command that ran, expected versus actual, and the file or input involved. "assertion failed" on its own costs a rerun.
4. **It owns its state.** It uses temp dirs and its own (or `:0`) ports, cleans up after itself, and never depends on another test running first.
5. **One command runs just it.** Its registry `cmd`, run from the project root, runs this test and no other. A test reachable only by running a whole suite gets its own `make` target or selector.
6. **It is bounded.** It finishes inside the 10-minute floor. Anything longer is a measured slow-tier test, not a hang.
7. **It exercises the product the way a caller does** (the shared real-world mandate).

## Rules

1. **Registry writes go through the binary**: `test add`, `test set`, `runner set`, `suite discover`. Never edit `registry.json`.
2. **Never remove anything.** `MISSING` tests and dead cases go to `$testmaster-prune`.
3. **Never register code this project does not own.**
4. **Blocking and serial are decided, never defaulted by omission.** Step 3's rules decide them, and a header's `parallel=` wins over the default.
5. **A test counts as registered only after one run through the binary.**
