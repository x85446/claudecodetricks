"""
normalizers — per-source raw-record -> unified-schema adapters.

Each module `normalizers/<short_name>.py` owns exactly one grant source and
declares:

    SOURCE      long dotted source identity stamped on every normalized
                record's "source" field, e.g. 'sam.gov', 'grants.gov.extract'.
                This is also the key merge.py dedupes and reports on.
    DIR         corpus subdirectory name: raw records live at
                <corpus>/<DIR>/raw/<id>.json and this module writes
                <corpus>/<DIR>/normalized.jsonl.
    PRIORITY    int; lower wins merge.py's cross-source dedupe when two
                sources report the same solicitation_number. Omit to sort
                last (least authoritative default).
    normalize(corpus_dir) -> Iterable[dict]
                reads <corpus_dir>/<DIR>/raw/*.json and yields unified-schema
                records (see normalize.py's module docstring for the schema).

Modules whose filename starts with "_" are shared helpers (e.g. _common.py),
not sources, and are skipped by discover().

Adding a new source is: drop a new module here with those four names.
normalize.py, merge.py and fetch_all.sh's per-source normalize step all pick
it up with no further editing.
"""
import importlib
import pkgutil

__all__ = ['discover', 'get', 'load_all']


def discover():
    """Return the set of short source-names that have a normalizer module."""
    return {
        modname for _, modname, _ in pkgutil.iter_modules(__path__)
        if not modname.startswith('_')
    }


def get(short_name):
    """Import and return the normalizer module for a short source-name."""
    return importlib.import_module(f'{__name__}.{short_name}')


def load_all():
    """Return {short_name: module} for every discovered source, ordered by
    each module's declared PRIORITY (lowest first; missing PRIORITY sorts
    last). This is the single ordering merge.py uses for its dedupe priority
    and normalize.py uses as its default source list."""
    mods = {name: get(name) for name in discover()}
    return dict(sorted(mods.items(), key=lambda kv: getattr(kv[1], 'PRIORITY', 10_000)))
