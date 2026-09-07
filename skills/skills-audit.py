#!/usr/bin/env python3
"""
skills-audit.py — registry auditor for every skill on this machine.

Policy: claudecodetricks/skills/ is the registry of record. Every FIRST-PARTY
skill on this system should have an entry here.

Finding SKILL.md files is trivial; there are hundreds. The work is telling apart
the skills you wrote from the ones that merely landed on disk, and telling real
source drift apart from runtime data.

CLASSES (priority order):
  GENERATED  build output (~/.agents, codex-skills). Regenerate, never back up.
  VENDORED   someone else's: marketplaces, plugin caches, third-party bundles.
  COPY       a first-party-looking path whose skill NAME also exists under a
             vendored path -- a vendored skill copied into a project. Not ours.
  BACKUP     a .bk/.bk2/.temp/.old copy of a tree already covered.
  FIRST      authored in one of your own repos. THESE must be registered.

DRIFT is only reported for files the registry actually tracks. A skill writes
state next to itself at runtime -- oracle/known.md, accounts/known.md,
incus/routing.md -- and those living only in the install is correct, not drift.
Reporting them would bury the one finding that matters under noise that never
resolves.

Usage: skills-audit.py [--adopt] [--json] [--include-copies]
"""
import argparse, collections, json, os, re, shutil, subprocess, sys

H = os.path.expanduser("~")
REPO = f"{H}/workspace/x85446/claudecodetricks"
REG = f"{REPO}/skills"

VENDORED_MARKERS = [
    "/plugins/marketplaces/", "/claude-market-place/", "/claude-things/",
    "/.claude/plugins/", "/3rd-party-Skills/", "/claudekit-skills",
    "/claude-plugins-official/", "/external_plugins/", "/plugins/plugin-dev",
    "/node_modules/", "/.cursor/", "/skills-paused/",
]
GENERATED_MARKERS = ["/.agents/skills/", "/codex-skills/", "/.claude/skills/.backups/"]
BACKUP_RE = re.compile(r'(\.bk\d*|\.backup|\.old|\.temp|\.orig)(/|$)')

# Files a skill writes at runtime. Their absence from the registry is correct.
RUNTIME_NAMES = {"known.md", "routing.md", "findings.md", "registry.json",
                 "catalog.json", "notes.md", "state.json"}

# Written into a registry entry at adoption time, recording the repo that owns
# the skill. Without it the registry cannot tell a skill it authored from one it
# is merely mirroring, and drift has no direction -- which is exactly how
# izmachine's Codex port ended up built from a 3-day-stale copy.
ORIGIN = ".origin"

# A SKILL.md sitting at a repository root does not make that repository a skill.
# voicemode keeps its SKILL.md at the top of its repo; adopting by "the directory
# containing SKILL.md" pulled in 1.2 GB and a nested .git before this guard
# existed. A real skill directory holds SKILL.md and skill-shaped siblings only.
# Keying on build files is wrong: a skill may legitimately ship a small program
# that implements it (the code-factory skills are SKILL.md + a 16 KB Rust bin).
# What actually distinguishes a repository is its own .git and its size.
MAX_SKILL_KB = 5 * 1024


def looks_like_skill_dir(d):
    """(ok, reason). Rejects repository roots, accepts skills that ship code."""
    try:
        entries = set(os.listdir(d))
    except OSError as e:
        return False, str(e)
    if ".git" in entries:
        return False, "repository root (has its own .git)"
    kb = 0
    for root, dirs, files in os.walk(d):
        dirs[:] = [x for x in dirs if x not in {".git", "node_modules", ".venv", "target"}]
        for f in files:
            fp = os.path.join(root, f)
            if os.path.islink(fp) or not os.path.exists(fp):
                continue
            kb += os.path.getsize(fp) / 1024
            if kb > MAX_SKILL_KB:
                return False, f"too large for a skill (>{MAX_SKILL_KB//1024} MB)"
    return True, ""


def read_origin(reg_entry):
    p = os.path.join(reg_entry, ORIGIN)
    if not os.path.exists(p):
        return None
    return open(p).read().strip().replace("~", H, 1)


def newer_side(reg_entry, live):
    """Which side has the newer SKILL.md. Reported, never acted on."""
    a = os.path.getmtime(os.path.join(reg_entry, "SKILL.md"))
    b = os.path.getmtime(os.path.join(live, "SKILL.md"))
    if abs(a - b) < 2:
        return "same-time"
    return "REGISTRY newer" if a > b else "LIVE newer"


def base_class(path):
    if any(m in path for m in GENERATED_MARKERS):
        return "GENERATED"
    if any(m in path for m in VENDORED_MARKERS):
        return "VENDORED"
    if BACKUP_RE.search(path):
        return "BACKUP"
    return "FIRST"


def find_all():
    out = subprocess.run(
        ["find", f"{H}/workspace", f"{H}/.claude/skills", "-name", "SKILL.md",
         "-not", "-path", "*/node_modules/*", "-not", "-path", "*/.git/*",
         "-not", "-path", "*/target/*"],
        capture_output=True, text=True).stdout.split()
    return [os.path.dirname(p) for p in out]


