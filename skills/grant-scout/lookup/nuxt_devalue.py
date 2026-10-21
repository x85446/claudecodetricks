"""nuxt_devalue.py — pure-stdlib parser for Nuxt 2 / @nuxt/devalue __NUXT__ payloads.

Nuxt 2 SSR pages embed their full page state directly in the served HTML as:

    <script>window.__NUXT__=(function(a,b,c,...){
        <zero or more mutation statements, e.g. Rj.metadata={...};Rj.sys={...};>
        return <EXPR using bare a,b,c,... refs mixed with literals>
    }(<literal for a>,<literal for b>,...))</script>

This is NOT JSON: it has unquoted object keys, bare-identifier back-references
(devalue's dedup mechanism — a value referenced more than once is hoisted into
its own call-argument slot and pointed at by name wherever it recurs), plus
undefined/NaN/Infinity literals JSON can't express, and — for circular/shared
containers that can't be built as a single expression — a handful of
`ident.prop=value;` / `ident[idx]=value;` mutation statements before the
`return`. It IS, however, a small deterministic grammar, so this extracts a
page's full state with a single `curl` + this parser — no browser, no JS
runtime, no third-party package.

Usage:
    from nuxt_devalue import extract_nuxt_expr, parse_nuxt_devalue
    expr = extract_nuxt_expr(html)          # None if the page has no payload
    data = parse_nuxt_devalue(expr)         # -> plain Python dict/list/... tree
"""
import re

__all__ = ['extract_nuxt_expr', 'parse_nuxt_devalue', 'DevalueParseError']


class DevalueParseError(ValueError):
    pass


# ---------------------------------------------------------------------------
# Extraction: find `window.__NUXT__=...;` inside a <script> tag in raw HTML.
# ---------------------------------------------------------------------------

_NUXT_ASSIGN_RE = re.compile(r'window\.__NUXT__\s*=')


def extract_nuxt_expr(html):
    """Return the JS expression text assigned to window.__NUXT__ (trailing ';'
    stripped), or None if the page carries no such payload."""
    m = _NUXT_ASSIGN_RE.search(html)
    if not m:
        return None
    start = m.end()
    end = html.find('</script>', start)
    if end == -1:
        end = len(html)
    expr = html[start:end].rstrip()
    if expr.endswith(';'):
        expr = expr[:-1]
    return expr


# ---------------------------------------------------------------------------
# Tokenizer
# ---------------------------------------------------------------------------

_TOKEN_RE = re.compile(r'''
    (?P<ws>\s+)
  | (?P<string>"(?:\\.|[^"\\])*")
  | (?P<number>-?\d+(?:\.\d+)?(?:[eE][+-]?\d+)?)
  | (?P<ident>[A-Za-z_$][A-Za-z0-9_$]*)
  | (?P<punct>[{}\[\]:,();.=])
''', re.VERBOSE)

_ESCAPES = {'"': '"', '\\': '\\', '/': '/', 'b': '\b', 'f': '\f',
            'n': '\n', 'r': '\r', 't': '\t'}


def _tokenize(s):
    toks = []
    pos = 0
    n = len(s)
    while pos < n:
        m = _TOKEN_RE.match(s, pos)
        if not m:
            raise DevalueParseError(f"tokenize failure at {pos}: {s[pos:pos + 60]!r}")
        pos = m.end()
        if m.lastgroup == 'ws':
            continue
        toks.append((m.lastgroup, m.group()))
    return toks


def _decode_js_string(lit):
    """lit includes the surrounding double quotes."""
    body = lit[1:-1]
    out = []
    i, n = 0, len(body)
    while i < n:
        c = body[i]
        if c == '\\':
            nxt = body[i + 1]
            if nxt == 'u':
                out.append(chr(int(body[i + 2:i + 6], 16)))
                i += 6
                continue
            if nxt in _ESCAPES:
                out.append(_ESCAPES[nxt])
                i += 2
                continue
            out.append(nxt)
            i += 2
            continue
        out.append(c)
        i += 1
    return ''.join(out)


