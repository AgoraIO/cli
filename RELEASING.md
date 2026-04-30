# Releasing Agora CLI

Releases are fully automated via GoReleaser. Pushing a `v*` tag is the only manual step.

## Release

```bash
git tag v0.1.7
git push origin v0.1.7
```

The release workflow (`.github/workflows/release.yml`) then:

1. **GoReleaser** builds and publishes everything in parallel:
   - Cross-platform binaries (Linux, macOS, Windows — amd64 + arm64)
   - Archives: `.tar.gz` (Unix), `.zip` (Windows)
   - Linux packages: `.deb`, `.rpm`, `.apk`
   - GitHub release with auto-generated changelog and checksums
   - Docker images → GitHub Container Registry (`ghcr.io/{owner}/agora-cli`)

2. **npm publish** job (active):
   - Downloads the release archives, verifies them against `checksums.txt` (SHA-256), and refuses to publish on mismatch
   - Stages the per-platform binary into each `@agoraio/cli-{os}-{arch}` package
   - Stamps the tag version into all package.json files (wrapper + 6 platform packages)
   - Publishes the six per-platform packages with `npm publish --provenance`
   - Publishes the wrapper package (`agoraio-cli`) with `npm publish --provenance`
   - Runs a post-publish smoke test: `npx --yes agoraio-cli@<tag> --version` with retry/backoff to handle registry propagation
   - Requires `NPM_TOKEN` secret with publish access to `agoraio-cli` and `@agoraio/*`
   - Requires `id-token: write` workflow permission for sigstore-backed npm provenance attestations

3. **Apt repository** job (triggered by the published release):
   - Downloads `.deb` files from the release
   - Rebuilds the signed apt repo on GitHub Pages
   - Requires `APT_SIGNING_KEY` secret + `APT_SIGNING_KEY_ID` variable

## Local Verification

Before cutting a tag:

```bash
go test ./...
go build -o agora .
./agora --help
./agora whoami

# Dry-run GoReleaser to catch config errors before the real release:
goreleaser release --snapshot --clean
```

## Manual npm dry-run (no tag required)

The release workflow exposes a `workflow_dispatch` trigger that runs the npm publish job in `--dry-run` mode against a synthetic version tag. Use this to validate npm packaging changes (metadata, scripts, provenance permissions) without minting a real GitHub release:

1. GitHub → Actions → Release → Run workflow → leave `dry_run` set to `true`.
2. Inspect the job logs for what would be published, including provenance request and tarball contents.
3. The smoke-test step is skipped in dry-run mode (nothing was actually published).

## Pre-tag checklist (npm)

Before tagging the first real release that ships npm, confirm:

- [ ] `NPM_TOKEN` secret is set in the repo (Settings → Secrets and variables → Actions). Token must have publish access to `agoraio-cli` (unscoped) and the `@agoraio` scope.
- [ ] `agoraio-cli` package name on npmjs.com is owned by the Agora npm org / publisher and not squatted.
- [ ] `@agoraio` scope on npmjs.com is owned by the Agora npm org and the `NPM_TOKEN` user is a member with publish access.
- [ ] The workflow has `id-token: write` permission (already set in `release.yml`); npm provenance requires it.
- [ ] A `workflow_dispatch` dry-run on the current `main` succeeds end-to-end (validates packaging, scripts, provenance).
- [ ] First publish should be a release-candidate tag (e.g. `v0.1.x-rc.1`) so an unexpected failure does not affect a "latest" tag in the registry.

## Required Secrets and Variables

| Name | Type | Required for |
|------|------|-------------|
| `NPM_TOKEN` | secret | npm publish (active) |
| `APT_SIGNING_KEY` | secret | Signed apt repo on GitHub Pages |
| `APT_SIGNING_KEY_ID` | variable | Signed apt repo on GitHub Pages |

Homebrew and Scoop are not part of the current GoReleaser config. Add `brews:` / `scoops:` blocks before documenting them as automated channels.

## Distribution Channels

| Channel | How |
|---------|-----|
| Homebrew | Coming soon; direct installer is current primary macOS path |
| npm (convenience) | Active; published with provenance from `release.yml` |
| apt/deb (Debian/Ubuntu) | apt-repo.yml → GitHub Pages |
| rpm (RHEL/Fedora) | Release artifact (.rpm via GoReleaser) |
| apk (Alpine/Docker) | Release artifact (.apk via GoReleaser) |
| Scoop (Windows) | Coming soon |
| Docker (GHCR) | GoReleaser dockers block |
| Shell install script | `install.sh` downloads from GitHub Releases |
| Winget (Windows) | Manual: submit PR to microsoft/winget-pkgs |

## Rollback (npm)

If a published version is bad:

- Use `npm deprecate agoraio-cli@<bad-version> "<reason and recommended version>"` to warn anyone who installs it.
- Cut a fixed patch release as soon as possible.
- **Do not** `npm unpublish` (irreversible reputational damage and registry policy restricts unpublishing after 72 hours anyway).

## One-Time Setup Checklist

- [ ] Enable GitHub Pages on this repo (Settings → Pages → Source: `gh-pages` branch)
- [ ] Generate GPG key for apt signing; set `APT_SIGNING_KEY` and `APT_SIGNING_KEY_ID`
- [ ] Set `NPM_TOKEN` with publish access to `agoraio-cli` and `@agoraio/*`
- [ ] Verify `@agoraio` scope ownership on npmjs.com
- [ ] Run a `workflow_dispatch` dry-run of the release workflow to validate npm packaging
- [ ] Add Homebrew and Scoop GoReleaser blocks before announcing those channels
- [ ] Submit first Winget manifest PR to `microsoft/winget-pkgs` after the first release
