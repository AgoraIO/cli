#!/usr/bin/env python3
"""Prepare the GitHub Pages artifact for human and agent docs.

The Pages workflow renders the human site with Jekyll, then this script:

1. Reads internal-docs/pages/site.env for default published URLs.
2. Lets workflow/job env vars override those defaults for staging builds.
3. Replaces URL placeholders in rendered site files.
4. Copies raw Markdown and MDC files to _site/md/ with the same replacements.
5. Copies installer scripts into the Pages artifact.
6. Publishes installer metadata at _site/installers.env.
7. Publishes the resolved URL config at _site/docs.env for transparency.
"""

from __future__ import annotations

import argparse
import hashlib
import os
import shutil
from datetime import UTC, datetime
from pathlib import Path


DEFAULTS = {
    "CLI_DOCS_BASE_URL": "https://agoraio.github.io/cli",
    "CLI_DOCS_MD_BASE_URL": "https://agoraio.github.io/cli/md",
    "CLI_INSTALL_SH_URL": "https://dl.agora.io/cli/install.sh",
    "CLI_INSTALL_PS1_URL": "https://dl.agora.io/cli/install.ps1",
    "CLI_INSTALL_SH_FALLBACK_URL": "https://raw.githubusercontent.com/AgoraIO/cli/main/install.sh",
    "CLI_INSTALL_PS1_FALLBACK_URL": "https://raw.githubusercontent.com/AgoraIO/cli/main/install.ps1",
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
    for key in values:
        if key.endswith("_URL"):
            values[key] = values[key].rstrip("/")
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


def sha256(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as handle:
        for chunk in iter(lambda: handle.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def copy_installers(repo_root: Path, site: Path) -> dict[str, str]:
    installers = {
        "INSTALL_SH_SHA256": ("install.sh", site / "install.sh"),
        "INSTALL_PS1_SHA256": ("install.ps1", site / "install.ps1"),
    }
    metadata: dict[str, str] = {}
    for key, (source_name, target) in installers.items():
        source = repo_root / source_name
        if not source.is_file():
            raise FileNotFoundError(f"installer not found: {source}")
        target.parent.mkdir(parents=True, exist_ok=True)
        shutil.copy2(source, target)
        metadata[key] = sha256(source)
    return metadata


def write_installer_metadata(site: Path, metadata: dict[str, str]) -> None:
    values = {
        **metadata,
        "GITHUB_SHA": os.environ.get("GITHUB_SHA", "").strip() or "local",
        "PUBLISHED_AT": datetime.now(UTC).replace(microsecond=0).isoformat().replace("+00:00", "Z"),
    }
    body = "".join(f"{key}={value}\n" for key, value in sorted(values.items()))
    (site / "installers.env").write_text(body, encoding="utf-8")


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--source", default="docs", type=Path)
    parser.add_argument("--site", default="_site", type=Path)
    parser.add_argument("--env-file", default=Path("internal-docs/pages/site.env"), type=Path)
    parser.add_argument("--repo-root", default=Path("."), type=Path)
    args = parser.parse_args()

    values = resolved_values(args.env_file)
    replace_tokens_in_tree(args.site, values)
    copy_raw_markdown(args.source, args.site, values)
    installer_metadata = copy_installers(args.repo_root, args.site)
    write_installer_metadata(args.site, installer_metadata)
    write_resolved_env(args.site, values)
    print(f"prepared Pages docs with CLI_DOCS_BASE_URL={values['CLI_DOCS_BASE_URL']}")
    print(f"prepared Pages docs with CLI_DOCS_MD_BASE_URL={values['CLI_DOCS_MD_BASE_URL']}")
    print(f"prepared Pages installer at {values['CLI_INSTALL_SH_URL']}")
    print(f"prepared Pages PowerShell installer at {values['CLI_INSTALL_PS1_URL']}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