class _Ref:
    """AST node: a bare identifier — a back-reference to another argument slot."""
    __slots__ = ('name',)

    def __init__(self, name):
        self.name = name

    def __repr__(self):
        return f'_Ref({self.name})'


_UNDEFINED = object()  # sentinel for JS `undefined` / array elisions -> None on output


class _Parser:
    def __init__(self, toks):
        self.toks = toks
        self.i = 0
        self.n = len(toks)

    def peek(self):
        return self.toks[self.i] if self.i < self.n else (None, None)

    def next(self):
        t = self.toks[self.i]
        self.i += 1
        return t

    def expect(self, val):
        kind, text = self.next()
        if text != val:
            raise DevalueParseError(f"expected {val!r}, got {text!r} at token {self.i}")

    def parse_value(self):
        kind, text = self.peek()
        if kind is None:
            raise DevalueParseError("unexpected EOF while parsing a value")
        if kind == 'string':
            self.next()
            return _decode_js_string(text)
        if kind == 'number':
            self.next()
            return float(text) if ('.' in text or 'e' in text or 'E' in text) else int(text)
        if text == '{':
            return self._parse_object()
        if text == '[':
            return self._parse_array()
        if text == '-':
            self.next()
            kind2, text2 = self.next()
            if text2 == 'Infinity':
                return float('-inf')
            raise DevalueParseError(f"unexpected '-{text2}' at token {self.i}")
        if kind == 'ident':
            self.next()
            if text == 'true':
                return True
            if text == 'false':
                return False
            if text == 'null':
                return None
            if text == 'undefined':
                return _UNDEFINED
            if text == 'NaN':
                return float('nan')
            if text == 'Infinity':
                return float('inf')
            return _Ref(text)
        raise DevalueParseError(f"unexpected token {text!r} at {self.i}")

    def _parse_object(self):
        self.expect('{')
        obj = {}
        if self.peek()[1] == '}':
            self.next()
            return obj
        while True:
            kkind, ktext = self.next()
            if kkind == 'string':
                key = _decode_js_string(ktext)
            elif kkind in ('ident', 'number'):
                key = ktext
            else:
                raise DevalueParseError(f"bad object key {ktext!r} at {self.i}")
            self.expect(':')
            obj[key] = self.parse_value()
            _, text = self.peek()
            if text == ',':
                self.next()
                if self.peek()[1] == '}':  # trailing comma
                    self.next()
                    break
                continue
            self.expect('}')
            break
        return obj

    def _parse_array(self):
        self.expect('[')
        arr = []
        if self.peek()[1] == ']':
            self.next()
            return arr
        while True:
            _, text = self.peek()
            if text == ',':
                arr.append(_UNDEFINED)  # elision, e.g. [1,,3]
                self.next()
                continue
            if text == ']':
                self.next()
                break
            arr.append(self.parse_value())
            _, text = self.peek()
            if text == ',':
                self.next()
                if self.peek()[1] == ']':
                    self.next()
                    break
                continue
            self.expect(']')
            break
        return arr

    def parse_function_body(self):
        """Zero or more mutation statements, then exactly one `return <expr>;`.
        Returns (mutations, return_ast); mutations is a list of
        (base_ident, path, value_ast) with path entries ('prop', key) /
        ('index', key)."""
        mutations = []
        while True:
            kind, text = self.peek()
            if kind == 'ident' and text == 'return':
                self.next()
                root_ast = self.parse_value()
                if self.peek()[1] == ';':
                    self.next()
                return mutations, root_ast
            kind, base = self.next()
            if kind != 'ident':
                raise DevalueParseError(f"expected statement start ident, got {base!r} at {self.i}")
            path = []
            while True:
                _, text = self.peek()
                if text == '.':
                    self.next()
                    _, key = self.next()
                    path.append(('prop', key))
                elif text == '[':
                    self.next()
                    kkind, idxtext = self.next()
                    if kkind == 'number':
                        idx = int(idxtext)
                    elif kkind == 'string':
                        idx = _decode_js_string(idxtext)
                    else:
                        raise DevalueParseError(f"bad index token {idxtext!r} at {self.i}")
                    self.expect(']')
                    path.append(('index', idx))
                else:
                    break
            self.expect('=')
            val_ast = self.parse_value()
            if self.peek()[1] == ';':
                self.next()
            mutations.append((base, path, val_ast))

    def parse_comma_list_until_rparen(self):
        """Parses a comma-separated value list. Does NOT consume the closing
        ')' — it only peeks at it, leaving it for the caller to consume (the
        caller also needs to consume an enclosing ')' immediately after)."""
        vals = []
        if self.peek()[1] == ')':
            return vals
        while True:
            vals.append(self.parse_value())
            _, text = self.peek()
            if text == ',':
                self.next()
                continue
            if text == ')':
                break
            raise DevalueParseError(f"expected ',' or ')', got {text!r} at {self.i}")
        return vals


