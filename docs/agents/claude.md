# Agora CLI Agent Rules

- Use `agora --help --all --json` or `agora introspect --json` for command discovery.
- Prefer `--json` for commands whose output you need to parse.
- Set `AGORA_HOME` to an isolated temporary directory in CI or multi-agent runs.
- Use explicit `--project <id-or-name>` when working outside a bound quickstart.
- Run `agora project doctor --json` before assuming a project is ready.
- Do not parse pretty output; it is for humans.
- Do not print App Certificates, session tokens, or `.env` secret values.

