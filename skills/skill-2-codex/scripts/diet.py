#!/usr/bin/env python3
"""
diet.py — fit the ported skills inside Codex's startup manifest budget.

Codex loads three levels: the manifest (name + description) at startup, the
SKILL.md body on trigger, and references/ on demand. Only the first is charged
against the 2%-of-context / 8,000-char budget.

The description cannot be relocated -- it is the only routing signal Codex has,
and the spec provides no trigger file. But descriptions written for Claude
Code's 16,000-char budget routinely carry material that was never routing
signal: what the skill does, where it stores state, which rules it enforces.
That belongs one level down, in the body, where it is read at the moment it
matters instead of being paid for in every session.

So this moves documentation OUT of the description and INTO the body. Nothing is
deleted; text changes level. Trigger phrases are never touched.

Conservative by construction:
  - a sentence is kept in the description on ANY hint of routing signal
  - a skill with no clearly-trigger sentence is left completely alone
  - skills are trimmed largest-payload-first and only until the budget is met,
    so most are never touched at all
  - explicit-only skills cost nothing and are skipped entirely

Usage: diet.py <skills-root> [--budget 8000] [--apply] [--json]
"""
import argparse, glob, json, os, re, sys

# Any of these in a sentence means it carries routing signal -- keep it.
TRIGGER = re.compile(
    r'"'                                   # a quoted trigger phrase
    r'|\btrigger'
    r'|\buse (when|whenever|for|it for|this when)\b'
    r'|\balways invoke\b|\buse immediately\b'
    r'|\basks? (to|about|for|whether)\b'
    r'|\bphrases? include\b'
    r'|\broute .{0,30}here\b'
    r'|\bfires? on\b|\binvoked? (with|as)\b'
    # Negative scope is routing signal too -- the spec asks a description to say
    # when the skill should AND should not trigger, so "out of scope" earns its
    # place in the manifest exactly as much as a trigger phrase does.
    r'|\bout of scope\b|\bnot for\b|\bnever use\b|\bdo(es)? not (use|apply|cover)\b'
    r'|\$[a-z][a-z0-9-]*',                 # explicit $name self-reference
    re.I)

HEADING = "## What this skill does"


def split_description(desc):
    """Return (keep, move) -- sentences that route vs sentences that document."""
    keep, move = [], []
    for s in re.split(r'(?<=[.!?])\s+', desc.strip()):
        if not s.strip():
            continue
        (keep if TRIGGER.search(s) else move).append(s.strip())
    return keep, move


def read_desc(path):
    txt = open(path).read()
    parts = txt.split("---", 2)
    if len(parts) < 3:
        return None, None, None
    m = re.search(r'^description: (.+)$', parts[1], re.M)
    return (m.group(1).strip() if m else None), parts, txt


def cost(path):
    d, _, _ = read_desc(path)
    return len(os.path.basename(os.path.dirname(path))) + len(d or "")


def is_implicit(skill_dir):
    y = os.path.join(skill_dir, "agents", "openai.yaml")
    return not (os.path.exists(y) and "allow_implicit_invocation: false" in open(y).read())


def apply_diet(path, move):
    """Relocate `move` sentences from the description into the body."""
    desc, parts, txt = read_desc(path)
    keep = [s for s in re.split(r'(?<=[.!?])\s+', desc.strip())
            if s.strip() and s.strip() not in move]
    new_desc = " ".join(keep)
    fm = re.sub(r'^description: .+$', "description: " + new_desc, parts[1], count=1, flags=re.M)
    body = parts[2]

    para = " ".join(move)
    if HEADING in body:
        body = re.sub(re.escape(HEADING) + r'\n\n.*?(?=\n\n)',
                      f"{HEADING}\n\n{para}", body, count=1, flags=re.S)
    else:
        lines = body.split("\n")
        h1 = next((i for i, l in enumerate(lines) if l.startswith("# ")), None)
        ins = (h1 + 1) if h1 is not None else 0
        while ins < len(lines) and lines[ins].strip() == "":
            ins += 1
        while ins < len(lines) and lines[ins].strip() != "":
            ins += 1
        lines[ins:ins] = ["", HEADING, "",
                          "<!-- codex-port: moved out of the startup description, which is "
                          "charged against Codex's manifest budget in every session. This text "
                          "is documentation, not routing signal, so it belongs at the body "
                          "level where it loads on trigger. No trigger phrase was moved. -->",
                          "", para]
        body = "\n".join(lines)
    open(path, "w").write("---" + fm + "---" + body)


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("root")
    ap.add_argument("--budget", type=int, default=8000)
    ap.add_argument("--apply", action="store_true")
    ap.add_argument("--json", action="store_true")
    a = ap.parse_args()

    skills = [f for f in sorted(glob.glob(f"{a.root}/*/SKILL.md"))
              if is_implicit(os.path.dirname(f))]
    before = sum(cost(f) for f in skills)

    # Largest documentation payload first, so the fewest skills are touched.
    cands = []
    for f in skills:
        d, _, _ = read_desc(f)
        if not d:
            continue
        keep, move = split_description(d)
        if not keep or not move:      # nothing safe to move, or nothing to keep
            continue
        cands.append((sum(len(s) + 1 for s in move), f, move))
    cands.sort(reverse=True)

    total, touched = before, []
    for payload, f, move in cands:
        if total <= a.budget:
            break
        if a.apply:
            apply_diet(f, move)
        total -= payload
        touched.append({"skill": os.path.basename(os.path.dirname(f)),
                        "moved_chars": payload, "sentences": len(move)})

    res = {"before": before, "after": total, "budget": a.budget,
           "fits": total <= a.budget, "touched": touched,
           "untouched": len(skills) - len(touched), "applied": a.apply}
    if a.json:
        print(json.dumps(res))
    else:
        print(f"manifest {before} -> {total} / {a.budget}  "
              f"({'fits' if res['fits'] else 'STILL OVER by ' + str(total - a.budget)})")
        print(f"trimmed {len(touched)} skills, left {res['untouched']} untouched"
              f"{'' if a.apply else '   [dry run — pass --apply]'}")
        for t in touched:
            print(f"  -{t['moved_chars']:5d}  {t['skill']:<28} ({t['sentences']} sentences to body)")
    return 0


if __name__ == "__main__":
    sys.exit(main())
