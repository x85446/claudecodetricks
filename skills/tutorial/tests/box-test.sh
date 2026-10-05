#!/usr/bin/env bash
# box-test.sh — render test for the tutorial runtime's boxes and menu.
#
# Builds fixture tutorials from the lib under test, drives run.sh and a bucket
# under a real pty at 60, 80 and 132 columns, and checks:
#
#   box-1  every frame line (menu, title, done) has the same display width
#          with ANSI stripped, and ends in the right border
#   box-2  a row wider than the terminal wraps inside the frame; no line is
#          wider than the terminal
#   box-3  WALLCLOCK set → "~45 min running (10 hands-on)"
#   box-4  WALLCLOCK absent → "4 min", starting in the same column as box-3's
#   box-5  NO_COLOR, TERM=dumb and a piped stdout give no escapes, frame intact
#   box-6  a non-UTF-8 locale gives an ASCII frame
#   box-7  an empty tutorial directory gives a closed frame with the
#          placeholder row
#
#   bash skills/tutorial/tests/box-test.sh            # the lib beside it
#   TUT_LIB=/path/to/lib bash .../box-test.sh         # any other copy
#
# Exits 0 when all seven pass, 1 otherwise. Needs python3 for the pty.
#
# TESTMASTER: id=box-1 tier=? parallel=yes
# TESTMASTER: id=box-2 tier=? parallel=yes
# TESTMASTER: id=box-3 tier=? parallel=yes
# TESTMASTER: id=box-4 tier=? parallel=yes
# TESTMASTER: id=box-5 tier=? parallel=yes
# TESTMASTER: id=box-6 tier=? parallel=yes
# TESTMASTER: id=box-7 tier=? parallel=yes

set -uo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
LIB="${TUT_LIB:-$HERE/../lib}"
WORK="$(mktemp -d "${TMPDIR:-/tmp}/box-test.XXXXXX")"
trap 'rm -rf "$WORK"' EXIT

mkdir -p "$WORK/full" "$WORK/empty"
for d in full empty; do
    for f in run.sh tutorial.sh box.sh; do
        [ -f "$LIB/$f" ] && cp "$LIB/$f" "$WORK/$d/"
    done
done

