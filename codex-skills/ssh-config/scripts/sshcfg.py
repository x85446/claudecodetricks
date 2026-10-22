#!/usr/bin/env python3
"""sshcfg — keep ~/.ssh/config.d to live site files only.

`~/.ssh/config` loads `~/.ssh/config.d/*`, so every file in that directory is
live configuration: a backup, an editor copy or a temp file there applies its
Host blocks, and options such as LocalForward accumulate across them. This
helper keeps the skill's own scratch outside the include glob and finds
anything else the glob loads.

Usage:
  sshcfg.py verify <host>          [--config <path>] [--json]
  sshcfg.py audit                  [--config <path>] [--json]
  sshcfg.py backup <site>          [--config <path>]
  sshcfg.py tempfile <site>        [--config <path>]
  sshcfg.py write <site> <file>    [--verify <host>] [--config <path>]
  sshcfg.py restore <site> [<backup>] [--config <path>]
  sshcfg.py dupes [<site>]         [--config <path>] [--json]
  sshcfg.py migrate                [--config <path>]
  sshcfg.py sites                  [--config <path>]

Flags parse in any position. Exit codes: 0 ok, 1 a check found a problem,
2 usage error, 3 ssh could not parse the configuration.
"""
import datetime
import fnmatch
import glob
import json
import os
import re
import shutil
import subprocess
import sys
import tempfile

KEEP_BACKUPS = 20
# Loaded files that are not site files: editor, backup, temp and sync copies.
JUNK_PATTERNS = ["*.bak", "*.bak.*", "*~", "*.tmp", "*.swp", "*.swo", "*.orig",
                 "*.rej", "*.sync-conflict-*", "#*#", "*.old", "*.save"]
JUNK_NAMES = {"old"}
HOST_RE = re.compile(r"^\s*Host\s+(.+?)\s*$", re.I)
INCLUDE_RE = re.compile(r"^\s*Include\s+(.+?)\s*$", re.I)


class UsageError(Exception):
    pass


def ssh_home():
    return os.path.join(os.path.expanduser("~"), ".ssh")


def parse_args(argv):
    opts = {"config": os.path.join(ssh_home(), "config"), "json": False, "verify": None}
    pos = []
    i = 0
    while i < len(argv):
        a = argv[i]
        if a == "--":
            pos.extend(argv[i + 1:])
            break
        if a in ("--config", "-F", "--verify"):
            if i + 1 >= len(argv):
                raise UsageError(f"{a} needs a value")
            opts["verify" if a == "--verify" else "config"] = argv[i + 1]
            i += 2
            continue
        if a.startswith("--config="):
            opts["config"] = a.split("=", 1)[1]
        elif a.startswith("--verify="):
            opts["verify"] = a.split("=", 1)[1]
        elif a == "--json":
            opts["json"] = True
        elif a in ("-h", "--help"):
            pos.insert(0, "help")
        elif a.startswith("-"):
            raise UsageError(f"unknown flag {a}")
        else:
            pos.append(a)
        i += 1
    return pos, opts


def include_dirs(config):
    """Directories whose files the config's Include globs load."""
    dirs = []
    try:
        lines = open(config).read().splitlines()
    except OSError as e:
        raise UsageError(f"cannot read {config}: {e}")
    for line in lines:
        m = INCLUDE_RE.match(line)
        if not m:
            continue
        for pat in m.group(1).split():
            pat = os.path.expanduser(pat)
            if not os.path.isabs(pat):
                pat = os.path.join(ssh_home(), pat)
            d = os.path.dirname(pat)
            if d not in dirs:
                dirs.append(d)
    return dirs


def site_dir(config):
    dirs = include_dirs(config)
    if not dirs:
        raise UsageError(f"{config} has no Include line")
    return dirs[0]


def backup_dir(config):
    d = site_dir(config).rstrip("/")
    return d + "-backups"


def loaded_files(config):
    """Every file the Include globs load, in ssh's order (glob(3) skips dotfiles)."""
    out = []
    for line in open(config).read().splitlines():
        m = INCLUDE_RE.match(line)
        if not m:
            continue
        for pat in m.group(1).split():
            pat = os.path.expanduser(pat)
            if not os.path.isabs(pat):
                pat = os.path.join(ssh_home(), pat)
            for f in sorted(glob.glob(pat)):
                if os.path.isfile(f) and f not in out:
                    out.append(f)
    return out


def is_junk(name):
    return name in JUNK_NAMES or any(fnmatch.fnmatch(name, p) for p in JUNK_PATTERNS)


def has_hosts(path):
    try:
        return any(HOST_RE.match(l) or l.strip().lower().startswith("match ")
                   for l in open(path, errors="replace"))
    except OSError:
        return False


def audit(config):
    """Loaded files that are not live site files and carry configuration."""
    flagged = []
    for f in loaded_files(config):
        name = os.path.basename(f)
        if os.path.getsize(f) == 0:
            continue
        if is_junk(name):
            flagged.append({"file": f, "reason": "backup, editor or temp copy loaded as live config"
                            + ("" if has_hosts(f) else " (no Host block)")})
    return flagged


