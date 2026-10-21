#!/usr/bin/env python3
"""Translates the Go server's templ templates (web/template/*.templ) into
Askama templates for the Rust port (rust/ims/templates/).

The templates are mostly static markup, so this handles only the templ
features they use: components and their calls, `if`/`else`/`switch` on lines
of their own, `{ expr }` interpolation of simple Go expressions, spread
attributes from NoAutofill(), and `{{ }}` Go blocks (for which a hand-written
Askama equivalent is supplied below).

Run it after changing a .templ file:

    python3 rust/tools/templ2askama.py

then `cargo test` checks the result against Go's own rendering.
"""

import pathlib
import re
import sys

REPO = pathlib.Path(__file__).resolve().parents[2]
SRC = REPO / "web" / "template"
OUT = REPO / "rust" / "ims" / "templates"

VOID_TAGS = {
    "area", "base", "br", "col", "embed", "hr", "img", "input", "link",
    "meta", "source", "track", "wbr",
}

# The spread attributes of common.templ's NoAutofill(), which templ renders
# in key-sorted order.
NO_AUTOFILL = 'autocomplete="off" data-1p-ignore="true" data-bwignore="true" data-lpignore="true"'

# Askama stand-ins for the Go blocks ({{ ... }}) at the top of these components.
GO_BLOCKS = {
    "Head": '{%- let v = cache_version|short_version -%}',
    "Nav": '{%- let event_set = event_name != "" -%}',
}

# Go expressions that need more than mechanical translation.
EXPR_OVERRIDES = {
    "logoHref": '{% if event_set %}/ims/app/events/{{ event_name }}/incidents{% else %}/ims/app{% endif %}',
}
# In Nav, a missing event name shows as "Event".
NAV_EVENT_NAME = '{% if event_set %}{{ event_name }}{% else %}Event{% endif %}'

LICENSE = """{#
 # Generated from web/template/{src} by rust/tools/templ2askama.py.
 # Edit the .templ file and rerun that script, rather than editing this.
 #}
"""


def snake(name):
    return re.sub(r"(?<=[a-z0-9])([A-Z])", r"_\1", name).lower()


def translate_expr(expr):
    """A simple Go expression, as an Askama expression."""
    expr = expr.strip()
    expr = re.sub(r"strings\.ToLower\(([A-Za-z_]\w*)\)", lambda m: f"{snake(m.group(1))}.to_lowercase()", expr)
    out = []
    pos = 0
    for m in re.finditer(r'"(?:[^"\\]|\\.)*"|[A-Za-z_]\w*|\+|.', expr):
        tok = m.group(0)
        if tok.startswith('"'):
            out.append(tok)
        elif tok == "+":
            out.append("~")
        elif re.fullmatch(r"[A-Za-z_]\w*", tok):
            out.append(tok if tok in ("true", "false", "to_lowercase") or out[-1:] == ["."] else snake(tok))
        else:
            out.append(tok)
        pos = m.end()
    return "".join(out)


def interpolate(expr, component):
    expr = expr.strip()
    if expr == "":
        return ""
    if expr == "NoAutofill()...":
        return NO_AUTOFILL
    if expr in EXPR_OVERRIDES:
        return EXPR_OVERRIDES[expr]
    if component == "Nav" and expr == "eventName":
        return NAV_EVENT_NAME
    lit = re.fullmatch(r'"((?:[^"\\]|\\.)*)"', expr)
    if lit:
        return lit.group(1)
    return "{{ " + translate_expr(expr) + " }}"


def expand_self_closing(text):
    """templ renders <div/> as <div></div>; an HTML parser wouldn't."""
    def repl(m):
        tag, attrs = m.group(1), m.group(2) or ""
        if tag.lower() in VOID_TAGS:
            return m.group(0)
        return f"<{tag}{attrs.rstrip()}></{tag}>"
    return re.sub(r"<([A-Za-z][\w-]*)((?:\s[^<>]*?)?)\s*/>", repl, text)


def replace_braces(line, component):
    """Replaces { expr } interpolations in a line of markup."""
    out = []
    i = 0
    while i < len(line):
        c = line[i]
        if c == "{":
            depth, j = 1, i + 1
            in_str = False
            while j < len(line) and depth:
                ch = line[j]
                if in_str:
                    if ch == "\\":
                        j += 1
                    elif ch == '"':
                        in_str = False
                elif ch == '"':
                    in_str = True
                elif ch == "{":
                    depth += 1
                elif ch == "}":
                    depth -= 1
                j += 1
            expr = line[i + 1 : j - 1]
            value = interpolate(expr, component)
            # An attribute value (attr={ expr }) needs quoting.
            if out and "".join(out).endswith("=") and not value.startswith(("autocomplete=",)):
                value = f'"{value}"'
            out.append(value)
            i = j
        else:
            out.append(c)
            i += 1
    return "".join(out)


