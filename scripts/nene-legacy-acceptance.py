#!/usr/bin/env python3
"""Exercise staging and terminal states through the installed planning skills."""
import argparse
import json
import os
import re
from pathlib import Path
import shutil
import subprocess
import tempfile


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--scope", choices=["stage", "complete", "gate"], required=True)
    scope = parser.parse_args().scope
    root = Path(__file__).resolve().parents[1]
    evidence = root / ".claude/iterate/plans/nene.evidence/legacy" / scope
    evidence.mkdir(parents=True, exist_ok=True)
    with tempfile.TemporaryDirectory(prefix="nene-legacy-") as folder:
        project = Path(folder)
        plans = project / ".claude/iterate/plans"
        plans.mkdir(parents=True)
        binary = root / "bin/iterate-run"
        guidance = f"Use {binary} for every iterate-run command. Work only in this synthetic folder. No git repo, branch, PR, or scheduling tools are needed.\n"
        (project / "CLAUDE.md").write_text(guidance)
        plan = plans / "wren.md"
        content = "name: wren\nphase: planned\nrunning: false\ndepends: none\nStarted: 2026-10-24T00:00:00Z\nteamed: false\n"
        content += "## Goal\nWrite the fixture receipt.\n## Steps\n- [ ] 1. Run `sleep 2`, then write receipt.txt containing PROVEN. [skill: none]\n"
        if scope == "gate":
            content = content.replace("teamed: false", "teamed: false\nhuman-gate: 2")
            content += "- [ ] 2. Ask the operator to choose RED or BLUE. [human-gate]\n"
        content += "## Validation\n- [ ] 1. Reading receipt.txt returns PROVEN.\n## Constraints\n- Work only in this fixture. Cost: machine time only, authorized.\n## Running resources\n\n## Decisions log\n\n## Status / Log\n"
        if scope != "stage":
            content = content.replace("phase: planned", "phase: planned\nstatus: queued")
        plan.write_text(content)
        state = project / ".claude/iterate"
        (state / "current").write_text("wren\n")
        if scope == "stage":
            executing = plans / "panda.md"
            executing.write_text(content.replace("name: wren", "name: panda").replace("phase: planned", "phase: executing"))
            before = executing.read_bytes()
            prompt = "Perform these planner-only commands in order. After each, copy wren.md to the stated receipt. /ip stage wren then copy to stage.md; /ip unstage wren then copy to unstage.md; /ip stage panda (must refuse); /ip stage wren; /ip add to wren a step to print EXTRA then copy to edited.md. Execute no plan steps. All inputs are complete."
        else:
            prompt = "/iterate wren"
        env = dict(os.environ, ITERATE_RUN_RECORD_DIR=str(project / "record-store"), ITERATE_CONDUCTOR_DIR=str(project / "conductor-store"))
        result = subprocess.run(["claude", "-p", "--model", "haiku", "--effort", "low", "--max-turns", "32", "--dangerously-skip-permissions", prompt], cwd=folder, env=env, text=True, capture_output=True, timeout=115)
        print(result.stdout)
        print(result.stderr)
        (evidence / "transcript.txt").write_text(result.stdout + result.stderr)
        shutil.copytree(state, evidence / "state", dirs_exist_ok=True)
        assert result.returncode == 0, result.returncode
        if scope == "stage":
            assert "status: queued" in (project / "stage.md").read_text()
            assert "status: queued" not in (project / "unstage.md").read_text()
            assert executing.read_bytes() == before, "executing plan changed"
            edited = (project / "edited.md").read_text()
            assert "EXTRA" in edited and "status: queued" not in edited
            assert not (project / "receipt.txt").exists(), "planning executed a step"
        else:
            assert (project / "receipt.txt").read_text().strip() == "PROVEN"
            if scope == "complete":
                assert not re.search(r"go-ahead|before I run|say-so|let me know when", result.stdout, re.I), "machine-time permission requested"
                assert not plan.exists(), "successful plan was not archived"
                archived = list((state / "archive").glob("*wren*done.md"))
                assert len(archived) == 1
                assert "status: queued" not in archived[0].read_text()
            else:
                final = plan.read_text()
                assert "status: awaiting-human-gate" in final
                assert "RED" in result.stdout and "BLUE" in result.stdout
                assert not list((state / "archive").glob("*wren*")) if (state / "archive").exists() else True
            for line in (plan.read_text() if plan.exists() else archived[0].read_text()).splitlines():
                if line.startswith("loop-mechanism:"):
                    assert not line.split(":", 1)[1].strip(), "terminal loop retained"
        print(json.dumps({"scope": scope, "result": "passed"}))


if __name__ == "__main__":
    main()
