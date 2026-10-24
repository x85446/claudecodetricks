#!/usr/bin/env python3
"""Exercise iterate's installed entry rules against isolated plan state."""
import argparse
import json
import os
from pathlib import Path
import subprocess
import tempfile


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--harness", choices=["claude", "codex"], required=True)
    parser.add_argument("--busy", type=int, choices=[0, 1], default=0)
    args = parser.parse_args()
    root = Path(__file__).resolve().parents[1]
    binary = root / "bin/iterate-run"
    with tempfile.TemporaryDirectory(prefix="nene-entry-") as folder:
        project = Path(folder)
        state = project / ".claude/iterate"
        (state / "plans").mkdir(parents=True)
        plan = state / "plans/wren.md"
        plan.write_text(
            "name: wren\nphase: planned\nrunning: false\ndepends: none\n"
            "Started: 2026-10-24T00:00:00Z\nteamed: false\n"
            "## Goal\nWrite the fixture receipt.\n"
            "## Steps\n- [ ] 1. Write receipt.txt containing PROVEN. [skill: none]\n"
            "## Validation\n- [ ] 1. Reading receipt.txt returns PROVEN.\n"
            "## Constraints\n- Work only inside this fixture directory.\n"
            "- This directory has no git repo and needs no branch or PR.\n"
            "## Running resources\n\n## Decisions log\n\n## Status / Log\n"
        )
        (state / "current").write_text("wren\n")
        guidance = (
            "# Fixture execution\n\n"
            f"The iterate-run binary for this fixture is `{binary}`. "
            "Use that absolute executable path for every iterate-run command. "
            "Login shells can reset PATH. Work only in this fixture directory.\n"
        )
        (project / "AGENTS.md").write_text(guidance)
        (project / "CLAUDE.md").write_text(guidance)
        print('##ITERATE-PROGRESS## '+json.dumps({"done":0,"total":1,
              "message":args.harness+" entry fixture started"}), flush=True)
        env = dict(os.environ)
        env["PATH"] = str(binary.parent) + os.pathsep + env["PATH"]
        env["ITERATE_RUN_RECORD_DIR"] = str(project / "record-store")
        env["ITERATE_CONDUCTOR_DIR"] = str(project / "conductor-store")
        if args.busy:
            subprocess.run(
                [str(binary), "plan", "claim", "wren", "--owner", "conductor",
                 "--pid", str(os.getpid()), "--project", folder],
                check=True, env=env, capture_output=True,
            )
        before = plan.read_bytes()
        prompt = "/iterate" if args.harness == "claude" else "$iterate"
        command = (["claude", "-p", "--model", "haiku", "--max-turns", "24",
                    "--dangerously-skip-permissions", prompt]
                   if args.harness == "claude" else
                   ["codex", "exec", "--skip-git-repo-check",
                    "--dangerously-bypass-approvals-and-sandbox", prompt])
        result = subprocess.run(command, cwd=folder, env=env, text=True,
                                capture_output=True, timeout=115)
        print(result.stdout)
        print(result.stderr)
        if result.returncode:
            raise SystemExit(f"{args.harness} fixture exited {result.returncode}")
        if args.busy:
            if not plan.exists() or plan.read_bytes() != before:
                raise SystemExit("busy entry modified the owned plan")
            if (project / "receipt.txt").exists():
                raise SystemExit("busy entry executed the owned step")
            if "conductor" not in (result.stdout + result.stderr).lower():
                raise SystemExit("busy entry did not identify the owning executor")
        else:
            receipt = project / "receipt.txt"
            if not receipt.exists() or receipt.read_text().strip() != "PROVEN":
                raise SystemExit("sole-ready entry did not execute its fixture plan")
        print(json.dumps({"harness": args.harness, "busy": bool(args.busy),
                          "result": "passed"}))


if __name__ == "__main__":
    main()
