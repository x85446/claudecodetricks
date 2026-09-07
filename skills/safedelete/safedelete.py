#!/usr/bin/env python3
"""
safedelete — prove that every page of a set of ORIGINAL PDFs is present, by
CONTENT, in a set of KEEPER PDFs, before deleting the originals. "Content" =
md5 of each rendered page image (pdftoppm -gray) with a normalized-text md5
fallback. This survives the byte-level re-encoding that qpdf / pdfseparate /
pdfunite perform, so an identical page still matches.

Verify only (default):
    safedelete.py <folder> [--bundle-name _original-bundles]
        originals = <folder>/<bundle-name>   (the delete candidate)
        keepers   = every *.pdf under <folder> NOT inside the originals folder
    safedelete.py --originals <file|dir> --against <file|dir> [<file|dir> ...]

Delete (only if SAFE, and only what verified): add --trash
    Moves the originals to the macOS Trash (recoverable). Refuses on any
    UNMATCHED page. Never hard-deletes.

Exit codes: 0 = SAFE (all original pages covered), 2 = NOT SAFE, 1 = error.
Requires: pdftoppm, pdftotext, pdfinfo (poppler); qpdf optional (for encrypted).
"""
import os, re, sys, glob, hashlib, argparse, subprocess, tempfile, shutil

def run(c): return subprocess.run(c, capture_output=True, text=True)
def md5(b): return hashlib.md5(b).hexdigest()
def have(t): return shutil.which(t) is not None

def info(f): return run(["pdfinfo", f]).stdout
def npages(f):
    m = re.search(r"Pages:\s*(\d+)", info(f)); return int(m.group(1)) if m else 0
def encrypted(f): return bool(re.search(r"Encrypted:\s*yes", info(f)))

def decrypted(f, tmp):
    if encrypted(f) and have("qpdf"):
        out = os.path.join(tmp, "dec_" + str(abs(hash(f)) % 99999) + ".pdf")
        r = run(["qpdf", "--decrypt", f, out])
        if os.path.exists(out): return out
    return f

def norm_text(t):
    t = re.sub(r"\s+", "", t.lower())
    return md5(t.encode()) if len(t) >= 24 else None   # ignore near-empty pages for text match

def page_fps(pdf, tmp, dpi):
    """Return list of (img_md5, txt_md5_or_None) per page."""
    src = decrypted(pdf, tmp)
    sub = tempfile.mkdtemp(dir=tmp)
    run(["pdftoppm", "-gray", "-r", str(dpi), src, os.path.join(sub, "pg")])
    imgs = sorted(glob.glob(os.path.join(sub, "pg-*.p?m")),
                  key=lambda p: int(re.search(r"-(\d+)\.", p).group(1)))
    img_md5 = [md5(open(p, "rb").read()) for p in imgs]
    text_pages = run(["pdftotext", "-layout", src, "-"]).stdout.split("\f")
    txt_md5 = [norm_text(tp) for tp in text_pages]
    # align lengths defensively
    n = len(img_md5)
    while len(txt_md5) < n: txt_md5.append(None)
    return list(zip(img_md5, txt_md5[:n]))

def collect_pdfs(paths, exclude_dir=None):
    out = []
    for p in paths:
        p = os.path.abspath(os.path.expanduser(p))
        if os.path.isfile(p) and p.lower().endswith(".pdf"):
            out.append(p)
        elif os.path.isdir(p):
            for root, _, files in os.walk(p):
                if exclude_dir and os.path.abspath(root).startswith(os.path.abspath(exclude_dir)):
                    continue
                for f in files:
                    if f.lower().endswith(".pdf") and not f.startswith("."):
                        out.append(os.path.join(root, f))
    return sorted(set(out))

def to_trash(path):
    ap = os.path.abspath(path)
    # 1) `trash` CLI if present
    if have("trash"):
        return run(["trash", ap]).returncode == 0
    # 2) macOS Finder via osascript (recoverable, handles name collisions)
    scr = f'tell application "Finder" to delete (POSIX file "{ap}" as alias)'
    if run(["osascript", "-e", scr]).returncode == 0:
        return True
    # 3) fallback: move into ~/.Trash with a suffix
    trash = os.path.expanduser("~/.Trash")
    if os.path.isdir(trash):
        dst = os.path.join(trash, os.path.basename(ap.rstrip("/")))
        i = 1
        while os.path.exists(dst):
            dst = os.path.join(trash, f"{os.path.basename(ap.rstrip('/'))} ({i})"); i += 1
        shutil.move(ap, dst); return True
    return False