def parse_nuxt_devalue(js_src):
    """Parse a `(function(a,b,...){...}(args...))` devalue payload (the string
    returned by extract_nuxt_expr) into a plain Python value tree."""
    toks = _tokenize(js_src)
    p = _Parser(toks)

    p.expect('(')
    kind, text = p.next()
    if text != 'function':
        raise DevalueParseError(f"expected 'function', got {text!r}")
    p.expect('(')
    params = []
    if p.peek()[1] != ')':
        while True:
            _, name = p.next()
            params.append(name)
            if p.peek()[1] == ',':
                p.next()
                continue
            break
    p.expect(')')
    p.expect('{')
    mutations, root_ast = p.parse_function_body()
    p.expect('}')
    p.expect('(')
    arg_asts = p.parse_comma_list_until_rparen()
    p.expect(')')
    p.expect(')')

    if len(arg_asts) != len(params):
        raise DevalueParseError(
            f"param/arg count mismatch: {len(params)} params vs {len(arg_asts)} args")
    slot_ast = dict(zip(params, arg_asts))

    resolved = {}
    resolving = set()

    def resolve(node):
        if isinstance(node, _Ref):
            name = node.name
            if name in resolved:
                return resolved[name]
            if name not in slot_ast:
                return f'__UNRESOLVED_REF_{name}__'
            if name in resolving:
                return None  # circular guard (shouldn't trigger for real content)
            resolving.add(name)
            val = resolve(slot_ast[name])
            resolving.discard(name)
            resolved[name] = val
            return val
        if isinstance(node, dict):
            return {k: (None if v is _UNDEFINED else resolve(v)) for k, v in node.items()}
        if isinstance(node, list):
            return [None if v is _UNDEFINED else resolve(v) for v in node]
        if node is _UNDEFINED:
            return None
        return node

    # Phase 1: eagerly resolve every param slot once, so any container object
    # (dict/list) is built exactly once and shared by reference everywhere else
    # in the graph that points at it (mirrors JS reference semantics).
    for name in params:
        resolve(_Ref(name))

    # Phase 2: apply mutation statements on top of the now-built containers —
    # this is how devalue expresses circular/shared structures that can't be
    # written as a single literal expression.
    for base, path, val_ast in mutations:
        if base not in resolved:
            continue
        container = resolved[base]
        if not path:
            resolved[base] = resolve(val_ast)
            continue
        for idx, (ptype, key) in enumerate(path):
            is_last = idx == len(path) - 1
            if is_last:
                value = resolve(val_ast)
                if ptype == 'index' and isinstance(container, list):
                    while len(container) <= key:
                        container.append(None)
                container[key] = value
            else:
                container = container[key]

    return resolve(root_ast)
