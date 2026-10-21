#!/usr/bin/env python3
"""
refresh_profile.py — keep grant-scout's view of Izuma current, and version it.

Reads:
  * products/*/one-pager.md (+ products/*.md)        — current capability claims
  * opportunities/submitted/*/                        — what Izuma has actually pursued
  * .claude/skills/grant-scout/izuma-profile.md       — the profile the rubric reads

Does:
  * Mines the submitted deals for the agencies Izuma has pursued and the technical
    themes that recur, producing an "Evidence from past submissions" digest.
  * Rewrites the managed evidence section of izuma-profile.md (between markers) with
    that digest — so each week the rubric grades against what we've actually chased.
  * Snapshots the resulting izuma-profile.md to output/grants/_profiles/v<N>.md and
    records a profile_versions row (version, captured_at, snapshot_path, source_hash).

Flags:
  --dry-run   read + mine + print; write nothing.
  --db / --profile / --submitted / --products / --snapshots  path overrides.

SOC 2 guard: the digest is mined from source text with a keyword whitelist; it never
introduces a SOC 2 certification claim. A final assertion refuses to write a profile
that contains "SOC 2 certified".
"""
import os, sys, re, json, glob, hashlib, sqlite3, datetime

MARK_START = "<!-- BEGIN auto-evidence (refresh_profile.py) -->"
MARK_END = "<!-- END auto-evidence (refresh_profile.py) -->"

# Capability themes the rubric cares about — token -> display label.
THEMES = {
    r'\bDDIL\b': 'DDIL / contested comms',
    r'\bmesh\b': 'secure mesh networking',
    r'wireguard': 'WireGuard overlay',
    r'\bCRDT\b': 'CRDT data sync',
    r'gossip': 'gossip protocols',
    r'kubernetes|k8s|vcluster|virtualcluster': 'Kubernetes / multi-tenant control planes',
    r'\bedge\b': 'edge orchestration',
    r'federat': 'federation',
    r'tenant|multi-tenan': 'multi-tenancy / isolation',
    r'\bTPM\b|UEFI|secure boot|attestation': 'hardware root of trust (TPM/UEFI)',
    r'bare.?metal|SMARC|\bSOM\b|provisioning': 'bare-metal provisioning',
    r'lwm2m|\bFOTA\b|firmware|device management': 'IoT device management / FOTA',
    r'\bPKI\b|x\.?509|mtls|\bTLS\b|\bMLS\b|\bNoise\b': 'crypto / PKI',
    r'zero.?trust': 'zero trust',
    r'sd-?wan': 'SD-WAN',
    r'quantum': 'quantum-related',
    r'\bOPA\b|\bOIDC\b|\bRBAC\b': 'authz (OPA/OIDC/RBAC)',
    r'disconnect|partition.?toleran|offline': 'disconnected / partition-tolerant ops',
}
# Agency detection from dir names + text.
AGENCIES = {
    r'\bARM\b|ARM26|\barmy\b|xtech': 'Army',
    r'\bDON\b|DON26|\bnavy\b|\bonr\b': 'Navy',
    r'\bDAF\b|DAF26|air force|afwerx|spacewerx': 'Air Force / Space Force',
    r'darpa|\bDPA\b|DPA26': 'DARPA',
    r'\bNEC\b': 'NEC',
    r'\bDHS\b|cisa': 'DHS / CISA',
    r'\bNSF\b': 'NSF',
    r'\bDOE\b|arpa-e': 'DOE',
    r'sbir|sttr': 'SBIR/STTR',
}


def utcnow():
    return datetime.datetime.now(datetime.timezone.utc).strftime('%Y-%m-%dT%H:%M:%SZ')


def gather_text(paths, exts=('.md', '.txt')):
    blobs = []
    for p in paths:
        for fp in glob.glob(p, recursive=True):
            if os.path.isfile(fp) and fp.lower().endswith(exts):
                try:
                    blobs.append(open(fp, encoding='utf-8', errors='ignore').read())
                except Exception:
                    pass
    return '\n'.join(blobs)


def mine(submitted_dir, products_glob):
    deal_dirs = sorted(d for d in glob.glob(os.path.join(submitted_dir, '*')) if os.path.isdir(d))
    deal_names = [os.path.basename(d) for d in deal_dirs]
    deal_text = gather_text([os.path.join(d, '**', '*') for d in deal_dirs])
    name_blob = ' '.join(deal_names)
    haystack = (deal_text + ' ' + name_blob)
    hay_l = haystack.lower()

    agencies = []
    for pat, label in AGENCIES.items():
        if re.search(pat, haystack, re.I):
            agencies.append(label)
    themes = []
    for pat, label in THEMES.items():
        hits = len(re.findall(pat, hay_l, re.I))
        if hits:
            themes.append((label, hits))
    themes.sort(key=lambda x: -x[1])

    products = sorted(os.path.basename(os.path.dirname(p)) or os.path.basename(p)
                      for p in glob.glob(products_glob))
    return {
        'deals': deal_names,
        'agencies': sorted(set(agencies)),
        'themes': themes,
        'products': products,
        'source_text_len': len(haystack),
    }


