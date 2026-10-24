#!/usr/bin/env python3
"""Exercise process-level ownership and dependency admission through the CLI."""
import json
import os
from pathlib import Path
import subprocess
import tempfile


def main():
    root = Path(__file__).resolve().parents[1]
    binary = root / "bin/iterate-run"
    with tempfile.TemporaryDirectory(prefix="nene-owner-") as folder:
        project = Path(folder)
        plans = project / ".claude/iterate/plans"
        plans.mkdir(parents=True)
        alpha = plans / "alpha.md"
        alpha.write_text("name: alpha\nphase: planned\ndepends: none\nrunning: false\n## Goal\nFixture.\n")

        def command(*words):
            return [str(binary), "plan", *words, "--project", folder, "--json"]

        def run(*words, ok=True):
            result = subprocess.run(command(*words), text=True, capture_output=True)
            if (result.returncode == 0) != ok:
                raise AssertionError((words, result.returncode, result.stdout, result.stderr))
            return result

        children = [subprocess.Popen(command("claim", "alpha", "--owner", owner,
                                           "--pid", str(os.getpid())),
                                     text=True, stdout=subprocess.PIPE,
                                     stderr=subprocess.PIPE)
                    for owner in ("cli", "conductor")]
        results = [(c.communicate(), c.returncode) for c in children]
        assert sorted(code for _, code in results) == [0, 1], results
        run("heartbeat", "alpha")
        run("release", "alpha")
        beta = plans / "beta.md"
        beta.write_text("name: beta\nphase: planned\ndepends: alpha\nrunning: false\n## Goal\nFixture.\n")
        original = beta.read_bytes()
        denied = run("claim", "beta", "--owner", "cli", "--pid", str(os.getpid()), ok=False)
        assert "alpha" in denied.stderr and beta.read_bytes() == original
        archive = project / ".claude/iterate/archive"
        archive.mkdir()
        alpha.rename(archive / "20261024T000000Z-alpha-done.md")
        run("claim", "beta", "--owner", "conductor", "--pid", str(os.getpid()))
        run("release", "beta")
        run("reconcile")
        print(json.dumps({"process_claim_winners":1,"dependency_denial_preserved":True,
                          "archived_predecessor_admitted":True,"result":"passed"}))


if __name__ == "__main__":
    main()
