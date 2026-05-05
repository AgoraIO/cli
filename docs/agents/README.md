---
title: Agent Rules
---

# Agent Rules For Agora Projects

These snippets help AI coding agents use Agora CLI safely and consistently in app repositories.

Use the CLI to scaffold rules into a new quickstart:

```bash
agora init my-nextjs-demo --template nextjs --add-agent-rules cursor
agora init my-python-demo --template python --add-agent-rules claude
agora init my-go-demo --template go --add-agent-rules windsurf
```

Available rule targets:

- [Cursor](cursor.mdc)
- [Claude Code](claude.md)
- [Windsurf](windsurf.md)

## Safety contract

`--add-agent-rules` never destroys your existing agent configuration:

- If the destination file (for example `CLAUDE.md`) does not exist yet, the CLI creates it with the Agora rules block.
- If the destination file exists and does **not** contain a previously-managed block, the CLI **appends** a new block to the end of the file, preserving every prior line you (or another tool) wrote.
- If the destination file already contains a previously-managed block, the CLI replaces only the contents between the markers — content before and after the markers stays exactly as you left it.

Each managed block is wrapped in HTML-comment sentinels:

```text
<!-- agora-cli:agent-rules:start -->
...rules...
<!-- agora-cli:agent-rules:end -->
```

You can move the block around inside the file or wrap it with your own commentary; subsequent runs will still recognize and refresh it.

