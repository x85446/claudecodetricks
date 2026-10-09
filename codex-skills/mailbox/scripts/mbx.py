#!/usr/bin/env python3
"""mbx — the deterministic half of /mailbox. Machine output: tab-separated, one fact per line.

  mbx inbox list                     name<TAB>path for every registered inbox
  mbx inbox self                     this project's inbox (registers it on first use)
  mbx inbox resolve <name|path>      name<TAB>path[<TAB>near:<asked>]; exit 2 ambiguous, 3 none
  mbx inbox register [path] [--name N]
  mbx inbox seed                     add iterate-run's projects and every git repo under ~/workspace

  mbx msg list [--all]               id<TAB>status<TAB>kind<TAB>from<TAB>trust<TAB>title (open+held, this agent)
  mbx msg show <id>                  the message file
  mbx msg claim <id> [--steal]       exit 4 when another session holds it
  mbx msg status <id> <text>         rewrite the status line (releases the claim unless text starts "claimed")
  mbx msg send --to T --kind K --title "…" [--agent A] [--reply wanted|none]
               [--in-reply-to ID] [--hops N] [--slug S]  < body
                                     new|updated<TAB>target<TAB>path

Registry: ~/.claude/mailbox/registry.json. Inbox: <project>/.claude/iterate/inbox/<id>.md.
Sent copies: <project>/.claude/iterate/sent/<id>.md. Claims: <inbox>/.claims/<id>/.
"""
import argparse
import datetime as dt
import fcntl
import json
import os
import re
import subprocess
import sys
from pathlib import Path

HOME = Path.home()
REG = HOME / ".claude" / "mailbox" / "registry.json"
MAX_HOPS = 3
STALE_CLAIM = 24 * 3600
OPEN_STATES = ("open", "held")


def die(msg, code=1):
    print(msg, file=sys.stderr)
    sys.exit(code)


