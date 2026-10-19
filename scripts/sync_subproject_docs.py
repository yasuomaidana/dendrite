"""Keep the Dendrite docs that sub-projects import in sync, and their docs indexes correct.

Each sub-project lists the Dendrite docs that specify its scope in imports.json. This script copies
them to <docdir>/imported/ with a provenance header and regenerates the imported-docs section of the
repo's docs index, keeping the descriptions already written there.

Descriptions belong to the sub-project: each one says what the spec defines for that repository
(its crates, algorithms, tools), never what it means for Dendrite. The script never writes one; new
entries appear without a description until someone writes it in the index.

    uv run scripts/sync_subproject_docs.py            # copy + re-index every sub-project
    uv run scripts/sync_subproject_docs.py --check    # report only; exit 1 on any problem
    uv run scripts/sync_subproject_docs.py --repo dendrite-rag-rs
"""

import argparse
import datetime
import json
import os
import re
import subprocess
import sys

DENDRITE = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
GOLAND = os.path.dirname(DENDRITE)

# name -> (repo path, docs dir, index file inside the docs dir, heading of the imported-docs section)
REPOS = {
    "dendrite-mcp-hub": (os.path.join(GOLAND, "dendrite-mcp-hub"), "docs", "README.md", "## Imported from Dendrite"),
    "dendrite-core-rs": (os.path.join(GOLAND, "dendrite-core-rs"), "docs", "README.md", "## Imported from Dendrite"),
    "dendrite-git-rs": (os.path.join(GOLAND, "dendrite-git-rs"), "docs", "README.md", "## Imported from Dendrite"),
    "dendrite-rag-rs": (os.path.join(GOLAND, "dendrite-rag-rs"), "docs", "README.md", "## Imported from Dendrite"),
    "embedag": (os.path.expanduser("~/RustRoverProjects/embedag"), "doc", "dendrite-integration.md", "## 7. Imported Docs"),
}

HEADER_PREFIX = "<!-- Imported from Dendrite "

CATEGORIES = [
    ("architecture", "Architecture"),
    ("adr", "Architecture decisions"),
    ("features", "Feature specifications"),
    ("cli", "CLI reference"),
    ("mcp", "MCP tool reference"),
    ("guides", "Guides"),
    ("quality", "Quality"),
    ("reports", "Reports"),
]


def dendrite_commit():
    return subprocess.check_output(["git", "-C", DENDRITE, "rev-parse", "--short", "HEAD"], text=True).strip()


def strip_header(text):
    """Return the body of an imported copy without its provenance header."""
    if text.startswith(HEADER_PREFIX):
        end = text.find("-->")
        if end >= 0:
            return text[end + 3:].lstrip("\n")
    return text


def rel_of(path):
    return path[len("docs/"):] if path.startswith("docs/") else path


def existing_descriptions(index_text, paths):
    """Map each import path to the description already written for it in the index."""
    out = {}
    for line in index_text.splitlines():
        for p in paths:
            rel = rel_of(p)
            if f"imported/{rel}" not in line and f"`{p}`" not in line:
                continue
            for sep in (" — ", "`): ", "): "):
                if sep in line:
                    desc = line.split(sep, 1)[1].strip()
                    if desc and p not in out:
                        out[p] = desc
                    break
    return out


def section_bounds(text, heading):
    """Return (start, end) of the section that starts at heading, or None."""
    start = text.find("\n" + heading + "\n")
    if start < 0:
        return None
    start += 1
    nxt = re.search(r"^#{1,2} ", text[start + len(heading):], re.MULTILINE)
    end = start + len(heading) + nxt.start() if nxt else len(text)
    return start, end


def render_section(name, heading, paths, descs, docdir):
    lines = [
        heading,
        "",
        "<!-- Generated from imports.json by scripts/sync_subproject_docs.py (control-plane repo). The list is regenerated; "
        "descriptions are kept and must describe this repository. -->",
        "",
        f"Specifications that define `{name}`'s responsibilities, listed in [`imports.json`](imports.json) and copied into "
        f"`{docdir}/imported/`. The first line of each copy records where it was copied from. The copies are regenerated, "
        "so do not edit them here. Each entry below says what the spec defines for this repository.",
        "",
    ]
    by_cat = {}
    for p in paths:
        by_cat.setdefault(rel_of(p).split("/", 1)[0], []).append(p)
    known = [c for c, _ in CATEGORIES]
    order = [(c, t) for c, t in CATEGORIES if c in by_cat] + [(c, c.capitalize()) for c in sorted(by_cat) if c not in known]
    for cat, title in order:
        lines += [f"### {title}", ""]
        for p in by_cat[cat]:
            rel = rel_of(p)
            label = f"`{os.path.splitext(os.path.basename(rel))[0]}`"
            desc = descs.get(p, "")
            lines.append(f"- [{label}](imported/{rel})" + (f" — {desc}" if desc else ""))
        lines.append("")
    return "\n".join(lines) + "\n"


def broken_links(index_path):
    base = os.path.dirname(index_path)
    text = open(index_path).read()
    bad = []
    for target in re.findall(r"\]\(([^)#\s]+)", text):
        if re.match(r"^[a-z]+://", target) or target.startswith("mailto:"):
            continue
        if not os.path.exists(os.path.join(base, target)):
            bad.append(target)
    return bad


