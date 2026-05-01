---
title: Agora CLI Docs
---

# Agora CLI Docs

Agora CLI is the native command-line tool for Agora authentication, project management, quickstart setup, and developer onboarding.

## Start Here

- [Install options](install.html)
- [Command reference](commands.html)
- [Automation and JSON contract](automation.html)
- [Stable error codes](error-codes.html)
- [Telemetry controls](telemetry.html)

## Agent Markdown

The same docs are published as raw Markdown under predictable `/md/` URLs for agents and scripts:

- [Markdown index](@@CLI_DOCS_MD_BASE_URL@@/index.md)
- [Markdown command reference](@@CLI_DOCS_MD_BASE_URL@@/commands.md)
- [Markdown automation contract](@@CLI_DOCS_MD_BASE_URL@@/automation.md)
- [Markdown error codes](@@CLI_DOCS_MD_BASE_URL@@/error-codes.md)
- [Markdown install guide](@@CLI_DOCS_MD_BASE_URL@@/install.md)
- [Markdown telemetry controls](@@CLI_DOCS_MD_BASE_URL@@/telemetry.md)
- [Markdown agent rules guide](@@CLI_DOCS_MD_BASE_URL@@/agents/README.md)

## Local Preview

```bash
make docs-preview
```

That builds the themed HTML site locally with system light/dark mode and serves the raw Markdown tree under `/md/`.

## Common Commands

```bash
agora login
agora init my-nextjs-demo --template nextjs
agora project doctor --json
agora --help --all
```

