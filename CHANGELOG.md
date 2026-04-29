# Changelog

All notable changes to Agora CLI are documented in this file.

This project follows semantic versioning for released CLI versions. The top section tracks the next release branch until it is tagged.

## 0.1.7

### Added

- Add an interactive sign-in prompt for human CLI sessions when an account connection is required and no local session exists. The prompt defaults to yes on Enter and launches the existing OAuth login flow.
- Add this changelog so users can review notable CLI changes from version to version.

### Changed

- Update installer, README, install docs, and Homebrew formula references from `AgoraIO-Community/cli` to `AgoraIO/cli`.
- Keep automation non-interactive when auth is missing. JSON output, `AGORA_OUTPUT=json`, CI, and non-TTY runs still fail fast with the existing login-required error instead of prompting.
- Update `agora init` project reuse to prefer a project named `Default Project`, then the project with the latest `createdAt` value from the current results page.

### Documentation

- Document the interactive-auth behavior and `init` default-project fallback in `docs/automation.md`.

## 0.1.6

### Fixed

- Update GoReleaser Docker image and manifest templates to lowercase the GitHub repository owner before publishing to GHCR, which requires lowercase registry paths.

## 0.1.5

### Changed

- Scope the release workflow to installer-supported artifacts while npm, Homebrew tap, and Scoop bucket publishing remain disabled.
- Keep GoReleaser archive naming stable for shell and PowerShell installers.
- Keep Docker image publishing through GoReleaser with per-architecture images and manifests.

## 0.1.4

### Added

- Provide the native Agora CLI command model for auth, project management, quickstart setup, and the composed `init` onboarding flow.
- Support OAuth login and logout through `agora login`, `agora auth login`, `agora logout`, and `agora auth logout`.
- Support session inspection through `agora whoami` and `agora auth status`.
- Support project creation, selection, env export, env file writes, and readiness checks through the `project` command group.
- Support official quickstart cloning and template-specific env file generation through the `quickstart` command group.
- Support `agora init` as the recommended end-to-end onboarding command that creates or reuses an Agora project, clones a quickstart, writes env, persists context, and prints next steps.
- Support machine-readable JSON output for automation and agent workflows.
- Ship automated release packaging through GoReleaser, including cross-platform archives, Linux packages, Homebrew, Scoop, npm wrapper packages, Docker images, and install scripts.