def process(name, check, commit, today):
    repo, docdir, index_name, heading = REPOS[name]
    ddir = os.path.join(repo, docdir)
    imports_path = os.path.join(ddir, "imports.json")
    index_path = os.path.join(ddir, index_name)
    problems, changes = [], []

    paths = json.load(open(imports_path))
    if len(set(paths)) != len(paths):
        problems.append("imports.json lists a path twice")
        paths = list(dict.fromkeys(paths))

    for p in paths:
        src = os.path.join(DENDRITE, p)
        if not os.path.isfile(src):
            problems.append(f"source missing in Dendrite: {p}")
            continue
        body = open(src).read()
        dst = os.path.join(ddir, "imported", rel_of(p))
        current = open(dst).read() if os.path.isfile(dst) else None
        if current is not None and current.startswith(HEADER_PREFIX) and strip_header(current) == body:
            continue
        problems.append(("missing copy: " if current is None else "stale copy: ") + rel_of(p))
        if not check:
            os.makedirs(os.path.dirname(dst), exist_ok=True)
            with open(dst, "w") as f:
                f.write(f"{HEADER_PREFIX}{p} @ {commit} on {today}. Source of truth for this repo's scope; "
                        f"keep in sync with the Dendrite copy until the extraction umbrella lands. -->\n\n{body}")
            changes.append(rel_of(p))

    listed = {rel_of(p) for p in paths}
    imported_root = os.path.join(ddir, "imported")
    for root, _, files in os.walk(imported_root):
        for fn in files:
            rel = os.path.relpath(os.path.join(root, fn), imported_root)
            if rel not in listed:
                problems.append(f"orphan copy not in imports.json: {rel}")

    index_text = open(index_path).read()
    section = render_section(name, heading, paths, existing_descriptions(index_text, paths), docdir)
    bounds = section_bounds(index_text, heading)
    if bounds:
        old = index_text[bounds[0]:bounds[1]]
        trailer = "---\n\n" if old.rstrip().endswith("---") else ""
        new_text = index_text[:bounds[0]] + section + trailer + index_text[bounds[1]:]
    else:
        new_text = index_text.rstrip("\n") + "\n\n" + section
    if new_text != index_text:
        problems.append(f"index section out of date: {docdir}/{index_name}")
        if not check:
            with open(index_path, "w") as f:
                f.write(new_text)
            changes.append(f"{docdir}/{index_name}")

    for target in broken_links(index_path):
        problems.append(f"broken link in {docdir}/{index_name}: {target}")

    return problems, changes


SUBPROJECTS_MD = os.path.join(DENDRITE, "docs", "architecture", "subprojects.md")
TABLE_BEGIN = "<!-- sync_subproject_docs:begin -->"
TABLE_END = "<!-- sync_subproject_docs:end -->"


def render_table():
    """Table of every Dendrite doc that sub-projects import, and which ones import it."""
    users = {}
    for name in sorted(REPOS):
        repo, docdir, _, _ = REPOS[name]
        for p in json.load(open(os.path.join(repo, docdir, "imports.json"))):
            users.setdefault(p, []).append(name)
    rows = ["| Dendrite doc | Imported by |", "|---|---|"]
    for p in sorted(users):
        link = os.path.relpath(os.path.join(DENDRITE, p), os.path.dirname(SUBPROJECTS_MD))
        rows.append(f"| [`{p}`]({link}) | {', '.join(users[p])} |")
    return "\n".join([TABLE_BEGIN, "", f"{len(users)} Dendrite docs are imported. After editing one, run `uv run scripts/sync_subproject_docs.py`.", ""] + rows + ["", TABLE_END])


def sync_table(check):
    """Regenerate the import table in subprojects.md; return a problem string when it is out of date."""
    text = open(SUBPROJECTS_MD).read()
    start, end = text.find(TABLE_BEGIN), text.find(TABLE_END)
    if start < 0 or end < 0:
        return f"{os.path.relpath(SUBPROJECTS_MD, DENDRITE)} has no {TABLE_BEGIN} block"
    new_text = text[:start] + render_table() + text[end + len(TABLE_END):]
    if new_text == text:
        return None
    if not check:
        with open(SUBPROJECTS_MD, "w") as f:
            f.write(new_text)
        print(f"{os.path.relpath(SUBPROJECTS_MD, DENDRITE)}: import table written")
        return None
    return f"import table out of date in {os.path.relpath(SUBPROJECTS_MD, DENDRITE)}"


def main():
    ap = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    ap.add_argument("--check", action="store_true", help="report problems without writing; exit 1 on any")
    ap.add_argument("--repo", action="append", choices=sorted(REPOS), help="limit to these sub-projects")
    args = ap.parse_args()

    commit, today = dendrite_commit(), datetime.date.today().isoformat()
    failed = False
    for name in args.repo or sorted(REPOS):
        problems, changes = process(name, args.check, commit, today)
        if args.check:
            status = "ok" if not problems else f"{len(problems)} problem(s)"
            print(f"{name}: {status}")
            for p in problems:
                print(f"  - {p}")
            failed |= bool(problems)
        else:
            remaining = [p for p in problems if p.startswith(("source missing", "orphan", "broken", "imports.json"))]
            print(f"{name}: {len(changes)} file(s) written" + ("" if not changes else ": " + ", ".join(changes[:6]) + (" …" if len(changes) > 6 else "")))
            for p in remaining:
                print(f"  ! {p}")
            failed |= bool(remaining)
    if not args.repo:
        problem = sync_table(args.check)
        if problem:
            print(problem)
            failed = True
    return 1 if failed else 0


if __name__ == "__main__":
    sys.exit(main())
