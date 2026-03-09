---
name: md-tester
description: Use when the user invokes /md-tester to manage test spec markdown files, add or update or delete test entries, process ai|feedback annotations, run integration tests, or fix failing tests.
disable-model-invocation: true
argument-hint: [run|add|update|feedback|delete|create] [file] [entry-id]
---

## What This Skill Does

Manages test spec `.md` files that use `{filename}-{NNNN}` labeled entries. Handles full CRUD on test entries, executes tests via their `run:` blocks, and auto-fixes failures in a retry loop.

## Entry ID Scheme

Every entry is labeled `{filename}-{NNNN}` where `{filename}` is the spec file name without `.md` and `{NNNN}` is a zero-padded 4-digit sequential number. Examples: `provider-0001`, `aws-cli-0003`. This makes every entry globally unique and unambiguous.

## Configuration

Read `.claude/skills/md-tester/config.yaml` in the current project for settings. If it doesn't exist, use these defaults:

- `test_docs_path`: `docs/tests/` (relative to project root)

## Initialization (every invocation)

1. Read `config.yaml` from this skill's directory to get `test_docs_path`.
2. Read `README.md` inside `test_docs_path` to load:
   - The **entry format** (`{filename}-{NNNN}` template)
   - All **rules** (R-001 through R-NNN)
   - All **macros** and their expansion tables (PROVIDERS, CELLMODES, etc.)
3. These rules and macros are the source of truth for all operations below.

## Dispatching

Parse `$ARGUMENTS` to determine the operation. Support both structured subcommands and natural language.

**Subcommands:**

| Command | Example | Action |
|---------|---------|--------|
| `run` | `/md-tester run provider-0003` | Execute tests |
| `add` | `/md-tester add provider` | Add new entry |
| `update` | `/md-tester update aws-cli-0005` | Fill in blank fields |
| `feedback` | `/md-tester feedback aws-cli` | Process ai\|feedback annotations |
| `delete` | `/md-tester delete provider-0004` | Remove an entry |
| `create` | `/md-tester create networking` | Create a new test spec file |

When an entry ID like `provider-0003` is given, the filename is derived from the prefix (`provider` → `provider.md`). When just a filename is given (e.g., `provider`), the operation applies to all entries in that file.

If no subcommand matches, interpret `$ARGUMENTS` as natural language and map to the closest operation.

If no arguments given, ask the user what they want to do.

---

## Operation: `run`

Execute tests defined in entries.

### Steps

1. **Identify targets.** Parse arguments to determine which entries to run:
   - `run provider-0003` — single entry in `provider.md`
   - `run provider` — all entries in `provider.md` that have a `run:` block
   - `run all` — all entries across all test spec files that have `run:` blocks
   - Natural language: `run all clipboard tests` — grep tags for "clipboard", run matching entries
2. **Skip unimplemented.** If an entry has no `run:` block, skip it and note it as "not yet implemented."
3. **Execute.** For each target entry, extract the `run:` bash block and execute it from the project root directory.
4. **Report results.** Show pass/fail status for each entry. Include the entry ID and short title.
5. **On failure — fix loop** (max 3 retries per entry):
   a. Show the failure output to the user.
   b. Read the Go test file listed in the entry's `test:` field.
   c. Read any relevant source files referenced by the test.
   d. Diagnose the failure — determine if it's a test bug or a source bug.
   e. Fix the code (test or source) using Edit tool.
   f. Re-run the `run:` command.
   g. If still failing after 3 retries, report the failure and move on.
6. **Summary.** After all targets, print a summary: X passed, Y failed, Z skipped (unimplemented).

---

## Operation: `add`

Add a new entry to an existing test spec file.

### Steps

1. **Identify file.** The argument is the spec file name (without `.md` extension). Read the file from `test_docs_path`.
2. **Determine next number.** Scan existing entries to find the highest `{filename}-{NNNN}` and increment by 1. Zero-pad to 4 digits.
3. **Gather details.** Ask the user:
   - What invariant is being tested? (1-3 sentences)
   - What tags should it have?
4. **Write the entry.** Append to the file using the exact format from README.md:

```
### {filename}-{NNNN}: {Short title}

{Description of the invariant.}

- tags: {tags}
- test:
- functions:
- run:
```