cat > "$WORK/full/01-long-run.sh" <<'EOF'
#!/usr/bin/env bash
# TUTORIAL-TITLE: Building the image end to end
# TUTORIAL-MINUTES: 10
# TUTORIAL-WALLCLOCK: 45
. "$(dirname "$0")/tutorial.sh"
tut_title "Building the image" "This subtitle is deliberately long so that it is wider than even a one-hundred-and-thirty-two column terminal and has to wrap inside the frame"
tut_step "Say hi" "a trivial command"
tut_run "echo hi"
tut_done "run bucket 2 next, which is the quick look at the result"
EOF
cat > "$WORK/full/02-quick.sh" <<'EOF'
#!/usr/bin/env bash
# TUTORIAL-TITLE: Quick look
# TUTORIAL-MINUTES: 4
. "$(dirname "$0")/tutorial.sh"
tut_title "Quick look"
tut_step "Fail once" "shows the failure row"
tut_run "false"
tut_done "fix it"
EOF
chmod +x "$WORK"/*/*.sh

WORK="$WORK" python3 - <<'PY'
import os, pty, re, select, struct, sys, time, fcntl, termios, subprocess

WORK = os.environ["WORK"]
ANSI = re.compile(r"\x1b\[[0-9;?]*[A-Za-z]")
MID = "│|"
END = {"╭": "╮", "+": "+", "│": "│", "|": "|", "╰": "╯"}

def base_env(**over):
    env = {k: v for k, v in os.environ.items()
           if k not in ("COLUMNS", "LINES", "NO_COLOR", "FORCE_COLOR", "CLICOLOR_FORCE")}
    env.update(TERM="xterm-256color", LANG="en_US.UTF-8")
    env.pop("LC_ALL", None); env.pop("LC_CTYPE", None)
    env.update(over)
    return env

def run_pty(cols, argv, keys=b"", **envover):
    pid, fd = pty.fork()
    if pid == 0:
        os.execvpe(argv[0], argv, base_env(**envover))
    fcntl.ioctl(fd, termios.TIOCSWINSZ, struct.pack("HHHH", 40, cols, 0, 0))
    out, sent, t0 = b"", False, time.time()
    while time.time() - t0 < 30:
        r, _, _ = select.select([fd], [], [], 0.4)
        if r:
            try:
                d = os.read(fd, 65536)
            except OSError:
                break
            if not d:
                break
            out += d
        elif not sent:
            if keys:
                os.write(fd, keys)
            sent = True
    try:
        os.kill(pid, 9)
    except ProcessLookupError:
        pass
    os.waitpid(pid, 0)
    return out.decode("utf-8", "replace").replace("\r", "")

def run_pipe(argv, keys=b"", **envover):
    p = subprocess.run(argv, input=keys, capture_output=True, env=base_env(**envover), timeout=30)
    return p.stdout.decode("utf-8", "replace")

def plain(s):
    return ANSI.sub("", s)

def boxes(text):
    """Group frame lines into boxes: a top line, its rows, a bottom line."""
    out, cur = [], None
    for line in plain(text).split("\n"):
        line = line.rstrip("\n")
        if cur is not None and line.startswith(("╰─", "+-")):
            cur.append(line); out.append(cur); cur = None
        elif cur is None and line.startswith(("╭─", "+-")):
            cur = [line]
        elif cur is not None and line[:1] in tuple(MID):
            cur.append(line)
        elif cur is not None:
            out.append(cur + ["<unclosed>"]); cur = None
    if cur is not None:
        out.append(cur + ["<unclosed>"])
    return out

def closed(box):
    """Every line equal width and ending in the border its first char calls for."""
    if box[-1] == "<unclosed>" or len(box) < 2:
        return False, "frame never closed: " + repr(box[0])
    widths = {len(l) for l in box}
    if len(widths) != 1:
        return False, f"widths {sorted(widths)} in box {box[0]!r}"
    for l in box:
        if l[-1] != END.get(l[0], "?"):
            return False, f"line does not end in its border: {l!r}"
    return True, ""

results = {}
def check(case, ok, why=""):
    results.setdefault(case, "")
    if not ok and not results[case]:
        results[case] = why

full = os.path.join(WORK, "full")
empty = os.path.join(WORK, "empty")
run = os.path.join(full, "run.sh")

for cols in (60, 80, 132):
    menu = run_pty(cols, [run], b"q\n")
    bucket = run_pty(cols, ["env", "TUT_AUTO=1", run, "1"])
    failing = run_pty(cols, ["env", "TUT_AUTO=1", run, "2"])
    for name, text, want in (("menu", menu, 1), ("bucket 1", bucket, 2), ("bucket 2", failing, 2)):
        bs = boxes(text)
        check("box-1", len(bs) >= want, f"{cols} cols {name}: {len(bs)} boxes found, want {want}")
        for b in bs:
            ok, why = closed(b)
            check("box-1", ok, f"{cols} cols {name}: {why}")
            for l in b:
                check("box-2", len(l) <= cols, f"{cols} cols {name}: line of {len(l)} > {cols}: {l!r}")
    title = [b for b in boxes(bucket) if "Building the image" in b[0]]
    check("box-2", bool(title) and len(title[0]) >= 4,
          f"{cols} cols: the long subtitle did not wrap into 2+ rows")
    pm = plain(menu)
    check("box-3", "~45 min running (10 hands-on)" in pm, f"{cols} cols: no '~45 min running (10 hands-on)'")
    lines = pm.split("\n")
    q = next((k for k, l in enumerate(lines) if "Quick look" in l), None)
    near = lines[q:q + 2] if q is not None else []
    r2 = next((l for l in near if re.search(r"(?<![~\d])4 min", l)), "")
    check("box-4", bool(r2), f"{cols} cols: no '4 min' on or under the Quick look row")
    r1 = next((l for l in lines if "~45 min" in l), "")
    c1, c2 = r1.find("~45 min"), re.search(r"(?<![~\d])4 min", r2).start() if r2 else -1
    check("box-4", c1 > 0 and c1 == c2, f"{cols} cols: durations start at columns {c1} and {c2}")

for label, text in (
    ("NO_COLOR=1", run_pty(80, [run], b"q\n", NO_COLOR="1")),
    ("TERM=dumb", run_pty(80, [run], b"q\n", TERM="dumb")),
    ("piped", run_pipe([run], b"q\n")),
    ("piped bucket", run_pipe([run, "2"], b"", TUT_AUTO="1")),
):
    check("box-5", "\x1b" not in text, f"{label}: escape sequences present")
    bs = boxes(text)
    check("box-5", bool(bs) and all(closed(b)[0] for b in bs), f"{label}: frame not intact")

c_locale = run_pty(80, [run], b"q\n", LANG="C", LC_ALL="C")
bs = boxes(c_locale)
check("box-6", bool(bs) and bs[0][0].startswith("+-") and "╭" not in c_locale,
      "LANG=C: frame is not ASCII")
check("box-6", bool(bs) and all(closed(b)[0] for b in bs), "LANG=C: frame not closed")

none = run_pty(80, [os.path.join(empty, "run.sh")], b"q\n")
bs = boxes(none)
check("box-7", bool(bs) and closed(bs[0])[0], "empty dir: frame not closed")
check("box-7", "(no tutorials yet" in plain(none), "empty dir: no placeholder row")

failed = 0
for case in sorted(results):
    why = results[case]
    if why:
        failed += 1
        print(f"{case}: FAIL — {why}")
    else:
        print(f"{case}: pass")
sys.exit(1 if failed else 0)
PY
