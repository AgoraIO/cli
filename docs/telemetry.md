---
title: Telemetry
---

# Telemetry

Agora CLI telemetry is limited to operational diagnostics such as command failures and local log metadata. It must never include OAuth tokens, app certificates, dotenv secrets, or project env values.

Telemetry is enabled by default in the local config. You can inspect or change the setting:

```bash
agora telemetry status
agora telemetry disable
agora telemetry enable
```

For scripts, use JSON:

```bash
agora telemetry disable --json
```

`DO_NOT_TRACK=1` disables telemetry at runtime even if the config file says telemetry is enabled. It also suppresses local diagnostic log writes for that process:

```bash
DO_NOT_TRACK=1 agora project list --json
```

The config file location is available with:

```bash
agora config path
```
