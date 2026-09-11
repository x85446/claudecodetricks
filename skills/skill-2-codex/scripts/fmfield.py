#!/usr/bin/env python3
"""Read a frontmatter field with a real YAML parser, or emit one safely quoted.

The shell can only see the first line of a value, so a block scalar
(`description: >-` with the text indented under it) came through as the literal
">-" and the port shipped with no description at all. And a plain scalar opening
with a YAML indicator (`**Always invoke...` — `*` starts an alias) does not
parse at all on the far side. Both are handled here instead: parse with yaml,
re-emit double-quoted, and the whole class stops existing.

Usage:
  fmfield.py get <file> <field>      # print the parsed value (empty if absent)
  fmfield.py quote <field>           # read value on stdin, print `field: "..."`
"""
import re
import sys

import yaml


def frontmatter(path):
    txt = open(path, encoding="utf-8").read()
    parts = txt.split("---", 2)
    if len(parts) < 3:
        return {}
    try:
        fm = yaml.safe_load(parts[1])
        if isinstance(fm, dict):
            return fm
    except yaml.YAMLError:
        pass
    # The SOURCE may itself be lenient-parser-only (Claude Code tolerates an
    # unquoted `**Always ...`, which is a YAML alias). Fall back to a line read
    # so the port gets the real text rather than nothing — emitting an empty
    # description would turn one tolerable source into a port Codex rejects.
    out, key = {}, None
    for line in parts[1].splitlines():
        if re.match(r'^[A-Za-z_-]+:', line):
            key, _, val = line.partition(":")
            out[key.strip()] = val.strip()
        elif key and line.startswith(("  ", "\t")) and out.get(key) in ("", ">", ">-", "|", "|-"):
            out[key] = (out[key] if out[key] not in (">", ">-", "|", "|-") else "") + " " + line.strip()
    return {k: v.strip() for k, v in out.items()}


def main():
    if len(sys.argv) < 3:
        sys.exit(__doc__)
    mode = sys.argv[1]
    if mode == "get":
        val = frontmatter(sys.argv[2]).get(sys.argv[3])
        # collapse a folded/multi-line value to the single line Codex wants
        print(" ".join(str(val).split()) if val is not None else "")
    elif mode == "quote":
        val = " ".join(sys.stdin.read().split())
        # yaml.dump gives us correct escaping for every scalar it is handed;
        # default_style forces the quoted form even when it would not be needed,
        # so no port can regress into an unquoted opener later.
        scalar = yaml.dump(val, default_style='"', width=10**9,
                           allow_unicode=True, explicit_end=False).rstrip("\n")
        if scalar.endswith("\n..."):
            scalar = scalar[:-4]
        print(f"{sys.argv[2]}: {scalar.strip()}")
    else:
        sys.exit(__doc__)


if __name__ == "__main__":
    main()