def ssh_trace(config, host):
    p = subprocess.run(["ssh", "-G", "-vvv", "-F", config, host],
                       capture_output=True, text=True)
    if p.returncode != 0:
        return None, p.stderr.strip().splitlines()[-1:] or ["ssh -G failed"]
    applied = []
    for line in p.stderr.splitlines():
        m = re.match(r"debug\d: (.+) line (\d+): Applying options for (.+)$", line)
        if m:
            applied.append({"file": m.group(1), "line": int(m.group(2)), "pattern": m.group(3)})
    return applied, None


def cmd_verify(pos, opts):
    if len(pos) != 1:
        raise UsageError("verify needs exactly one host")
    host, config = pos[0], opts["config"]
    applied, err = ssh_trace(config, host)
    if applied is None:
        flagged = audit(config)
        report = {"host": host, "parsed": False, "error": err[0], "loaded_non_site": flagged}
        if opts["json"]:
            print(json.dumps(report, indent=2))
        else:
            print(f"FAIL\tssh -G {host}\t{err[0]}")
            for f in flagged:
                print(f"LOADED\t{f['file']}\t{f['reason']}")
        return 3
    junk = {f["file"] for f in audit(config)}
    applying = [a for a in applied if os.path.basename(a["file"]) and a["file"] in junk]
    flagged = audit(config)
    report = {"host": host, "parsed": True, "applied_from": sorted({a["file"] for a in applied}),
              "applying_non_site": applying, "loaded_non_site": flagged}
    if opts["json"]:
        print(json.dumps(report, indent=2))
    else:
        for a in applying:
            print(f"APPLIES\t{a['file']}:{a['line']}\tHost {a['pattern']}")
        for f in flagged:
            print(f"LOADED\t{f['file']}\t{f['reason']}")
        if not applying and not flagged:
            print(f"ok\t{host}\toptions come only from live site files")
    return 1 if (applying or flagged) else 0


def cmd_audit(pos, opts):
    flagged = audit(opts["config"])
    if opts["json"]:
        print(json.dumps({"loaded_non_site": flagged}, indent=2))
    else:
        for f in flagged:
            print(f"LOADED\t{f['file']}\t{f['reason']}")
        if not flagged:
            print("ok\tthe include glob loads only live site files")
    return 1 if flagged else 0


def stamp():
    return datetime.datetime.now(datetime.timezone.utc).strftime("%Y%m%dT%H%M%S%fZ")


def site_path(config, site):
    if "/" in site or site in ("", ".", ".."):
        raise UsageError(f"not a site name: {site!r}")
    return os.path.join(site_dir(config), site)


def prune(bdir, site):
    keep = sorted(f for f in os.listdir(bdir)
                  if f.startswith(site + ".") and not f.startswith(".")
                  and re.fullmatch(re.escape(site) + r"\.\d{8}T\d+Z(\.migrated)?", f))
    for old in keep[:-KEEP_BACKUPS]:
        os.remove(os.path.join(bdir, old))


def do_backup(config, site):
    src = site_path(config, site)
    if not os.path.exists(src):
        return None
    bdir = backup_dir(config)
    os.makedirs(bdir, exist_ok=True)
    dst = os.path.join(bdir, f"{site}.{stamp()}")
    shutil.copy2(src, dst)
    prune(bdir, site)
    return dst


def cmd_backup(pos, opts):
    if len(pos) != 1:
        raise UsageError("backup needs exactly one site")
    dst = do_backup(opts["config"], pos[0])
    if dst is None:
        print(f"skip\t{pos[0]}\tno site file to back up")
        return 0
    print(dst)
    return 0


def cmd_tempfile(pos, opts):
    if len(pos) != 1:
        raise UsageError("tempfile needs exactly one site")
    site_path(opts["config"], pos[0])
    bdir = backup_dir(opts["config"])
    os.makedirs(bdir, exist_ok=True)
    fd, path = tempfile.mkstemp(prefix=f".{pos[0]}.", suffix=".new", dir=bdir)
    os.close(fd)
    print(path)
    return 0


def install(config, site, source):
    """Atomically replace the site file with source's bytes, staged outside the glob."""
    dst = site_path(config, site)
    bdir = backup_dir(config)
    os.makedirs(bdir, exist_ok=True)
    fd, staged = tempfile.mkstemp(prefix=f".{site}.", suffix=".new", dir=bdir)
    with os.fdopen(fd, "wb") as out, open(source, "rb") as src:
        shutil.copyfileobj(src, out)
        out.flush()
        os.fsync(out.fileno())
    if os.path.exists(dst):
        shutil.copymode(dst, staged)
    os.replace(staged, dst)


