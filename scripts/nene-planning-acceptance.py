#!/usr/bin/env python3
"""Exercise installed planning skills with synthetic requirement sets."""
import argparse
import json
import os
from pathlib import Path
import re
import collections
import subprocess
import shutil
import tempfile


def invoke(folder, prompt):
    print('##ITERATE-PROGRESS## '+json.dumps({"done":0,"total":1,
          "message":"planning fixture started"}), flush=True)
    result = subprocess.run(
        ["claude", "-p", "--model", "haiku", "--max-turns", "24",
         "--dangerously-skip-permissions", prompt],
        cwd=folder, text=True, capture_output=True, timeout=115,
    )
    print(result.stdout)
    print(result.stderr)
    if result.returncode:
        raise SystemExit(f"planning fixture exited {result.returncode}")


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--scope", choices=["small", "large", "consolidate", "handoff"], required=True)
    args = parser.parse_args()
    root = Path(__file__).resolve().parents[1]
    artifacts = root / ".claude/iterate/plans/nene.evidence/planning"
    artifacts.mkdir(exist_ok=True)
    with tempfile.TemporaryDirectory(prefix="nene-planning-") as folder:
        p = Path(folder)
        (p / "docs").mkdir()
        guidance = (
            "# Synthetic planning fixture\n\n"
            f"Use `{root / 'bin/iterate-run'}` for every iterate-run command. "
            "Work only in this synthetic directory. It has no git repo. "
            "Plan only. Preserve requirement IDs and acceptance wording.\n"
        )
        (p / "CLAUDE.md").write_text(guidance)
        if args.scope == "handoff":
            state = p / ".claude/product"
            state.mkdir(parents=True)
            (state / "state.md").write_text(
                "# Product state\n\n| Stage | Status |\n|---|---|\n| 8 | done |\n| 9 | pending |\n")
            requirements = []
            milestones = []
            for i in range(1, 11):
                context = "intake lifecycle" if i <= 5 else "export lifecycle"
                requirements.append(f"## R-{i:03d}\nF-001. {context}. AC-{i:03d}: action {i} prints receipt {i}.\n")
                milestones.append(
                    f"### M1-{i:02d} — {context} checkpoint {i}\n"
                    f"**Goal:** R-{i:03d}, {context}.\n"
                    f"**Demo:** Run action {i} and read receipt {i}.\n"
                    f"**Delivers:** R-{i:03d}, F-001.\n"
                    f"**Touches:** {context} state machine.\n"
                    f"**Retires:** transition {i} correctness.\n"
                    "**Depends on:** —\n"
                    f"**Exit criteria:**\n- [ ] AC-{i:03d}: action {i} prints receipt {i}.\n"
                )
            (p / "docs/milestones.md").write_text("# Milestones\n\n"+"\n".join(milestones))
            (p / "docs/requirements.md").write_text("# Requirements\n\n"+"\n".join(requirements))
            (p / "docs/features.md").write_text("# Features\n\nF-001: receipt processing.\n")
            (p / "docs/architecture.md").write_text(
                "# Architecture\n\nTwo independent state machines: intake lifecycle and export lifecycle. "
                "Each lifecycle shares implementation and delivery across all five checkpoints.\n")
            (p / "docs/engineering.md").write_text(
                "# Engineering\n\nGo stdlib CLI. License: BSD-3-Clause. Cost: none. "
                "make build; make test. Fast and standard tests exercise receipt output. "
                "No external accounts or guests. Publication uses a distinct release plan.\n")
            (p / "docs/environment.md").write_text(
                "# Environment\n\nNo external accounts or guests. Stage 9 awaits approval; stage no plans.\n")
            invoke(folder, "/product-iteratemilestones")
            active = list((p / ".claude/iterate/plans").glob("*.md"))
            shutil.copytree(p / ".claude/iterate", artifacts / "handoff-state", dirs_exist_ok=True)
            if len(active) != 2:
                raise SystemExit(f"initial handoff wrote {len(active)} plans, expected the two coherent lifecycles")
            groups = []
            combined = ""
            for plan in active:
                text = plan.read_text()
                groups.append(set(id for line in re.findall(r"^milestone:\s*(.*)$", text, re.M)
                                  for id in re.findall(r"M1-\d+", line)))
                combined += text+"\n"
                if re.search(r"^status:\s*queued", text, re.M):
                    raise SystemExit("handoff staged work before environment approval")
            expected = [{f"M1-{i:02d}" for i in range(1,6)}, {f"M1-{i:02d}" for i in range(6,11)}]
            if not all(group in groups for group in expected):
                raise SystemExit(f"handoff grouping differs from coherent consolidation: {groups}")
            for i in range(1,11):
                if len(re.findall(rf"AC-{i:03d}\b", combined)) < 1:
                    raise SystemExit(f"handoff lost acceptance {i}")
            (artifacts / "handoff.json").write_text(json.dumps(
                {"milestones":10,"plans":2,"groups":[sorted(g) for g in groups]},indent=2))
            print(json.dumps({"milestones":10,"plans":2,"result":"passed"}))
        elif args.scope != "consolidate":
            count = 6 if args.scope == "small" else 40
            groups = 2 if args.scope == "small" else 8
            rows = []
            for i in range(count):
                group = i % groups
                rows.append(
                    f"## R-{i+1:03d} — Workflow {group+1}, action {i+1}\n"
                    f"Feature: F-{group+1:03d}; release: v1.\n"
                    f"Context: workflow {group+1} has its own state and independent delivery.\n"
                    f"Acceptance AC-{i+1:03d}: Running action {i+1} in workflow {group+1} "
                    f"prints receipt {i+1}.\n"
                )
            requirements = "# Requirements\n\n" + "\n".join(rows)
            (p / "docs/requirements.md").write_text(requirements)
            (p / "docs/features.md").write_text(
                "# Features\n" + "\n".join(
                    f"F-{i+1:03d}: independent workflow {i+1}" for i in range(groups)))
            (p / "docs/roadmap.md").write_text(
                "# Roadmap\n\n## v1\nFor: operators who run independent receipt workflows.\n"
                "Slice: every requirement belongs to v1.\n"
                "End-to-end check: run every defined action in every workflow; "
                "each prints its numbered receipt.\nNot yet: remote synchronization.\n"
                "## v2 and v3\nNo approved scope or release; write no milestones "
                "and ask no scope question for these releases.\n")
            (p / "docs/architecture.md").write_text(
                "# Architecture\n\nEach workflow owns an isolated state machine. "
                "A shared CLI dispatches workflow actions. No external dependencies.\n"
                "ADR-001: independent state machines isolate each workflow's receipts.\n")
            (p / "docs/engineering.md").write_text(
                "# Engineering\n\nGo stdlib CLI. make build and make test. "
                "Fast and standard tests verify receipt stdout. Target OS: macOS arm64. "
                "The binary is fixture-tool; syntax: fixture-tool workflow N action M. "
                "Workflow numbers and action numbers are the product's exact names. "
                "Each independent workflow has a distinct transition risk; workflow 1 "
                "has the highest risk and successive numbers have descending risk. "
                "Publication uses a release plan.\n")
            invoke(folder, "/product-milestones")
            milestone = p / "docs/milestones.md"
            if not milestone.exists():
                raise SystemExit("milestone skill wrote no document")
            text = milestone.read_text()
            (artifacts / (args.scope+"-attempt.md")).write_text(text)
            for i in range(1, count+1):
                if f"R-{i:03d}" not in text or f"AC-{i:03d}" not in text:
                    raise SystemExit(f"lost requirement or acceptance {i}")
            headings = re.findall(r"^###\s+(M\d+-\d+)", text, re.M)
            if not headings:
                raise SystemExit("milestone document has no milestone blocks")
            assigned = []
            for line in text.splitlines():
                if line.startswith("**Delivers:**"):
                    assigned += re.findall(r"\bR-\d+\b", line)
            expected = {f"R-{i:03d}" for i in range(1, count+1)}
            if set(assigned) != expected or any(v != 1 for v in collections.Counter(assigned).values()):
                raise SystemExit("milestone ownership is incomplete or duplicated")
            acceptance = []
            for line in text.splitlines():
                checkbox = re.match(r"^\s*- \[[ x]\]\s*(.*)$", line)
                if checkbox and re.match(r"^(?:\*\*|`)*(?:R-|AC-)\d+", checkbox.group(1)):
                    acceptance += re.findall(r"\bAC-\d+\b", checkbox.group(1).split(":",1)[0])
            if set(acceptance) != {f"AC-{i:03d}" for i in range(1, count+1)} or len(acceptance) != count:
                raise SystemExit("acceptance ownership is incomplete or duplicated")
            (artifacts / (args.scope+"-milestones.md")).write_text(text)
            (artifacts / (args.scope+"-result.json")).write_text(json.dumps(
                {"requirements": count, "milestones": len(set(headings))}, indent=2))
            print(json.dumps({"requirements":count,"milestones":len(set(headings))}))
        else:
            plans = p / ".claude/iterate/plans"
            archive = p / ".claude/iterate/archive"
            plans.mkdir(parents=True)
            originals = {}
            names = [f"checkpoint{i}" for i in range(10)]
            for i, name in enumerate(names):
                context = "intake lifecycle" if i < 5 else "export lifecycle"
                text = (
                    f"name: {name}\nphase: planned\nrunning: false\ndepends: none\n"
                    f"milestone: M1-{i+1:02d}\n"
                    f"## Goal\nR-{i+1:03d}: {context} checkpoint.\n"
                    f"## Steps\n- [ ] 1. Implement action {i+1} for {context}. [skill: none]\n"
                    f"## Validation\n- [ ] 1. AC-{i+1:03d}: action {i+1} prints receipt {i+1}.\n"
                    f"## Constraints\n- Shared implementation context: {context}.\n"
                    "- Acceptance checkpoints share implementation and delivery.\n"
                    "## Decisions log\n\n## Status / Log\n"
                )
                (plans / (name+".md")).write_text(text)
                originals[name] = text
            release = "name: publish\nphase: planned\nmode: release\ndepends: none\n## Goal\nPublication only.\n"
            (plans / "publish.md").write_text(release)
            invoke(folder, "/ip consolidate " + " ".join(names))
            active = [f for f in plans.glob("*.md") if f.name != "publish.md"]
            if not 0 < len(active) < 10:
                raise SystemExit(f"consolidation left {len(active)} execution plans")
            combined = "\n".join(f.read_text() for f in active)
            for i in range(1, 11):
                if len(re.findall(rf"AC-{i:03d}\b", combined)) != 1:
                    raise SystemExit(f"acceptance AC-{i:03d} missing or duplicated")
                if f"Implement action {i} " not in combined:
                    raise SystemExit(f"source task {i} lost")
            archived = [f.read_text() for f in archive.glob("*.md")]
            for name, text in originals.items():
                if text not in archived:
                    raise SystemExit(f"original {name} lacks a byte-preserved archive")
            if (plans / "publish.md").read_text() != release:
                raise SystemExit("release plan changed")
            result = subprocess.run([str(root / "bin/iterate-run"), "plan", "graph",
                                     "--check", "-C", folder], capture_output=True,text=True)
            if result.returncode:
                raise SystemExit(result.stdout+result.stderr)
            (artifacts / "consolidation.json").write_text(json.dumps(
                {"source_plans":10,"successors":len(active),"preserved":10},indent=2))
            print(json.dumps({"source_plans":10,"successors":len(active),"preserved":10}))


if __name__ == "__main__":
    main()
