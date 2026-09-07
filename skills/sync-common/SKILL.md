---
name: sync-common
description: Use after editing a file in googlesheets/common/. Identifies every project that includes the edited file (from the projects array in bin/claspcdr2.sh), runs the architecture-guardian agent over the change, then pushes to each affected project via claspcdr2.sh. Invoke with the filename, e.g. /sync-common hierarchy.js.
disable-model-invocation: true
---

You are propagating an edit in `googlesheets/common/` to every project that consumes it.

Argument: `$ARGUMENTS` is the filename inside `common/` that was edited (e.g. `hierarchy.js`, `menu.js`, `hierarchyUI.html`). If empty, ask the user which file.

## Steps

1. **Confirm the file exists.** Verify `googlesheets/common/$ARGUMENTS` is present. If not, list `common/` contents and stop.

2. **Find affected projects.** Read `googlesheets/bin/claspcdr2.sh` and parse the `projects` associative array. Each entry is `projects["<name>"]="<cred>:<comma_separated_files>:<scriptId>"`. Return every project whose file list contains `$ARGUMENTS`. Print the list (project name + credential profile) and a short summary of what was changed.

3. **Architecture review.** Run `git diff -- googlesheets/common/$ARGUMENTS` to capture the change, then invoke the `architecture-guardian` agent (parent `.claude/agents/architecture-guardian.md`) with that diff and the list of affected projects. Relay its findings to the user verbatim. If it flags violations, STOP and do not push — ask the user whether to fix or override.

4. **Confirm before pushing.** Ask the user "Push to these N projects? [yes / pick subset / no]". Don't push without explicit confirmation — pushes touch live spreadsheets used in production.

5. **Push.** For each approved project, run from `googlesheets/`:

   ```bash
   cd bin && ./claspcdr2.sh <project> push && cd ..
   ```

   Run sequentially (each push swaps `~/.clasprc.json`; parallel pushes will race on credentials). Report success/failure per project.

6. **Summary.** Print one line per project: pushed / skipped / failed. If any failed, surface the clasp error so the user can debug.

## Notes

- This skill never edits files in `scripts/<project>/`. If the architecture-guardian flags that the change really belongs in a project-specific data file rather than `common/`, surface that and let the user decide.
- If `$ARGUMENTS` is `menu.js` or any file that's used by nearly every project, double-check before mass-pushing — a bad `menu.js` can silently break menus across all spreadsheets.
- The `projects` array uses both real script IDs and placeholders like `scriptID1` / `scriptID2`. If a project's scriptId looks like a placeholder, warn the user before pushing.