def cmd_write(pos, opts):
    if len(pos) != 2:
        raise UsageError("write needs a site and a file")
    config, site, source = opts["config"], pos[0], pos[1]
    if os.path.dirname(os.path.abspath(source)) == site_dir(config):
        raise UsageError(f"{source} is inside the include glob; build the new file with 'sshcfg.py tempfile {site}'")
    backup = do_backup(config, site)
    install(config, site, source)
    if os.path.dirname(os.path.abspath(source)) == backup_dir(config) and os.path.basename(source).startswith("."):
        os.remove(source)
    if opts["verify"]:
        applied, err = ssh_trace(config, opts["verify"])
        if applied is None:
            if backup:
                install(config, site, backup)
            else:
                os.remove(site_path(config, site))
            print(f"FAIL\tssh -G {opts['verify']}\t{err[0]}\trestored {site} from {backup or 'nothing (new site removed)'}")
            return 3
    print(f"wrote\t{site}\tbackup {backup or 'none (new site)'}")
    return 0


def cmd_restore(pos, opts):
    if len(pos) not in (1, 2):
        raise UsageError("restore needs a site and optionally a backup file")
    config, site = opts["config"], pos[0]
    bdir = backup_dir(config)
    if len(pos) == 2:
        backup = pos[1] if os.path.isabs(pos[1]) else os.path.join(bdir, pos[1])
    else:
        cands = sorted(f for f in os.listdir(bdir) if re.fullmatch(re.escape(site) + r"\.\d{8}T\d+Z(\.migrated)?", f)) if os.path.isdir(bdir) else []
        if not cands:
            print(f"FAIL\t{site}\tno backup in {bdir}")
            return 1
        backup = os.path.join(bdir, cands[-1])
    install(config, site, backup)
    print(f"restored\t{site}\tfrom {backup}")
    return 0


def site_files(config):
    out = []
    for f in loaded_files(config):
        name = os.path.basename(f)
        if os.path.dirname(f) == site_dir(config) and not is_junk(name) and os.path.getsize(f) > 0:
            out.append(f)
    return out


def hosts_in(path):
    found = []
    for n, line in enumerate(open(path, errors="replace"), 1):
        m = HOST_RE.match(line)
        if m:
            for name in m.group(1).split():
                found.append((name, n))
    return found


def cmd_dupes(pos, opts):
    config = opts["config"]
    files = site_files(config)
    if pos:
        want = site_path(config, pos[0])
        if want not in files:
            raise UsageError(f"{pos[0]} is not a live site file")
    within, where = [], {}
    for f in files:
        seen = {}
        for name, line in hosts_in(f):
            seen.setdefault(name, []).append(line)
            where.setdefault(name, {}).setdefault(os.path.basename(f), []).append(line)
        if pos and f != site_path(config, pos[0]):
            continue
        for name, lines in seen.items():
            if len(lines) > 1:
                within.append({"site": os.path.basename(f), "host": name, "lines": lines})
    cross = [{"host": n, "sites": {s: l for s, l in sites.items()}}
             for n, sites in sorted(where.items()) if len(sites) > 1
             and (not pos or pos[0] in sites)]
    if opts["json"]:
        print(json.dumps({"within_site": within, "cross_site": cross}, indent=2))
    else:
        for d in within:
            print(f"DUPLICATE\t{d['site']}\tHost {d['host']}\tlines {', '.join(map(str, d['lines']))}")
        for d in cross:
            print(f"CROSS-SITE\tHost {d['host']}\t" + "; ".join(f"{s} line {', '.join(map(str, l))}" for s, l in d["sites"].items()))
        if not within and not cross:
            print("ok\tno duplicate Host names")
    return 0


def cmd_migrate(pos, opts):
    config = opts["config"]
    d, bdir, moved = site_dir(config), backup_dir(config), 0
    for f in sorted(glob.glob(os.path.join(d, "*.bak.skill"))):
        site = os.path.basename(f)[:-len(".bak.skill")]
        os.makedirs(bdir, exist_ok=True)
        when = datetime.datetime.fromtimestamp(os.path.getmtime(f), datetime.timezone.utc).strftime("%Y%m%dT%H%M%S%fZ")
        dst = os.path.join(bdir, f"{site}.{when}.migrated")
        shutil.move(f, dst)
        print(f"moved\t{f}\t{dst}")
        moved += 1
    if not moved:
        print("ok\tno skill backups inside the include glob")
    return 0


def cmd_sites(pos, opts):
    for f in site_files(opts["config"]):
        print(os.path.basename(f))
    return 0


CMDS = {"verify": cmd_verify, "audit": cmd_audit, "backup": cmd_backup, "tempfile": cmd_tempfile,
        "write": cmd_write, "restore": cmd_restore, "dupes": cmd_dupes, "migrate": cmd_migrate,
        "sites": cmd_sites}


def main(argv):
    try:
        pos, opts = parse_args(argv)
        if not pos or pos[0] == "help":
            print(__doc__.strip())
            return 0 if pos else 2
        verb = pos.pop(0)
        if verb not in CMDS:
            raise UsageError(f"unknown verb {verb}")
        return CMDS[verb](pos, opts)
    except UsageError as e:
        print(f"usage: {e}", file=sys.stderr)
        return 2


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
