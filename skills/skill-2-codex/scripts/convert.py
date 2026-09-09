#!/usr/bin/env python3
"""
convert.py — body-conversion half of the Claude Code -> Codex skill port.

Splits the step-4 body pass into two honest halves:

  SAFE   deterministic rewrites with exactly one correct answer. Applied
         automatically, every run, no judgment involved. Re-running on an
         already-converted file is a no-op.

  FLAG   constructs whose port is a genuine judgment call (what does this
         polling loop become? what shape should this picker take?). NEVER
         rewritten automatically -- reported, so a model or a human does that
         pass deliberately.

That split is what makes an unattended daily sync safe: the cron job owns the
SAFE half and can regenerate freely, while anything in the FLAG half is
surfaced rather than guessed at.

Usage: convert.py <codex-skill-dir> --all-skills <comma-separated names>
                  [--source <claude-skill-dir>] [--json]
"""
import argparse, json, os, re, sys, glob

# Constructs with no single correct port. Reported, never auto-rewritten.
FLAGS = {
    "loop":       (r'(?<![\w./-])/loop\b|\bCronCreate\b|\bCronList\b|\bCronDelete\b|\bScheduleWakeup\b',
                   "self-scheduling has NO Codex equivalent -- Cron*/ScheduleWakeup are Claude Code tools; "
                   "rewrite to ask the user for a Codex Automation or an OS cron running `codex exec`"),
    "askuser":    (r'\bAskUserQuestion\b',
                   "structured picker -> plain numbered prose list; no confirmed Codex equivalent"),
    "agenttool":  (r'\bAgent tool\b|\bsubagent_type\b|\brun_in_background\b',
                   "Agent-tool call shape -> plain-language subagent orchestration"),
    "notify":     (r'background-completion notification|completion notification',
                   "background completion notification -> Codex waits and returns consolidated; delete the poll"),
    "polltier":   (r'\bOverdue tier\b|\bStale\b.*\bFresh\b',
                   "notification/poll staleness tiers -> collapse; keep only crash recovery"),
}


def load_frontmatter(text):
    parts = text.split("---", 2)
    return (parts[1], parts[2]) if len(parts) >= 3 else ("", text)


def convert(path, all_skills, moved_refs):
    """Apply every SAFE rewrite. Returns (n_changes, flags_found)."""
    src = open(path).read()
    out = src

    # 1. /skill-name -> $skill-name. Longest first so /iterate-planner is not
    #    eaten by /iterate. Guarded on the left against path characters so
    #    ./.claude/iterate/plans/ is never touched, and on the right against
    #    name characters so /i does not match inside /importer.
    # Claude Code's orphan skills are invoked as "/-name"; Codex has no leading-
    # hyphen convention, so both "/-name" and "/name" land on "$name".
    for name in sorted(all_skills, key=len, reverse=True):
        bare = name.lstrip("-")
        if not bare:
            continue
        out = re.sub(r'(?<![A-Za-z0-9_./])/-' + re.escape(bare) + r'(?![A-Za-z0-9_-])',
                     '$' + bare, out)
        out = re.sub(r'(?<![A-Za-z0-9_./-])/' + re.escape(bare) + r'(?![A-Za-z0-9_-])',
                     '$' + bare, out)

    # 2. Claude Code's global skill dir -> Codex's.
    out = out.replace("~/.claude/skills/", "~/.agents/skills/")

    # 3. Links to supporting .md files that scaffold.sh relocated into references/.
    for ref in moved_refs:
        out = re.sub(r'\]\((?:\./)?' + re.escape(ref) + r'\)', f'](references/{ref})', out)

    # 4. Skill-tool delegation -> Codex explicit $name invocation.
    out = re.sub(r'[Ii]nvoke the Skill tool with skill `([a-z0-9-]+)`',
                 lambda m: f'Invoke `${m.group(1)}` explicitly', out)
    out = re.sub(r'`?Skill\((["\']?)([a-z0-9-]+)\1\)`?',
                 lambda m: f'`${m.group(2)}`', out)
    out = out.replace("the Skill tool", "explicit `$name` invocation")

    # 5. Skill-tool routing tables -> explicit $name invocation.
    out = re.sub(r'Skill tool:\s*`([a-z0-9-]+)`',
                 lambda m: f'invoke `${m.group(1)}` explicitly', out)

    # 6. AskUserQuestion -> plain-language ask. Codex documents no structured
    #    picker, so every one of these becomes an ordinary question. Ordered
    #    most-specific first so the general case never eats a phrasing that has
    #    a better rendering.
    # 6b. Claude Code's dual /loop-vs-cron cancel hazard does not exist in Codex,
    #     which has only cron -- so the warning becomes a plain verify step.
    out = out.replace(
        "(a cron needs CronDelete — a /loop stop will NOT kill it)",
        "(cancel with `CronDelete <job-id>`, then confirm with `CronList`)")

    ASK = [
        (r'\bAskUserQuestion, multiSelect\b',
         'ask the user to pick any number of them from a numbered list'),
        (r"[Dd]on't pop an `?AskUserQuestion`?", "don't stop to ask"),
        (r'\(no `?AskUserQuestion`?\)', '(do not ask)'),
        (r'\bOne-line `?AskUserQuestion`?', 'One-line question'),
        (r'\bone `?AskUserQuestion`?\b', 'one question'),
        (r'\b(via|using|through) `?AskUserQuestion`?',
         'by asking the user to choose from a short numbered list'),
        (r'\((?:AskUserQuestion)\)', '(ask the user to choose from a short numbered list)'),
        (r'\bAskUserQuestion\b', 'a plain numbered-list question'),
    ]
    had_ask = bool(re.search(r'\bAskUserQuestion\b', out))
    for pat, rep in ASK:
        out = re.sub(pat, rep, out)
    if had_ask and "codex-port: no confirmed structured-picker" not in out:
        marker = ("\n<!-- codex-port: no confirmed structured-picker equivalent in Codex; every "
                  "structured picker in this file became an ordinary numbered-list question -- "
                  "verify the wording reads naturally where it mattered. -->\n")
        lines = out.split("\n")
        h1 = next((i for i, l in enumerate(lines) if l.startswith("# ")), None)
        if h1 is not None:
            lines.insert(h1, marker)
            out = "\n".join(lines)

    changed = sum(1 for a, b in zip(src.split("\n"), out.split("\n")) if a != b)
    if out != src:
        open(path, "w").write(out)

    found = []
    for line_no, line in enumerate(out.split("\n"), 1):
        if "codex-port:" in line:
            continue
        for key, (pat, desc) in FLAGS.items():
            if re.search(pat, line):
                found.append({"flag": key, "line": line_no, "why": desc,
                              "file": path, "text": line.strip()[:140]})
    return changed, found


