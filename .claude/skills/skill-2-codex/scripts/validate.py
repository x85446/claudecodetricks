#!/usr/bin/env python3
"""Gate every generated Codex port before it ships.

Codex parses frontmatter with strict YAML and charges every implicitly-invocable
skill against one catalog budget (see manifest.py for the cost model). Neither
failure is loud: a port whose YAML does not parse is skipped at load, and a
catalog over budget has its descriptions shortened without saying which. So
both are checked here, and both block the install.

Usage: validate.py <skills-root> [--budget TOKENS] [--json]
--budget defaults to `[skills] max_context_tokens` in ~/.codex/config.toml; with
neither, the budget is unknown and the gate fails. Exit 0 only when every port
parses, carries its required fields, and the catalog fits. Anything else exits 1
with the specific skills named.
"""
import argparse, glob, json, os, sys

import yaml

import manifest

REQUIRED = ("name", "description")
MIN_DESC = 40          # a description shorter than this routes nothing
YAML_INDICATORS = "*&!%@`>|{[#-?:,"


def frontmatter(path):
    txt = open(path, encoding="utf-8").read()
    if not txt.startswith("---"):
        return None, "no frontmatter block"
    parts = txt.split("---", 2)
    if len(parts) < 3:
        return None, "unterminated frontmatter block"
    try:
        fm = yaml.safe_load(parts[1])
    except yaml.YAMLError as e:
        first = str(e).splitlines()[0]
        return None, f"invalid YAML: {first}"
    if not isinstance(fm, dict):
        return None, "frontmatter is not a mapping"
    return fm, None


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("root")
    ap.add_argument("--budget", type=int, default=None)
    ap.add_argument("--json", action="store_true")
    a = ap.parse_args()

    budget = a.budget or manifest.configured_budget()
    problems, spenders = [], []
    if not budget:
        problems.append(("config", "no budget: set [skills] max_context_tokens in ~/.codex/config.toml"))
        budget = 0
    others = manifest.neighbours()
    total = sum(c for _, c in others)
    for path in sorted(glob.glob(os.path.join(a.root, "*", "SKILL.md"))):
        skill_dir = os.path.dirname(path)
        name = os.path.basename(skill_dir)
        fm, err = frontmatter(path)
        if err:
            problems.append((name, err))
            continue
        for field in REQUIRED:
            v = fm.get(field)
            if v is None or not str(v).strip():
                problems.append((name, f"missing field `{field}`"))
        desc = str(fm.get("description") or "").strip()
        if desc and len(desc) < MIN_DESC:
            problems.append((name, f"description is {len(desc)} chars — routes nothing"))
        # A plain scalar opening with a YAML indicator parses as something else
        # (`**Always` is an alias); it must have been quoted at write time.
        raw = ""
        for line in open(path, encoding="utf-8").read().split("---", 2)[1].splitlines():
            if line.startswith("description:"):
                raw = line[len("description:"):].strip()
                break
        if raw and raw[0] in YAML_INDICATORS and raw[0] not in "'\"":
            problems.append((name, f"description opens with `{raw[0]}` unquoted"))
        if manifest.is_implicit(skill_dir):
            spend = manifest.port_cost(name, desc)
            total += spend
            spenders.append((spend, name))

    ours = sum(c for c, _ in spenders)
    over = total - budget
    if a.json:
        print(json.dumps({"problems": problems, "manifest": total, "ports": ours,
                          "others": total - ours, "budget": budget, "unit": "tokens",
                          "fits": over <= 0}))
    else:
        for name, err in problems:
            print(f"BROKEN\t{name}\t{err}")
        status = "fits" if over <= 0 else f"OVER by {over}"
        print(f"manifest\t{total}/{budget} tokens\t{status}\tports {ours}, others {total - ours}")
        if over > 0:
            print("cut from the largest spenders:")
            for spend, name in sorted(spenders, reverse=True)[:5]:
                print(f"  {spend:5d}\t{name}")
    return 1 if (problems or over > 0) else 0


if __name__ == "__main__":
    sys.exit(main())
