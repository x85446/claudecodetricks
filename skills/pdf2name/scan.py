#!/usr/bin/env python3
"""
scan.py — dump candidate metadata from PDF(s) so Claude can compose the
standard filename:  YYMMDD accountID source $amount description.pdf

For each PDF it prints candidate DATES (labeled statement/bill/invoice/due),
candidate AMOUNTS, and detected INSTITUTION / ADDRESS keywords. It does NOT
rename anything and does NOT decide the final fields — Claude reads this
output plus the raw text and makes the call.

Usage:
    scan.py <file.pdf | folder> [--recursive]

Requires: pdftotext, pdfinfo (poppler). Encrypted files are read via a
temporary qpdf --decrypt if qpdf is available.
"""
import os, re, sys, subprocess, shutil, tempfile

def run(c): return subprocess.run(c, capture_output=True, text=True)

def readable(f, tmp):
    """Return a path pdftotext can read (decrypt to temp if needed)."""
    enc = "Encrypted:      yes" in run(["pdfinfo", f]).stdout or \
          bool(re.search(r"Encrypted:\s*yes", run(["pdfinfo", f]).stdout))
    if enc and shutil.which("qpdf"):
        dst = os.path.join(tmp, "d.pdf")
        r = run(["qpdf", "--decrypt", f, dst])
        if os.path.exists(dst):
            return dst
    return f

def text(f, tmp):
    return run(["pdftotext", "-layout", readable(f, tmp), "-"]).stdout

DATE_LABELS = [
    ("statement", r"statement\s*date"),
    ("bill",      r"bill(?:ing)?\s*date"),
    ("invoice",   r"invoice\s*date"),
    ("service",   r"service\s*(?:date|period|from)"),
    ("due",       r"(?:payment\s*)?due\s*(?:date|by)?"),
]
INSTITUTIONS = [
    "citimortgage", "wells fargo", "chase", "bank of america", "usaa",
    "union bank", "midland", "farmers", "allstate", "state farm", "progressive",
    "city of", "water", "electric", "gas", "atmos", "pedernales", "pec",
    "spectrum", "at&t", "comcast", "google fiber", "waste", "hoa",
]

def find_dates(t):
    out = []
    for label, rx in DATE_LABELS:
        for m in re.finditer(rx + r"[:\s]*?(\d{1,2}[/-]\d{1,2}[/-]\d{2,4})", t, re.I):
            out.append((label, norm(m.group(1))))
    # bare dates as fallback
    bare = sorted(set(norm(d) for d in re.findall(r"\b\d{1,2}[/-]\d{1,2}[/-]\d{2,4}\b", t)))
    return out, bare

def norm(d):
    p = re.split(r"[/-]", d)
    if len(p) != 3: return d
    mm, dd, yy = p
    yy = int(yy)
    yy = 2000 + yy if yy < 100 else yy
    try:
        return f"{yy:04d}{int(mm):02d}{int(dd):02d}"   # YYYYMMDD (Claude shortens to YYMMDD)
    except ValueError:
        return d

def find_amounts(t):
    labeled = []
    for rx in [r"(?:total\s*(?:payment|amount)?\s*due|amount\s*due|payment\s*amount|total\s*current\s*charges|balance\s*due|new\s*balance)[^\n$]*\$?\s?([\d,]+\.\d\d)"]:
        for m in re.finditer(rx, t, re.I):
            labeled.append(m.group(1))
    allamt = re.findall(r"\$\s?([\d,]+\.\d\d)", t)
    from collections import Counter
    common = [a for a, _ in Counter(allamt).most_common(6)]
    return labeled[:4], common

def find_keywords(t):
    low = t.lower()
    inst = [k for k in INSTITUTIONS if k in low]
    # street-address-ish: number + word + (st|dr|rd|ave|cir|ln|blvd|way|ct)
    addr = re.findall(r"\b(\d{2,5}\s+[A-Za-z][A-Za-z .]{2,25}?\s+(?:st|street|dr|drive|rd|road|ave|avenue|cir|circle|ln|lane|blvd|way|ct|court|pkwy|trail|trl)\b)", t, re.I)
    return inst, sorted(set(a.strip() for a in addr))[:4]

def scan_one(f, tmp):
    t = text(f, tmp)
    dl, bare = find_dates(t)
    la, ca = find_amounts(t)
    inst, addr = find_keywords(t)
    print(f"\n=== {os.path.basename(f)} ===")
    if not t.strip():
        print("  (no extractable text — likely a scanned image; Claude must open it visually)")
        return
    print(f"  labeled dates : {dl if dl else '—'}")
    print(f"  other dates   : {bare[:8] if bare else '—'}")
    print(f"  labeled amts  : {la if la else '—'}")
    print(f"  common amts   : {ca if ca else '—'}")
    print(f"  institutions  : {inst if inst else '—'}")
    print(f"  addresses     : {addr if addr else '—'}")

def main():
    args = [a for a in sys.argv[1:] if a != "--recursive"]
    rec = "--recursive" in sys.argv
    if not args:
        sys.exit("usage: scan.py <file.pdf | folder> [--recursive]")
    target = os.path.abspath(os.path.expanduser(args[0]))
    files = []
    if os.path.isfile(target):
        files = [target]
    elif os.path.isdir(target):
        if rec:
            for root, _, fs in os.walk(target):
                files += [os.path.join(root, x) for x in fs if x.lower().endswith(".pdf") and not x.startswith(".")]
        else:
            files = [os.path.join(target, x) for x in os.listdir(target)
                     if x.lower().endswith(".pdf") and not x.startswith(".")]
        files.sort()
    else:
        sys.exit(f"not found: {target}")
    with tempfile.TemporaryDirectory() as tmp:
        for f in files:
            scan_one(f, tmp)

if __name__ == "__main__":
    main()