def add_sections(skill_dir, name, source_dir, all_skills, ported):
    """Fold the dropped argument-hint into a Usage section and list $-deps."""
    p = f"{skill_dir}/SKILL.md"
    body = open(p).read()
    if "## Dependencies" in body:
        return False  # already has them; re-runs must not stack duplicates

    hint = ""
    if source_dir and os.path.exists(f"{source_dir}/SKILL.md"):
        fm, _ = load_frontmatter(open(f"{source_dir}/SKILL.md").read())
        m = re.search(r'^argument-hint:\s*(.+)$', fm, re.M)
        if m:
            hint = m.group(1).strip()

    bare_skills = {x.lstrip("-") for x in all_skills}
    deps = sorted({d for d in re.findall(r'\$([a-z][a-z0-9-]*)', body)
                   if d in bare_skills and d != name.lstrip("-")})

    block = []
    if hint:
        block += ["", "## Usage", "",
                  f"Argument: {hint}. `$1` is its first word; `$ARGUMENTS` is the whole thing.",
                  "", "<!-- codex-port: `argument-hint` has no Codex frontmatter home; folded "
                  "into this Usage section. Argument substitution is documented for Codex custom "
                  "prompts but not for skills, so the meaning is stated in prose rather than left "
                  "to the token alone. -->"]
    if deps:
        block += ["", "## Dependencies", "",
                  "Invoked with Codex's explicit `$name` syntax. Each must also exist under "
                  "Codex's skill-discovery path or the call will not resolve:", ""]
        block += [f"- `${d}` — {'ported' if d in ported else '**not yet ported** — run `$skill-2-codex` on it'}."
                  for d in deps]
    if not block:
        return False

    lines = body.split("\n")
    h1 = next((i for i, l in enumerate(lines) if l.startswith("# ")), 4)
    j = h1 + 1
    while j < len(lines) and lines[j].strip() == "":
        j += 1
    while j < len(lines) and lines[j].strip() != "":
        j += 1
    lines[j:j] = block
    open(p, "w").write("\n".join(lines))
    return True


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("skill_dir")
    ap.add_argument("--all-skills", required=True)
    ap.add_argument("--source")
    ap.add_argument("--ported", default="")
    ap.add_argument("--json", action="store_true")
    a = ap.parse_args()

    all_skills = {s for s in a.all_skills.split(",") if s}
    ported = {s.lstrip("-") for s in a.ported.split(",") if s}
    name = os.path.basename(a.skill_dir.rstrip("/"))
    moved = {os.path.basename(f) for f in glob.glob(f"{a.skill_dir}/references/*.md")}

    total, flags = 0, []
    for f in [f"{a.skill_dir}/SKILL.md"] + sorted(glob.glob(f"{a.skill_dir}/references/*.md")):
        n, fl = convert(f, all_skills, moved)
        total += n
        flags += fl
    added = add_sections(a.skill_dir, name, a.source, all_skills, ported)

    res = {"skill": name, "lines_rewritten": total, "sections_added": added, "flags": flags}
    if a.json:
        print(json.dumps(res))
    else:
        print(f"{name}: {total} lines rewritten, "
              f"{'usage/deps added' if added else 'no sections needed'}, {len(flags)} flags")
        for f in flags:
            print(f"  FLAG[{f['flag']}] {os.path.basename(f['file'])}:{f['line']} — {f['why']}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