def convert_body(name, body):
    body = expand_self_closing(body)
    lines = body.split("\n")
    out = []
    stack = []
    switch_first = {}
    in_go_block = False
    for line in lines:
        stripped = line.strip()
        if stripped == "{{":
            in_go_block = True
            continue
        if in_go_block:
            if stripped == "}}":
                in_go_block = False
                out.append(GO_BLOCKS[name])
            continue
        # templ drops Go-style comments in markup.
        if stripped.startswith("//"):
            continue
        m = re.fullmatch(r"if (.+) \{", stripped)
        if m:
            stack.append("if")
            out.append(f"{{% if {translate_expr(m.group(1))} %}}")
            continue
        if stripped == "} else {":
            out.append("{% else %}")
            continue
        m = re.fullmatch(r"switch (.+) \{", stripped)
        if m:
            stack.append(("switch", translate_expr(m.group(1))))
            switch_first[len(stack)] = True
            continue
        m = re.fullmatch(r'case (".*"):', stripped)
        if m and stack and isinstance(stack[-1], tuple):
            subject = stack[-1][1]
            kw = "if" if switch_first[len(stack)] else "else if"
            switch_first[len(stack)] = False
            out.append(f"{{% {kw} {subject} == {m.group(1)} %}}")
            continue
        if stripped == "default:" and stack and isinstance(stack[-1], tuple):
            out.append("{% else %}")
            continue
        if stripped == "}" and stack:
            stack.pop()
            out.append("{% endif %}")
            continue
        if stripped.startswith("@templ.JSONScript(\"importmap\""):
            # Head's import map, which templ renders from a Go map.
            out.append('    <script id="importmap" type="importmap">{"imports":{"/ims/static/ims.js":"/ims/static/ims.js?v={{ v }}"}}\n</script>')
            continue
        m = re.fullmatch(r"@(\w+)\((.*)\)", stripped)
        if m:
            args = m.group(2).strip()
            call_args = ", ".join(translate_expr(a) for a in split_args(args)) if args else ""
            out.append(f"{{% call c::{snake(m.group(1))}({call_args}) %}}")
            continue
        out.append(replace_braces(line, name))
    if stack:
        sys.exit(f"{name}: unclosed {stack}")
    return "\n".join(out).strip("\n")


def split_args(args):
    parts, depth, cur, in_str = [], 0, "", False
    for ch in args:
        if in_str:
            cur += ch
            if ch == '"':
                in_str = False
            continue
        if ch == '"':
            in_str = True
        elif ch == "(":
            depth += 1
        elif ch == ")":
            depth -= 1
        elif ch == "," and depth == 0:
            parts.append(cur)
            cur = ""
            continue
        cur += ch
    parts.append(cur)
    return parts


def components(path):
    """(name, params, body) for each templ component in a file."""
    text = path.read_text()
    # A component ends at a column-0 brace that's followed by another
    # top-level item (or the end), since an if block can close at column 0 too.
    pattern = r"^templ (\w+)\(([^)]*)\) \{\n(.*?)^\}\n(?=\s*(?:templ |//|func |\Z))"
    for m in re.finditer(pattern, text, re.S | re.M):
        name, params, body = m.group(1), m.group(2), m.group(3)
        names = []
        for group in params.split(","):
            group = group.strip()
            if group:
                names.append(snake(group.split()[0]))
        yield name, names, body


def main():
    OUT.mkdir(exist_ok=True)
    macros = []
    pages = {}
    for path in sorted(SRC.glob("*.templ")):
        for name, params, body in components(path):
            converted = convert_body(name, body)
            # Pages are the components that the web mux renders directly.
            if params[:3] == ["deployment", "version_name", "version_ref"]:
                pages[path.stem] = (path.name, params, converted)
            else:
                macros.append((path.name, name, params, converted))
    parts = [LICENSE.replace("{src}", "*.templ") + "{% import \"components.html\" as c %}"]
    for src, name, params, converted in macros:
        parts.append(f"{{# From {src} #}}\n{{% macro {snake(name)}({', '.join(params)}) %}}\n{converted}\n{{% endmacro %}}")
    (OUT / "components.html").write_text("\n\n".join(parts) + "\n")
    for stem, (src, params, converted) in pages.items():
        header = LICENSE.replace("{src}", src) + '{% import "components.html" as c %}\n'
        (OUT / f"{stem}.html").write_text(header + converted + "\n")
    print(f"Wrote {len(pages)} pages and {len(macros)} components to {OUT.relative_to(REPO)}")


if __name__ == "__main__":
    main()
