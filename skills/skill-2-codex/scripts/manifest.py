"""Codex's skills-catalog cost model, shared by validate.py and diet.py.

Mirrors codex-rs/ext/skills/src/render.rs:
  - budget is `[skills] max_context_tokens` from ~/.codex/config.toml (Codex
    caps it at 10,000); unset, it is 2% of the model's context window in tokens,
    or 8,000 characters when the window is unknown. Only the configured value is
    deterministic, so it is the one this model reads.
  - each implicitly-invocable skill costs one line,
    `- {name}: {description} (file: {path})\n`, at ceil(bytes / 4) tokens.
  - a description over 1,024 chars is cut to that before it is charged.
  - the budget is shared with every other skill Codex lists: its bundled
    `.system` skills and the skills of each enabled plugin.
"""
import glob, os, re, tomllib

CODEX_HOME = os.path.expanduser("~/.codex")
INSTALL_ROOT = os.path.expanduser("~/.agents/skills")
MAX_CONFIGURED_TOKENS = 10_000
MAX_DESC_CHARS = 1_024
BYTES_PER_TOKEN = 4


def configured_budget():
    """Tokens from [skills] max_context_tokens, or None when unset."""
    try:
        with open(os.path.join(CODEX_HOME, "config.toml"), "rb") as f:
            cfg = tomllib.load(f)
    except (OSError, tomllib.TOMLDecodeError):
        return None
    v = (cfg.get("skills") or {}).get("max_context_tokens")
    return min(int(v), MAX_CONFIGURED_TOKENS) if v else None


def line_cost(name, desc, path):
    if len(desc) > MAX_DESC_CHARS:
        desc = desc[:MAX_DESC_CHARS - 3] + "..."
    line = f"- {name}: {desc} (file: {path})\n" if desc else f"- {name}: (file: {path})\n"
    return (len(line.encode("utf-8")) + BYTES_PER_TOKEN - 1) // BYTES_PER_TOKEN


def is_implicit(skill_dir):
    y = os.path.join(skill_dir, "agents", "openai.yaml")
    return not (os.path.exists(y) and
                re.search(r"allow_implicit_invocation:\s*false", open(y, encoding="utf-8").read()))


def _desc(path):
    import yaml
    parts = open(path, encoding="utf-8").read().split("---", 2)
    try:
        fm = yaml.safe_load(parts[1]) if len(parts) == 3 else None
    except yaml.YAMLError:
        return None, None
    if not isinstance(fm, dict):
        return None, None
    return str(fm.get("name") or ""), str(fm.get("description") or "").strip()


def neighbours():
    """(name, cost) for every non-ported skill Codex also lists."""
    paths = glob.glob(os.path.join(CODEX_HOME, "skills", ".system", "*", "SKILL.md"))
    try:
        with open(os.path.join(CODEX_HOME, "config.toml"), "rb") as f:
            plugins = tomllib.load(f).get("plugins") or {}
    except (OSError, tomllib.TOMLDecodeError):
        plugins = {}
    for key, conf in plugins.items():
        if not (isinstance(conf, dict) and conf.get("enabled")) or "@" not in key:
            continue
        plugin, market = key.split("@", 1)
        versions = sorted(glob.glob(os.path.join(CODEX_HOME, "plugins", "cache", market, plugin, "*")),
                          key=os.path.getmtime)
        if versions:
            paths += glob.glob(os.path.join(versions[-1], "skills", "*", "SKILL.md"))
    out = []
    for p in sorted(paths):
        if not is_implicit(os.path.dirname(p)):
            continue
        name, desc = _desc(p)
        if desc is None:
            continue
        out.append((name or os.path.basename(os.path.dirname(p)), line_cost(name, desc, p)))
    return out


def port_cost(name, desc):
    """Cost of a ported skill at the path it is installed to, not generated at."""
    return line_cost(name, desc, os.path.join(INSTALL_ROOT, name, "SKILL.md"))