def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("folder", nargs="?")
    ap.add_argument("--originals")
    ap.add_argument("--against", nargs="+", default=[])
    ap.add_argument("--bundle-name", default="_original-bundles")
    ap.add_argument("--trash", action="store_true")
    ap.add_argument("--dpi", type=int, default=100)
    a = ap.parse_args()
    for t in ("pdftoppm", "pdftotext", "pdfinfo"):
        if not have(t): sys.exit(f"ERROR: '{t}' not found. Install: brew install poppler")

    # resolve originals + keepers
    if a.originals:
        orig_root = os.path.abspath(os.path.expanduser(a.originals))
        originals = collect_pdfs([orig_root])
        if not a.against: sys.exit("ERROR: --originals requires --against <keepers>")
        keepers = collect_pdfs(a.against)
        del_target = orig_root
    elif a.folder:
        folder = os.path.abspath(os.path.expanduser(a.folder))
        orig_root = os.path.join(folder, a.bundle_name)
        if not os.path.isdir(orig_root):
            sys.exit(f"ERROR: no '{a.bundle_name}/' in {folder} (pass --originals to name it)")
        originals = collect_pdfs([orig_root])
        keepers = collect_pdfs([folder], exclude_dir=orig_root)
        del_target = orig_root
    else:
        sys.exit("usage: safedelete.py <folder> | --originals X --against Y ...")

    keepers = [k for k in keepers if k not in originals]
    if not originals: sys.exit(f"ERROR: no PDFs to verify in {del_target}")
    if not keepers:   sys.exit("ERROR: no keeper PDFs to verify against")

    with tempfile.TemporaryDirectory() as tmp:
        # build keeper fingerprint index
        kimg, ktxt = {}, {}
        for k in keepers:
            for i, (im, tx) in enumerate(page_fps(k, tmp, a.dpi), 1):
                kimg.setdefault(im, []).append((k, i))
                if tx: ktxt.setdefault(tx, []).append((k, i))

        print(f"Verifying {len(originals)} original PDF(s) against {len(keepers)} keeper(s) "
              f"@ {a.dpi}dpi\n")
        total = matched_strong = matched_weak = unmatched = 0
        gaps = []
        for o in originals:
            fps = page_fps(o, tmp, a.dpi)
            print(f"  {os.path.relpath(o, os.path.dirname(del_target))}  ({len(fps)}pp)")
            for i, (im, tx) in enumerate(fps, 1):
                total += 1
                if im in kimg:
                    matched_strong += 1
                    where = kimg[im][0]
                    print(f"     p{i}: ✓ image-match  -> {os.path.basename(where[0])} p{where[1]}")
                elif tx and tx in ktxt:
                    matched_weak += 1
                    where = ktxt[tx][0]
                    print(f"     p{i}: ~ text-match (image differs) -> {os.path.basename(where[0])} p{where[1]}")
                else:
                    unmatched += 1
                    gaps.append((o, i))
                    print(f"     p{i}: ✗ UNMATCHED — not found in any keeper")

        safe = unmatched == 0
        print(f"\n{'='*60}")
        print(f"pages: {total}  |  image-match {matched_strong}  |  text-match {matched_weak}  |  UNMATCHED {unmatched}")
        print(("SAFE TO DELETE — every original page is present in a keeper."
               if safe else
               "NOT SAFE — some original pages are not represented (see UNMATCHED)."))
        if matched_weak:
            print("note: text-only matches mean the image differs (re-scan/rotation) but the "
                  "text is identical — review if that matters.")

        if a.trash:
            if not safe:
                print("\nRefusing to delete: verdict is NOT SAFE.")
                sys.exit(2)
            if to_trash(del_target):
                print(f"\nMoved to Trash (recoverable): {del_target}")
            else:
                print(f"\nERROR: could not move {del_target} to Trash — left in place."); sys.exit(1)

        sys.exit(0 if safe else 2)

if __name__ == "__main__":
    main()
