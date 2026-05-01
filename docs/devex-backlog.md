---
title: DevEx backlog (prioritized)
---

# Developer experience backlog

Prioritized follow-ups from the DevEx review (human + agentic developers).  
Update this file as items ship or priorities change.

## Legend

| Priority | Meaning |
| -------- | ------- |
| **P0** | Blocks adoption or trust (wrong paths, misleading guarantees, security perception). |
| **P1** | High-impact polish for clarity, consistency, or agent reliability. |
| **P2** | Strategic improvements, coordination outside this repo, or deeper test coverage. |

---

## P0

### P0-1 — Contributor clone path matches canonical repo layout

**Problem:** `CONTRIBUTING.md` instructs `cd cli/agora-cli-go` after cloning `AgoraIO/cli`. If the default clone layout differs (single-package repo vs monorepo), new contributors hit a broken path immediately.

**Target files**

- [`CONTRIBUTING.md`](../CONTRIBUTING.md)
- Optionally [`README.md`](../README.md) “Build from source” if it duplicates the path

**Acceptance criteria**

- [ ] Paths match the **actual** default layout of `github.com/AgoraIO/cli` (verify on default branch).
- [ ] If multiple layouts exist (e.g. subtree mirror), document **both** with a one-line “use this if…” disambiguation.
- [ ] `make test` / `go build` instructions run verbatim from a fresh clone.

---

### P0-2 — Enterprise-friendly install entry point

**Problem:** Teams that block `curl | sh` need a obvious first-class path (npm, package managers, manual tarball + verify) without digging through long install docs.

**Target files**

- [`README.md`](../README.md) — short **Enterprise / locked-down environments** subsection near Install.
- [`docs/install.md`](install.md) — optional anchor target if README stays minimal.

**Acceptance criteria**

- [ ] README surfaces **non–pipe-to-shell** options in under ~15 lines (links out to `docs/install.md` for detail).
- [ ] Mentions checksum / Cosign verification pointers (link to existing README or install sections).
- [ ] No new promises beyond what installers actually support today.

---

## P1

### P1-1 — Align user-facing copy with `project doctor --deep` stability

**Problem:** `AGENTS.md` notes `--deep` is not fully stable yet. User-facing docs must not imply guarantees agents/contributors do not have.

**Target files**

- [`README.md`](../README.md), [`docs/commands.md`](commands.md) (generated), [`docs/troubleshooting.md`](troubleshooting.md)
- [`internal/cli/`](../internal/cli/) — Cobra `Long:` / `Example:` for `project doctor` if needed

**Acceptance criteria**

- [ ] Public docs describe `--deep` **exactly** as implemented (or omit until stable).
- [ ] `go run ./cmd/gendocs` / `make docs-commands` refreshed if command text changes.

---

### P1-2 — Agent discovery: MCP prerequisites + tool surface in one place

**Problem:** Agents that fetch `llms.txt` but do not run the binary first still need a compact map of MCP tools and auth prerequisites (`agora login` on host).

**Target files**

- [`docs/llms.txt`](llms.txt)
- [`docs/automation.md`](automation.md) — short subsection if you want normative detail beyond llms.txt

**Acceptance criteria**

- [ ] `llms.txt` links or summarizes: MCP auth model (login on host), transport (`stdio`), and pointer to [`docs/automation.md`](automation.md) for JSON/MCP alignment.
- [ ] Tool list stays maintainable (either generated snippet from code or explicit list with “refresh when MCP surface changes” note in [`AGENTS.md`](../AGENTS.md)).

---

### P1-3 — JSON schema / envelope versioning policy

**Problem:** [`docs/schema/envelope.v1.json`](schema/envelope.v1.json) implies future versions; agents need a single place for deprecation and additive-only vs breaking rules.

**Target files**

- [`docs/automation.md`](automation.md)
- [`CHANGELOG.md`](../CHANGELOG.md) — for any breaking envelope changes

**Acceptance criteria**

- [ ] Document how `envelope.vN.json` relates to releases (e.g. additive fields OK in minor; breaking only major).
- [ ] Document where breaking changes are announced (changelog + automation.md “Migration” subsection).

---

## P2

### P2-1 — Contract tests for automation examples

**Problem:** Golden tests cover slices of introspect; broader drift between `docs/automation.md` examples and actual CLI output can still slip through.

**Target files**

- [`internal/cli/integration_*_test.go`](../internal/cli/), [`internal/cli/golden_test.go`](../internal/cli/golden_test.go)
- [`docs/automation.md`](automation.md)

**Acceptance criteria**

- [ ] Representative commands from automation.md are exercised in CI (or key paths extracted to shared test fixtures).
- [ ] Failure messages point authors to `docs/automation.md` / `make docs-commands` / error-code script as appropriate.

---

### P2-2 — Cross-link from Agora product / Console developer surfaces

**Problem:** Developers and agents often land on product docs first; discovery of CLI mirror URLs is faster with inbound links.

**Target:** Agora properties **outside** this repository (Console, developer portal, RTC docs). Track as a DevRel coordination task.

**Acceptance criteria**

- [ ] At least one canonical product doc page links to `https://agoraio.github.io/cli/` and `https://agoraio.github.io/cli/md/`.
- [ ] Optional: link `https://agoraio.github.io/cli/llms.txt` for agent retrieval.

---

### P2-3 — GitHub Pages accessibility pass

**Problem:** Custom theme is good for branding; WCAG contrast and keyboard nav should be validated.

**Target files**

- [`docs/_layouts/default.html`](_layouts/default.html), [`docs/assets/css/site.css`](assets/css/site.css)

**Acceptance criteria**

- [ ] Spot-check focus states, heading order, and contrast on home + one inner page (light/dark).
- [ ] Fix any **obvious** issues (missing button labels already partially addressed on index copy button — extend as needed).

---

## Changelog

| Date | Change |
| ---- | ------ |
| 2026-05-01 | Initial backlog from DevEx review. |
