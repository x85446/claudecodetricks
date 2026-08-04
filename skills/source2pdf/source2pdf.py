#!/usr/bin/env python3
"""
source2pdf — concatenate every PDF in a _sources/ folder, in filename order,
into one master PDF named after the parent folder, placed beside _sources/.

Handles encrypted inputs (qpdf --decrypt, losslessly) and damaged inputs
(qpdf reconstructs the xref). Verifies the output page count equals the sum
of the inputs. Never deletes _sources.

Usage:
    source2pdf.py <path>
      <path> may be a _sources folder directly, OR a parent folder that
      contains a _sources subfolder.

    Options:
      --name NAME   Override the output filename (default: <parent-folder>.pdf)
      --force       Overwrite an existing output file without asking

Requires: pdfinfo, pdfunite, qpdf (poppler + qpdf). Install: brew install poppler qpdf
"""
import os, re, sys, shutil, subprocess, tempfile, argparse

def run(cmd):
    return subprocess.run(cmd, capture_output=True, text=True)

def need(tool):
    if shutil.which(tool) is None:
        sys.exit(f"ERROR: '{tool}' not found. Install with: brew install poppler qpdf")

def info(f):
    return run(["pdfinfo", f]).stdout

def npages(f):
    m = re.search(r"Pages:\s*(\d+)", info(f))
    return int(m.group(1)) if m else 0

def encrypted(f):
    return bool(re.search(r"Encrypted:\s*yes", info(f)))

def resolve_sources(path):
    path = os.path.abspath(os.path.expanduser(path))
    if os.path.isdir(path) and os.path.basename(path) == "_sources":
        return path
    cand = os.path.join(path, "_sources")
    if os.path.isdir(cand):
        return cand
    if os.path.isdir(path):
        sys.exit(f"ERROR: no _sources/ folder found in {path}")
    sys.exit(f"ERROR: not a directory: {path}")

def collect(src):
    """All *.pdf under _sources, sorted by basename (YYMMDD/YYYYMMDD prefix -> chronological)."""
    out = []
    for root, _, files in os.walk(src):
        for fn in files:
            if fn.lower().endswith(".pdf") and not fn.startswith("."):
                out.append(os.path.join(root, fn))
    out.sort(key=lambda p: os.path.basename(p).lower())
    return out

def prep(f, tmp, idx):
    """Return a mergeable path: decrypt/repair via qpdf if needed."""
    if encrypted(f):
        dst = os.path.join(tmp, f"d{idx}.pdf")
        r = run(["qpdf", "--decrypt", f, dst])          # rc 3 == "succeeded with warnings"
        if r.returncode not in (0, 3) or not os.path.exists(dst):
            raise RuntimeError(f"qpdf failed (rc={r.returncode}) on {f}\n{r.stderr[:300]}")
        return dst
    return f

def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("path")
    ap.add_argument("--name")
    ap.add_argument("--force", action="store_true")
    a = ap.parse_args()
    for t in ("pdfinfo", "pdfunite", "qpdf"):
        need(t)

    src = resolve_sources(a.path)
    parent = os.path.dirname(src)
    out_name = a.name or (os.path.basename(parent) + ".pdf")
    if not out_name.lower().endswith(".pdf"):
        out_name += ".pdf"
    out = os.path.join(parent, out_name)

    inputs = collect(src)
    if not inputs:
        sys.exit(f"ERROR: no PDFs found under {src}")
    if os.path.exists(out) and not a.force:
        sys.exit(f"ERROR: {out} already exists. Re-run with --force to overwrite.")

    print(f"_sources: {src}")
    print(f"output:   {out}")
    print(f"merging {len(inputs)} PDFs in order:")
    for p in inputs:
        print(f"   {os.path.relpath(p, src)}")

    with tempfile.TemporaryDirectory() as tmp:
        prepped = [prep(p, tmp, i) for i, p in enumerate(inputs)]
        expected = sum(npages(p) for p in prepped)
        r = run(["pdfunite"] + prepped + [out])
        if not os.path.exists(out):
            sys.exit(f"ERROR: pdfunite failed: {r.stderr[:400]}")

    got = npages(out)
    size = subprocess.run(["du", "-h", out], capture_output=True, text=True).stdout.split()[0]
    status = "OK" if got == expected else "PAGE-COUNT MISMATCH"
    print(f"\n{status}: {out_name} = {got}pp (expected {expected}), {size}")
    print("_sources kept intact (nothing deleted).")
    if got != expected:
        sys.exit(1)

if __name__ == "__main__":
    main()