def tracked_diff(reg_dir, live_dir):
    """Differences in files the registry tracks. Runtime-only files ignored."""
    out = []
    for root, _, files in os.walk(reg_dir):
        for f in files:
            if f in RUNTIME_NAMES or f in {".portstamp", ORIGIN, ".DS_Store"}:
                continue
            rp = os.path.relpath(os.path.join(root, f), reg_dir)
            live = os.path.join(live_dir, rp)
            if not os.path.exists(live):
                out.append(f"missing in live: {rp}")
            elif subprocess.run(["diff", "-q", os.path.join(root, f), live],
                                capture_output=True).returncode:
                out.append(f"differs: {rp}")
    return out


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--adopt", action="store_true")
    ap.add_argument("--json", action="store_true")
    ap.add_argument("--include-copies", action="store_true")
    a = ap.parse_args()

    reg = {n for n in os.listdir(REG) if os.path.isdir(f"{REG}/{n}")}
    reg_bare = {n.lstrip("-"): n for n in reg}

    dirs = [d for d in find_all() if not d.startswith(REG + "/")]
    cls = {d: base_class(d) for d in dirs}
    vendored_names = {os.path.basename(d) for d, c in cls.items() if c == "VENDORED"}
    for d, c in list(cls.items()):
        if c == "FIRST" and os.path.basename(d) in vendored_names:
            cls[d] = "COPY"

    buckets = collections.defaultdict(list)
    for d, c in cls.items():
        buckets[c].append(d)

    scope = list(buckets["FIRST"]) + (list(buckets["COPY"]) if a.include_copies else [])
    missing, drifted, ok = collections.defaultdict(list), [], 0
    for d in scope:
        n = os.path.basename(d)
        if n.lstrip("-") in reg_bare:
            entry = f"{REG}/{reg_bare[n.lstrip('-')]}"
            origin = read_origin(entry)
            # If the entry records an origin, only that path can drift it. A
            # stale copy installed into some other project is not the registry's
            # problem and reporting it would never resolve.
            if origin and os.path.realpath(origin) != os.path.realpath(d):
                ok += 1
                continue
            diffs = tracked_diff(entry, d)
            if diffs:
                drifted.append((n, d, diffs, newer_side(entry, d)))
            else:
                ok += 1
        else:
            missing[n].append(d)

    if a.json:
        print(json.dumps({
            "counts": {k: len(v) for k, v in buckets.items()},
            "in_sync": ok, "drifted": len(drifted), "unregistered": len(missing),
            "unregistered_names": sorted(missing),
            "drifted_detail": [{"skill": n, "path": p.replace(H, "~"),
                                "diffs": d, "newer": s_}
                               for n, p, d, s_ in drifted]}, indent=2))
        return 0

    print(f"registry: ~{REG[len(H):]}  ({len(reg)} skills)\n")
    for k in ("FIRST", "COPY", "VENDORED", "BACKUP", "GENERATED"):
        print(f"  {k:<10} {len(buckets[k]):4d}")
    print(f"\nfirst-party:  {ok} in sync   {len(drifted)} drifted   "
          f"{len(missing)} UNREGISTERED\n")

    if drifted:
        print("DRIFTED (tracked files only — runtime state ignored):")
        for n, d, diffs, side in sorted(drifted):
            print(f"  {n:<28} [{side}]  {d.replace(H,'~')}")
            for x in diffs[:3]:
                print(f"      {x}")
        print()
    if missing:
        byrepo = collections.defaultdict(list)
        for n, ds in missing.items():
            root = ds[0].replace(H, "~")
            for sep in ("/.claude/skills/", "/.claude/skillet/", "/skills/"):
                if sep in root:
                    root = root.split(sep)[0]
                    break
            byrepo[root].append(n)
        print(f"UNREGISTERED first-party ({len(missing)} skills, "
              f"{len(byrepo)} repos):")
        for repo, names in sorted(byrepo.items(), key=lambda kv: -len(kv[1])):
            print(f"  {repo}  ({len(names)})")
            print(f"      {', '.join(sorted(names))}")

    if a.adopt:
        n_ad, skipped = 0, []
        for n, ds in sorted(missing.items()):
            # A generic directory name would collide with any real skill later
            # and tells a reader nothing. Register it deliberately, not in bulk.
            if n in {"skill", "skills", "example-skill", "template-skill"}:
                skipped.append((n, "generic name")); continue
            good, why = looks_like_skill_dir(ds[0])
            if not good:
                skipped.append((n, why)); continue
            dst = f"{REG}/{n}"
            if os.path.exists(dst):
                continue
            shutil.copytree(ds[0], dst, symlinks=True, ignore_dangling_symlinks=True)
            with open(f"{dst}/{ORIGIN}", "w") as f:
                f.write(ds[0].replace(H, "~") + "\n")
            n_ad += 1
        print(f"\nadopted {n_ad} into the registry (each stamped with .origin)")
        if skipped:
            print(f"NOT adopted ({len(skipped)}) — register by hand if wanted:")
            for n, why in sorted(skipped):
                print(f"  {n:<34} {why}")
    elif missing:
        print(f"\n(dry run — --adopt registers these {len(missing)})")
    return 0


if __name__ == "__main__":
    sys.exit(main())
