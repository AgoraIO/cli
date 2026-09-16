---
title: Using Quickstart env files
---

# Using Quickstart env files

For official Next.js, Python, Go, and Android Quickstarts, Agora CLI creates or updates
the runtime-specific env file with the Agora App ID and App Certificate for the
selected project. It does not download a ready-made dotenv file from Console.
The CLI starts with the example env file from the cloned repository, then
writes the credential keys required by that runtime.

## How the file is created

| Command | Behavior |
|---------|----------|
| `agora init <name> --template <id>` | Clones the Quickstart, selects or creates a project, and writes its env file. |
| `agora quickstart create ...` | Writes the env file when a project is resolved; use `--template-only` to explicitly clone without credentials. Interactive runs prompt when no project resolves. |
| `agora quickstart env write [dir]` | Creates or updates the runtime-specific env file in an existing Quickstart. |
| `agora project env write [path]` | Creates or updates a dotenv file at the selected path without cloning a Quickstart. |

Quickstart env layouts:

| Quickstart | Example source | Target path | Credential keys |
|------------|----------------|-------------|-----------------|
| Next.js | `env.local.example` | `.env.local` | `NEXT_PUBLIC_AGORA_APP_ID`, `NEXT_AGORA_APP_CERTIFICATE` |
| Python | `server/.env.example` | `server/.env` | `AGORA_APP_ID`, `AGORA_APP_CERTIFICATE` |
| Go | `server/.env.example` | `server/.env` | `AGORA_APP_ID`, `AGORA_APP_CERTIFICATE` |
| Android | `server/.env.example` | `server/.env.local` | `AGORA_APP_ID`, `AGORA_APP_CERTIFICATE` |

If the target env file already exists, the CLI uses it as the starting content
and updates the Agora credential keys while preserving unrelated entries. If
the target does not exist, the CLI starts from the Quickstart's example file.
If neither file exists, it creates a new file containing the credential entries.

To refresh credentials or switch the Quickstart to another project, run the env
write command again with the target project. The CLI updates the same env file
in place:

```bash
cd <quickstart>
agora quickstart env write . --project <project-id-or-name>
```

Prefer `agora quickstart env write` for official Quickstarts. Use
`agora project env write <path>` when you want to write credentials to a
specific dotenv path outside the official Quickstart layout. Both commands use
the same credential merge rules: unrelated entries are preserved, current keys
are updated, and unsupported legacy credential names are commented out.

## Where the credentials come from

After authentication, the CLI fetches the selected project's details from the
Agora CLI project API. The App ID and App Certificate returned for that project
are written to the local env file.

Commands that resolve an existing project context, including `quickstart create`,
`quickstart env write`, and `project env write`, use this precedence:

1. Explicit `--project <id-or-name>`
2. Repo-local `.agora/project.json`
3. Global project context set by `agora project use`

`agora init` has a separate onboarding flow. Use `--project` to select an
existing project or `--new-project` to force creation. Without either flag, it
prefers a project named `Default Project`, prompts in an interactive terminal,
uses the most recently created project in non-interactive runs, or creates a
project when none exist.

The selected project must have an App Certificate. If it does not, enable one
in Agora Console or select a different project before writing the env file.

Restart the development server after updating the env file so it reloads the
new values.

## Keep credentials private

The env file can contain an App Certificate. Do not commit it to version
control, paste its values into issues or logs, or share it outside the intended
development environment. Confirm that the file is covered by the repository's
`.gitignore` rules.