def build_digest(m):
    lines = [MARK_START,
             "## Evidence from past submissions",
             "",
             "_Auto-maintained by `refresh_profile.py` — mined from `opportunities/submitted/`. "
             "Informs the rubric with what Izuma has actually pursued._",
             ""]
    lines.append(f"**Deals submitted ({len(m['deals'])}):** " + (', '.join(m['deals']) or '—'))
    lines.append("")
    lines.append("**Agencies pursued:** " + (', '.join(m['agencies']) or '—'))
    lines.append("")
    if m['themes']:
        top = m['themes'][:12]
        lines.append("**Recurring technical themes (by mention count in past deals):**")
        for label, n in top:
            lines.append(f"- {label} ({n})")
    else:
        lines.append("**Recurring technical themes:** —")
    lines.append("")
    lines.append(MARK_END)
    return '\n'.join(lines)


def splice(profile_text, digest):
    if MARK_START in profile_text and MARK_END in profile_text:
        return re.sub(re.escape(MARK_START) + r'.*?' + re.escape(MARK_END),
                      digest, profile_text, flags=re.S)
    sep = '' if profile_text.endswith('\n') else '\n'
    return profile_text + sep + '\n' + digest + '\n'


def main():
    args = sys.argv[1:]
    db = 'output/grants/grants.db'
    profile = '.claude/skills/grant-scout/izuma-profile.md'
    submitted = 'opportunities/submitted'
    products = 'products/*/one-pager.md'
    snapshots = 'output/grants/_profiles'
    dry = '--dry-run' in args
    i = 0
    while i < len(args):
        if args[i] == '--db': db = args[i+1]; i += 2
        elif args[i] == '--profile': profile = args[i+1]; i += 2
        elif args[i] == '--submitted': submitted = args[i+1]; i += 2
        elif args[i] == '--products': products = args[i+1]; i += 2
        elif args[i] == '--snapshots': snapshots = args[i+1]; i += 2
        else: i += 1

    m = mine(submitted, products)
    print(f"[refresh] submitted deals: {m['deals']}")
    print(f"[refresh] agencies: {m['agencies']}")
    print(f"[refresh] top themes: {[t[0] for t in m['themes'][:8]]}")
    print(f"[refresh] products: {m['products']}")

    digest = build_digest(m)
    if dry:
        print("[refresh] --dry-run: no writes. Digest preview:\n")
        print(digest)
        return

    prof_text = open(profile, encoding='utf-8').read() if os.path.exists(profile) else "# Izuma Networks — Company Profile\n"
    new_text = splice(prof_text, digest)
    if re.search(r'soc\s*2\s*(?:type\s*[12]\s*)?certif', new_text, re.I):
        print("[refresh] ABORT: profile would contain a SOC 2 certification claim.", file=sys.stderr)
        sys.exit(3)
    open(profile, 'w', encoding='utf-8').write(new_text)

    # Version + snapshot.
    os.makedirs(snapshots, exist_ok=True)
    con = sqlite3.connect(db)
    con.execute("""CREATE TABLE IF NOT EXISTS profile_versions (
        version INTEGER PRIMARY KEY, captured_at TEXT, snapshot_path TEXT,
        source_hash TEXT, notes TEXT)""")
    cur = con.execute("SELECT COALESCE(MAX(version),0) FROM profile_versions").fetchone()[0]
    version = cur + 1
    snap_path = os.path.join(snapshots, f"v{version}.md")
    open(snap_path, 'w', encoding='utf-8').write(new_text)
    src_hash = hashlib.sha256(new_text.encode('utf-8')).hexdigest()[:16]
    notes = f"deals={len(m['deals'])} agencies={','.join(m['agencies'])} themes={len(m['themes'])}"
    con.execute("INSERT INTO profile_versions(version,captured_at,snapshot_path,source_hash,notes) VALUES(?,?,?,?,?)",
                (version, utcnow(), snap_path, src_hash, notes))
    con.commit(); con.close()
    print(f"[refresh] wrote profile v{version} → {snap_path} (hash {src_hash})")
    print(f"[refresh] updated {profile} evidence section + recorded profile_versions row")


if __name__ == '__main__':
    main()