def utc():
    return dt.datetime.now(dt.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")


def git(path, *a):
    r = subprocess.run(["git", "-C", str(path), *a], capture_output=True, text=True)
    return r.stdout.strip() if r.returncode == 0 else ""


def project_root(path):
    p = Path(path).expanduser().resolve()
    top = git(p, "rev-parse", "--show-toplevel")
    return Path(top) if top else p


def agent():
    if os.environ.get("MAILBOX_AGENT"):
        return os.environ["MAILBOX_AGENT"]
    if os.environ.get("CLAUDECODE") == "1":
        return "claude"
    if any(k.startswith("CODEX_") for k in os.environ):
        return "codex"
    return "human"


def session_id():
    for k in ("CLAUDE_CODE_SESSION_ID", "CODEX_THREAD_ID", "CODEX_SESSION_ID"):
        if os.environ.get(k):
            return os.environ[k][:8]
    return str(os.getppid())


# ---------------------------------------------------------------- registry

def load_reg():
    try:
        return json.loads(REG.read_text())
    except (OSError, ValueError):
        return {"schema": 1, "inboxes": {}}


def save_reg(reg):
    REG.parent.mkdir(parents=True, exist_ok=True)
    tmp = REG.with_suffix(".tmp")
    tmp.write_text(json.dumps(reg, indent=2, sort_keys=True) + "\n")
    os.replace(tmp, REG)


class Locked:
    """flock around read-modify-write of the registry; sessions register concurrently."""
    def __enter__(self):
        REG.parent.mkdir(parents=True, exist_ok=True)
        self.f = open(REG.parent / "registry.lock", "w")
        fcntl.flock(self.f, fcntl.LOCK_EX)
        return load_reg()

    def __exit__(self, *exc):
        fcntl.flock(self.f, fcntl.LOCK_UN)
        self.f.close()


def add(reg, path, name=None, source="manual"):
    path = str(project_root(path))
    boxes = reg.setdefault("inboxes", {})
    for n, e in boxes.items():
        if e["path"] == path:
            if name and name != n:
                boxes[name] = boxes.pop(n)
                return name
            return n
    base = name or Path(path).name
    if base in boxes:  # same basename, different repo: qualify with the parent directory
        base = f"{Path(path).parent.name}/{Path(path).name}"
    boxes[base] = {"path": path, "registered": utc(), "source": source}
    return base


def seed_candidates():
    out = set()
    try:
        out.update(json.loads((HOME / ".claude/iterate-run/projects.json").read_text()))
    except (OSError, ValueError):
        pass
    r = subprocess.run(["find", str(HOME / "workspace"), "-mindepth", "2", "-maxdepth", "4",
                        "-name", ".git", "-prune"], capture_output=True, text=True)
    out.update(str(Path(p).parent) for p in r.stdout.split())
    # scratch projects under /tmp and copies in hidden dirs (.stversions, .codex/worktrees)
    # are not inboxes; both are judged below HOME so a HOME under /tmp still seeds
    def keep(p):
        h = str(HOME)
        rel = p[len(h):] if p.startswith(h + "/") else None
        if rel is None and re.match(r"^(/private)?/tmp/", p):
            return False
        return "/." not in (rel if rel is not None else p)
    return sorted(p for p in out if os.path.isdir(p) and keep(p))


def seed():
    with Locked() as reg:
        n0 = len(reg.get("inboxes", {}))
        for p in seed_candidates():
            add(reg, p, source="seed")
        save_reg(reg)
        return len(reg["inboxes"]) - n0


def ensure_seeded():
    if not REG.exists():
        seed()


def self_box():
    ensure_seeded()
    with Locked() as reg:
        name = add(reg, os.getcwd(), source="self")
        save_reg(reg)
        return name, reg["inboxes"][name]["path"]


def lev(a, b):
    prev = list(range(len(b) + 1))
    for i, ca in enumerate(a, 1):
        cur = [i]
        for j, cb in enumerate(b, 1):
            cur.append(min(prev[j] + 1, cur[j - 1] + 1, prev[j - 1] + (ca != cb)))
        prev = cur
    return prev[-1]


def resolve(q, reseed=True):
    """[(name, path, near)]: one row = resolved; several = ambiguous; none = unknown."""
    boxes = load_reg().get("inboxes", {})
    ql = q.lower()
    exact = [] if q.startswith(("/", "~", ".")) else [
        (n, e["path"], False) for n, e in boxes.items()
        if n.lower() == ql or Path(n).name.lower() == ql]
    if exact:
        return exact
    if q.startswith(("/", "~", ".")) or "/" in q:  # a path: register it on first use
        p = Path(q).expanduser()
        if not p.is_dir():
            return []
        with Locked() as reg:
            n = add(reg, p, source="manual")
            save_reg(reg)
            return [(n, reg["inboxes"][n]["path"], False)]
    near = [(n, e["path"], True) for n, e in boxes.items()
            if lev(Path(n).name.lower(), ql) <= 2]
    if not near:
        near = [(n, e["path"], True) for n, e in boxes.items()
                if Path(n).name.lower().startswith(ql) or ql in Path(n).name.lower()]
    if not near and reseed:
        seed()  # a repo cloned since the last seed
        return resolve(q, reseed=False)
    return near


# ---------------------------------------------------------------- messages

def inbox_dir(root):
    return Path(root) / ".claude" / "iterate" / "inbox"


def key_block(path):
    head = []
    for line in Path(path).read_text(errors="replace").splitlines():
        if line.startswith("## "):
            break
        head.append(line)
    return head


def fields(path):
    f = {"id": Path(path).stem, "kind": "inform", "title": "", "status": "", "from": "",
         "from_path": "", "to_agent": "any", "hops": "0", "reply": "none", "thread": "",
         "in-reply-to": ""}
    for line in key_block(path):
        m = re.match(r"^# (\w+) — (.*)", line)
        if m:
            f["kind"], f["title"] = m.group(1).lower(), m.group(2).strip()
            continue
        m = re.match(r"^([A-Za-z-]+):\s*(.*)$", line)
        if not m:
            continue
        k, v = m.group(1).lower(), m.group(2).strip()
        if k == "from":
            f["from"] = v.split(" ")[0]
            pm = re.search(r"\(([^)]+)\)", v)
            f["from_path"] = pm.group(1) if pm else ""
        elif k == "to":
            am = re.search(r"agent (\w+)", v)
            f["to_agent"] = am.group(1) if am else "any"
        elif k in ("status", "hops", "reply", "thread", "in-reply-to", "id"):
            f[k] = v
    return f  # a pre-mailbox inform has no reply: line, so it reads reply none


def msg_path(mid):
    root = project_root(os.getcwd())
    p = inbox_dir(root) / f"{mid}.md"
    if not p.exists():
        hits = sorted(inbox_dir(root).glob(f"*{mid}*.md"))
        if len(hits) != 1:
            die(f"no message {mid}" if not hits else f"ambiguous {mid}: " +
                " ".join(h.stem for h in hits), 3)
        p = hits[0]
    return p


def set_status(path, text):
    lines = Path(path).read_text().splitlines(keepends=True)
    for i, line in enumerate(lines):
        if line.startswith("## "):
            break
        if re.match(r"^status:", line):
            lines[i] = f"status: {text}\n"
            Path(path).write_text("".join(lines))
            return
    die(f"no status line in {path}")


def claim_dir(path):
    return Path(path).parent / ".claims" / Path(path).stem


def cmd_msg_list(a):
    name, root = self_box()
    paths = {e["path"] for e in load_reg().get("inboxes", {}).values()}
    me = agent()
    for p in sorted(inbox_dir(root).glob("*.md")):
        f = fields(p)
        state = f["status"].split(" ")[0]
        if not a.all and state not in OPEN_STATES:
            continue
        if not a.all and f["to_agent"] not in ("any", me):
            continue
        trust = "trusted" if f["from_path"] in paths else "unregistered"
        print("\t".join([f["id"], f["status"], f["kind"], f["from"], trust, f["title"]]))


def cmd_msg_claim(a):
    p = msg_path(a.id)
    d = claim_dir(p)
    d.parent.mkdir(parents=True, exist_ok=True)
    owner = f"{agent()} {session_id()} {utc()}"
    try:
        os.mkdir(d)  # atomic: exactly one session wins
    except FileExistsError:
        try:
            held = (d / "owner").read_text().strip()
        except OSError:
            held = "?"
        age = dt.datetime.now().timestamp() - d.stat().st_mtime
        if not (a.steal and age > STALE_CLAIM):
            die(f"claimed\t{held}", 4)
    (d / "owner").write_text(owner + "\n")
    set_status(p, f"claimed ({owner})")
    print(f"claimed\t{p.stem}")


def cmd_msg_status(a):
    p = msg_path(a.id)
    set_status(p, a.text)
    if not a.text.startswith("claimed"):
        d = claim_dir(p)
        for c in d.glob("*"):
            c.unlink()
        if d.exists():
            d.rmdir()
    print(f"status\t{p.stem}\t{a.text}")


def slugify(s):
    s = re.sub(r"[^a-z0-9]+", "-", s.lower()).strip("-")
    return "-".join(s.split("-")[:5]) or "message"


def cmd_msg_send(a):
    body = sys.stdin.read().strip()
    if not body:
        die("empty body on stdin")
    if a.hops > MAX_HOPS:
        die(f"hops {a.hops} > {MAX_HOPS}: a request chain this long is a loop, not work", 5)
    me_name, me_root = self_box()
    orig = None
    if a.in_reply_to:
        orig = fields(msg_path(a.in_reply_to))
    to = a.to or (orig and orig["from_path"]) or die("--to required")
    hits = resolve(to)
    if not hits:
        die(f"no inbox named {to} — give a path", 3)
    if len(hits) > 1:
        die("ambiguous\t" + "\t".join(n for n, _, _ in hits), 2)
    t_name, t_root, near = hits[0]
    if Path(t_root) == Path(me_root):
        die("that's this project", 6)
    kind = a.kind
    title = a.title.replace("\n", " ").strip()
    reply = a.reply or ("none" if kind == "reply" else "wanted")
    slug = a.slug or slugify(title)
    box = inbox_dir(t_root)
    box.mkdir(parents=True, exist_ok=True)
    # one problem, one open file: the same sender re-sending the same slug rewrites it
    target, verb = None, "new"
    if kind != "reply":
        for p in box.glob(f"*-{me_name.replace('/', '-')}-{slug}.md"):
            if fields(p)["status"].split(" ")[0] in OPEN_STATES:
                target, verb = p, "updated"
    stamp = dt.datetime.now(dt.timezone.utc).strftime("%Y%m%dT%H%M%SZ")
    mid = target.stem if target else f"{stamp}-{me_name.replace('/', '-')}-{slug}"
    thread = (orig["thread"] or orig["id"]) if orig else mid
    plan = ""
    try:
        plan = (Path(me_root) / ".claude/iterate/current").read_text().strip()
    except OSError:
        pass
    head = [f"# {kind.capitalize()} — {title}", "",
            f"From: {me_name} ({me_root}) · agent {agent()} · plan {plan or 'none'}"
            f" · branch {git(me_root, 'branch', '--show-current') or 'none'}"
            f" · commit {git(me_root, 'rev-parse', '--short=12', 'HEAD') or 'none'}",
            f"To: {t_name} · agent {a.agent}",
            f"Sent: {utc()}",
            f"id: {mid}",
            f"thread: {thread}"]
    if orig:
        head.append(f"in-reply-to: {orig['id']}")
    head += [f"reply: {reply}", f"hops: {a.hops}", "status: open", ""]
    text = "\n".join(head) + "\n" + body + "\n"
    out = box / f"{mid}.md"
    tmp = out.with_suffix(".tmp")
    tmp.write_text(text)
    os.replace(tmp, out)  # a reader never sees half a message
    sent = Path(me_root) / ".claude/iterate/sent"
    sent.mkdir(parents=True, exist_ok=True)
    (sent / f"{mid}.md").write_text(text)
    print("\t".join([verb, t_name + (f" (from \"{to}\")" if near else ""), str(out)]))


def main():
    ap = argparse.ArgumentParser(prog="mbx", description=__doc__,
                                 formatter_class=argparse.RawDescriptionHelpFormatter)
    sub = ap.add_subparsers(dest="noun", required=True)
    ib = sub.add_parser("inbox").add_subparsers(dest="verb", required=True)
    ib.add_parser("list")
    ib.add_parser("self")
    ib.add_parser("seed")
    r = ib.add_parser("resolve")
    r.add_argument("q")
    g = ib.add_parser("register")
    g.add_argument("path", nargs="?", default=".")
    g.add_argument("--name")
    ms = sub.add_parser("msg").add_subparsers(dest="verb", required=True)
    ls = ms.add_parser("list")
    ls.add_argument("--all", action="store_true")
    ms.add_parser("show").add_argument("id")
    c = ms.add_parser("claim")
    c.add_argument("id")
    c.add_argument("--steal", action="store_true")
    s = ms.add_parser("status")
    s.add_argument("id")
    s.add_argument("text")
    sd = ms.add_parser("send")
    sd.add_argument("--to")
    sd.add_argument("--kind", choices=["request", "inform", "reply"], default="request")
    sd.add_argument("--title", required=True)
    sd.add_argument("--agent", choices=["any", "claude", "codex"], default="any")
    sd.add_argument("--reply", choices=["wanted", "none"])
    sd.add_argument("--in-reply-to")
    sd.add_argument("--hops", type=int, default=0)
    sd.add_argument("--slug")
    a = ap.parse_args()

    if a.noun == "inbox":
        if a.verb == "list":
            ensure_seeded()
            for n, e in sorted(load_reg().get("inboxes", {}).items()):
                print(f"{n}\t{e['path']}")
        elif a.verb == "self":
            print("\t".join(self_box()))
        elif a.verb == "seed":
            print(f"added\t{seed()}")
        elif a.verb == "resolve":
            ensure_seeded()
            hits = resolve(a.q)
            if not hits:
                die(f"no inbox named {a.q}", 3)
            for n, p, near in hits:
                print("\t".join([n, p] + ([f"near:{a.q}"] if near else [])))
            sys.exit(2 if len(hits) > 1 else 0)
        elif a.verb == "register":
            if not Path(a.path).expanduser().is_dir():
                die(f"not a directory: {a.path}", 3)
            with Locked() as reg:
                n = add(reg, Path(a.path).expanduser(), a.name, "manual")
                save_reg(reg)
                print(f"{n}\t{reg['inboxes'][n]['path']}")
    else:
        {"list": cmd_msg_list, "claim": cmd_msg_claim, "status": cmd_msg_status,
         "send": cmd_msg_send, "show": lambda a: print(msg_path(a.id).read_text(), end="")}[a.verb](a)


if __name__ == "__main__":
    main()
