#!/usr/bin/env python3
"""Prepare the GitHub Pages artifact for human and agent docs.

The Pages workflow renders the human site with Jekyll, then this script:

1. Reads docs/site.env for default published URLs.
2. Lets workflow/job env vars override those defaults for staging builds.
3. Replaces URL placeholders in rendered site files.
4. Copies raw Markdown and MDC files to _site/md/ with the same replacements.
5. Publishes the resolved URL config at _site/docs.env for transparency.
"""

from __future__ import annotations

import argparse
import os
import shutil
from pathlib import Path


DEFAULTS = {
    "CLI_DOCS_BASE_URL": "https://agoraio.github.io/cli",
    "CLI_DOCS_MD_BASE_URL": "https://agoraio.github.io/cli/md",
}


def read_env_file(path: Path) -> dict[str, str]:
    values: dict[str, str] = {}
    if not path.exists():
        return values
    for raw in path.read_text(encoding="utf-8").splitlines():
        line = raw.strip()
        if not line or line.startswith("#") or "=" not in line:
            continue
        key, value = line.split("=", 1)
        values[key.strip()] = value.strip().strip('"').strip("'")
    return values


def resolved_values(env_file: Path) -> dict[str, str]:
    values = DEFAULTS | read_env_file(env_file)
    for key in DEFAULTS:
        override = os.environ.get(key, "").strip()
        if override:
            values[key] = override
    values["CLI_DOCS_BASE_URL"] = values["CLI_DOCS_BASE_URL"].rstrip("/")
    values["CLI_DOCS_MD_BASE_URL"] = values["CLI_DOCS_MD_BASE_URL"].rstrip("/")
    return values


def replace_tokens(text: str, values: dict[str, str]) -> str:
    for key, value in values.items():
        text = text.replace(f"@@{key}@@", value)
    return text


def replace_tokens_in_tree(root: Path, values: dict[str, str]) -> None:
    for path in root.rglob("*"):
        if not path.is_file() or path.suffix not in {".html", ".md", ".mdc", ".txt", ".xml"}:
            continue
        original = path.read_text(encoding="utf-8")
        updated = replace_tokens(original, values)
        if updated != original:
            path.write_text(updated, encoding="utf-8")


def copy_raw_markdown(source: Path, site: Path, values: dict[str, str]) -> None:
    destination = site / "md"
    for path in source.rglob("*"):
        if not path.is_file() or path.suffix not in {".md", ".mdc"}:
            continue
        target = destination / path.relative_to(source)
        target.parent.mkdir(parents=True, exist_ok=True)
        content = path.read_text(encoding="utf-8")
        target.write_text(replace_tokens(content, values), encoding="utf-8")
        shutil.copystat(path, target)


def write_resolved_env(site: Path, values: dict[str, str]) -> None:
    body = "".join(f"{key}={value}\n" for key, value in sorted(values.items()))
    (site / "docs.env").write_text(body, encoding="utf-8")


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--source", default="docs", type=Path)
    parser.add_argument("--site", default="_site", type=Path)
    parser.add_argument("--env-file", default=Path("docs/site.env"), type=Path)
    args = parser.parse_args()

    values = resolved_values(args.env_file)
    replace_tokens_in_tree(args.site, values)
    copy_raw_markdown(args.source, args.site, values)
    write_resolved_env(args.site, values)
    print(f"prepared Pages docs with CLI_DOCS_BASE_URL={values['CLI_DOCS_BASE_URL']}")
    print(f"prepared Pages docs with CLI_DOCS_MD_BASE_URL={values['CLI_DOCS_MD_BASE_URL']}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
