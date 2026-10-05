# UXMASTER findings — claudecodetricks

### F1 — tutorial runtime ignores FORCE_COLOR / CLICOLOR_FORCE
- surface: skills/tutorial/lib run.sh, tutorial.sh (colour decision)
- kind: platform-convention
- severity: low
- evidence: matrix row 6 — `printf 'q\n' | FORCE_COLOR=1 ./run.sh | cat | grep -c $'\033'` → 0 on a fixture project (gaur, 2026-10-05); run.sh has no --color flag, so the force variables are the only way to keep colour through a pipe
- fix: one ladder, `tut_color_on` in box.sh, used by both scripts: NO_COLOR → off, FORCE_COLOR / CLICOLOR_FORCE≠0 → on, TERM=dumb → off, non-TTY stdout → off
- rule: color.md §1 rung 4, §7 row 6
- status: fixed

### F2 — `↑` and `↯` have no ASCII fallback
- surface: tutorial.sh failure and Ctrl-C notices
- kind: platform-convention
- severity: low
- evidence: `LANG=C LC_ALL=C TUT_AUTO=1 ./run.sh 2` prints `↑ exited 1` as raw UTF-8 bytes while the box frame correctly drops to `+ - |`
- fix: `tut_utf8` in box.sh (UTF-8 locale and TERM not dumb) picks `↑ ↯` or `^ !`
- rule: color.md §5
- status: fixed

### F3 — menu numbers and keys in bold yellow are faint on light themes
- surface: run.sh menu, tutorial.sh `[N]` step headers
- kind: ui
- severity: low
- evidence: `python3 ptyrun.py 80 'q\n' ./run.sh` — numbers rendered `ESC[1;33m`; meaning is carried by the key column and `[N]` position, not by colour alone
- fix: none; the user chose bold yellow for numbers and keys when making the roles the standard (gaur step 5)
- rule: color.md §4 (yellow on white)
- status: deferred (user-chosen role; meaning survives with colour stripped)

### F4 — the command line uses the success hue (green) instead of the identifier role
- surface: tutorial.sh `$ cmd` line
- kind: ui
- severity: low
- evidence: `TUT_AUTO=1 ./run.sh 1` in a pty — `ESC[1;32m  $ echo hi`; the `$ ` prefix carries the meaning
- fix: none; bold green for the command is part of the role standard the user approved (gaur step 5); cyan is already the frame colour
- rule: color.md §3
- status: deferred (user-chosen role)

### F5 — failure notices go to stdout, not stderr
- surface: tutorial.sh `exited N`, `tut_done` failure row
- kind: platform-convention
- severity: low
- evidence: `TUT_AUTO=1 ./run.sh 2 2>/dev/null` still shows `^ exited 1`
- fix: none; the notice is narration interleaved with the step's own output, and splitting streams would reorder it under any pipe; the exit code (1) already reports the failure to scripts
- rule: color.md §3 (error role on stderr)
- status: deferred (tutorial narration must stay in order with command output)
