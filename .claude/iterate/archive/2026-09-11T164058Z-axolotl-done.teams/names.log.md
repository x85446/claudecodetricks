# Team: names — axolotl plan log

2026-09-10 (session start): Read src/internal/iterrun/names.go in full and existing names_test.go. Confirmed StoreDir() uses os.UserHomeDir() (respects $HOME env var) — safe for temp-HOME testing. Beginning compilation of expanded animal pool (110 -> 1320+, floor 40/letter). Strategy: real living single-word animal common names + real extinct-taxon genus names (dinosaurs/pterosaurs/fossil vertebrates) for padding thin letters (i,j,k,q,u,v,x,y,z). Will use WebSearch to verify/supplement thin-letter lists rather than relying purely on memory, then validate structurally with a python script (lowercase/prefix/dup/count) before writing final Go source.

## Step 13 (13a/13b) — DONE, first, as its own unit
Implemented graceful exhaustion in `nextPlanName` (src/internal/iterrun/names.go):
- When the letter a project's own sequence would naturally land on next has
  zero unused names left machine-wide, the loop already walked forward to
  the next letter with capacity (existing structure) — added a
  `fmt.Fprintf(os.Stderr, ...)` line so that advance is now LOGGED, not
  silent: "iterate-run: letter 'a' exhausted machine-wide, advancing to 'b'".
- Only the true all-pool-exhausted case still errors, and its message now
  names the remaining-name count per letter via two new helpers,
  `remainingByLetter` and `formatRemainingByLetter` (added to names.go).
- `go build ./...` green. `go test ./src/internal/iterrun -run TestNextPlanName -v`
  all 5 pass — confirmed thin letters ('u','x') already exhaust and advance
  correctly under the existing 110-name pool, logging as designed.
- Verified against the REAL BINARY (built via `go build -o /tmp/iterate-run-test
  ./src/cmd/iterate-run`), each with a synthetic temp HOME (never touched the
  real `~/.claude/iterate-run/plan-names.json`):
  - Scenario A: state file marks all 6 'a' names used. `iterate-run name next`
    (cwd = fresh temp project dir) printed "badger" to stdout, exit 0, and
    stderr showed `iterate-run: letter 'a' exhausted machine-wide, advancing
    to 'b'`. Confirms degrade-not-fail + logged advance.
  - Scenario B: state file marks all 110 current pool names used (extracted
    programmatically from animalsByLetter so it's exact, not hand-typed).
    `iterate-run name next` exited 1 with: "iterate-run: every animal
    codename in the pool is already in use (extend animalsByLetter in
    names.go) — remaining per letter: a=0 b=0 c=0 ... z=0". Confirms the
    all-exhausted error now reports per-letter remaining counts.
- Ran `make build`: green, all 4 binaries built (Version: iterate-v3.3-dirty).
- Cleaned up all scratch temp dirs and the throwaway test binary; the real
  global plan-names.json was never opened, only synthetic temp-HOME copies.

##ITERATE-VALIDATION## {"step":13,"status":"met","note":"exhaustion now degrades to next-letter-with-capacity (logged to stderr) and only the whole-pool-exhausted case errors, with a per-letter remaining-count breakdown; verified against the real binary with two synthetic temp-HOME scenarios, go build and go test both green"}

## Note: shared working tree observed mid-task
Before starting on step 13, `git reflog -10` showed recent checkouts through
this SAME working directory unrelated to this team's task: fix/conductor-stands-down -> main -> tool/ccperm -> main (commit 005a1567
"feat(ccperm)...") -> feature/axolotl-iterate-infra -> a reset. A
system-reminder also fired mid-task claiming names.go had "changed on disk"
back to the pre-edit 110-line version, which did NOT match the actual disk
content when I re-checked (275 lines, my edit intact, no conflict markers,
`git status --porcelain -- src/internal/iterrun/` showed only names.go
modified as expected). Read as: this repo's working directory is shared with
at least one other active process outside this team (likely the live
interactive session), and branch switches there are landing in the same
tree we're working in. My step-13 work was NOT lost or corrupted — verified
clean before and after — but flagging this since a badly-timed checkout
here could silently clobber uncommitted team work. Did not touch branches,
did not attempt any git remediation myself (out of scope, and none needed —
current state is clean). Continuing to steps 12 then 11 as instructed.

## Step 12 (12a/12b) — DONE, written before the pool is full
Added `TestAnimalPool` to src/internal/iterrun/names_test.go, checking:
- every letter a-z present in animalsByLetter (none missing/empty)
- every letter's pool >= animalPoolFloor (40)
- pool total >= animalPoolMinTotal (1320 — a real constant, NOT
  26*floor=1040, since 26 letters sitting exactly at the bare floor is
  itself the unhealthy case worth catching; commented as such)
- no duplicate name anywhere in the pool, across ALL letters (map keyed by
  name -> owning letter, flagged on second sighting)
- every entry lowercase ASCII, and every entry's first byte equals its own
  map key letter

Currently RED as expected/intended: with the pool still at 110 entries it
fails on every letter's floor assertion and the total assertion (110 < 1320)
— this is the progress meter for step 11, not a bug. `go build ./...` is
green; the test file compiles cleanly.

Verified all four failure modes by temporary mutation, each reverted
immediately after (diff-confirmed identical to pre-mutation file at the end):
1. Duplicate — copied "aardvark" into letter 'b' too -> failed with
   `entry "aardvark" appears under both letter 'a' and letter 'b'`.
2. Uppercase — changed "ape" to "Ape" -> failed with
   `entry "Ape" is not lowercase ASCII (offending byte 'A')`.
3. Wrong-letter — moved "badger" into letter 'a''s list -> failed with
   `entry "badger" does not start with its own key letter`.
4. Short letter — shrank 'q' to 1 entry -> failed with
   `letter 'q': pool has 1 names, want at least 40 (floor)`.

All other names tests (TestNextPlanName*) still pass; only TestAnimalPool
is red, and only for the two reasons named above. Moving to step 11 now —
TestAnimalPool going green is how I'll confirm the pool growth actually
worked, batch by batch.

##ITERATE-VALIDATION## {"step":12,"status":"met","note":"TestAnimalPool added and correctly red pre-growth (floor+total only); all four mutation failure modes (duplicate, uppercase, wrong-letter, short-letter) verified individually with clean reverts; go build stays green throughout"}