5. **Blank fields are correct.** Per rule R-004, leave `test:`, `functions:`, and `run:` blank for new entries. The description is sufficient — implementation comes later via `update`.
6. **Follow all rules.** Especially:
   - R-001: One description, many sub-tests (don't expand macros in the description)
   - R-003: Use macro names like "for PROVIDERS" or "across CELLMODES" in descriptions
   - R-006: No unit tests in these specs

---

## Operation: `update`

Fill in blank fields on existing entries by generating Go test code.

### Steps

1. **Identify targets.** Parse arguments to find which entries need updating.
   - `update aws-cli-0005` — single entry
   - `update aws-cli` — all entries with blank fields in that file
2. **Read the entry description.** Understand the invariant being tested.
3. **Read macros from README.md.** Expand any macro references in the description to determine what the test code needs to iterate over (R-001).
4. **Read existing test files.** If the `test:` field points to an existing file, read it to understand the testing patterns already in use.
5. **Generate or update Go test code.** Write the test function(s) that implement the described invariant. Follow:
   - R-001: Single test function with sub-tests iterating over macros
   - R-005: Use appropriate build tags for infrastructure tests
   - Match the style and patterns of existing tests in the same file
6. **Backfill the `.md` entry fields:**
   - `test:` — path to the Go test file (backtick-wrapped)
   - `functions:` — Go function name(s) (backtick-wrapped)
   - `run:` — the bash command to execute the test, in a fenced code block
7. **Verify.** Run the generated `run:` command to confirm the test compiles and (if infrastructure is available) passes.

---

## Operation: `feedback`

Process `ai | feedback:` annotations on entries (rule R-007).

### Steps

1. **Read the target file.** Scan for any entries containing `- ai | feedback:` or `- feedback:` lines.
2. **For each annotation:**
   a. Read the feedback instruction.
   b. Apply the requested change to the entry (modify description, tags, test code, etc.).
   c. If the feedback affects test code, update the Go test file as well.
   d. **Remove the `ai | feedback:` line** from the entry after applying the change.
3. **Report.** List each entry that was modified and what changed.

---

## Operation: `delete`

Remove an entry from a test spec file.

### Steps

1. **Identify the entry.** Parse the entry ID (e.g., `delete provider-0004`).
2. **Show the entry to the user.** Display the full entry content.
3. **Confirm deletion.** Ask the user to confirm before removing.
4. **Remove.** Delete the entire entry block (from `### {filename}-{NNNN}` to the next `###` heading or end of file).
5. **Do NOT renumber.** Leave gaps in numbers. Other entries may reference these IDs.
6. **Clean up test code.** If the entry had a `test:` and `functions:` field, ask the user if the corresponding Go test function should also be removed.

---

## Operation: `create`

Create a new test spec `.md` file.

### Steps

1. **Name.** The argument is the file name (e.g., `create networking` → `networking.md`).
2. **Gather context.** Ask the user:
   - What area/component does this test spec cover?
   - Any specific macros this spec will use?
3. **Generate the file.** Create it in `test_docs_path` with:
   - H1 title describing the test area
   - Brief intro paragraph
   - Format reference (same as other spec files)
   - Macro table (referencing README.md macros that apply)
   - Separator (`---`)
   - No initial entries (user adds them with `add`)
4. **Follow existing patterns.** Read one existing spec file to match the header/format conventions used in this project.

---

## Format Reference

The canonical entry format (loaded from README.md each time, but shown here for reference):

```
### {filename}-{NNNN}: {Short title}

{1-3 sentences stating the invariant being tested.}

- [ai | feedback]: comment to ai
- tags: tag1, tag2, tag3
- test: path/to/test_file.go
- functions: TestFunctionName
- run:
  ```bash
  go test -tags docker -v -timeout 5m ./path/to/pkg/... -run TestFunctionName
  ```
```

## Rules Summary

These are loaded from README.md each invocation. Key rules to always follow:

- **R-001**: One description, many sub-tests. Test code iterates over macros.
- **R-002**: Tags drive filtering, not structure. No category sections.
- **R-003**: Descriptions use macro names, don't expand them.
- **R-004**: Blank fields = not yet implemented. Don't invent values.
- **R-005**: Build tags gate infrastructure tests.
- **R-006**: No unit tests in these specs.
- **R-007**: Process and remove `ai | feedback:` annotations.

Always defer to README.md if rules have changed since this skill was written.

## Important Notes

- **Never invent `run:` commands** for entries that don't have test code yet. Blank means unimplemented (R-004).
- **Max 3 retries** on the fix loop. If a test still fails, stop and report.
- **Confirm before deleting.** Always show the entry and ask before removing.
- **Don't renumber entries.** Gaps are fine; IDs may be referenced elsewhere.
- **Read README.md every time.** Rules and macros may have been updated since last invocation.
- **Run tests from project root.** The `run:` commands assume cwd is the project root.
- **Respect build tags.** Don't run `-tags docker` tests if Docker isn't available — warn and skip.
