---
title: Supply-chain hardening proposal
status: proposed
target-release: next
---

# Supply-chain hardening proposal

> Status: **proposed**, awaiting approval. Do not implement until the
> proposal is signed off and scheduled for the next release.

This proposal closes the gap between the current Agora CLI release
pipeline and the supply-chain expectations of flagship Go CLIs (`gh`,
`k9s`, `gitleaks`, `golangci-lint`, Charm tools). The CLI already ships
SBOMs (Syft), Cosign-signed `checksums.txt`, and npm OIDC provenance.
This proposal adds the missing pieces required to pass enterprise
security questionnaires and to give downstream consumers a single,
verifiable trust chain from `git tag` to installed binary.

## Goals

1. Every artifact published from this repository — GitHub Release zips,
   npm packages, Docker images, deb/rpm/apk packages — has a verifiable
   provenance attestation traceable to a tagged commit and a recorded
   GitHub Actions run.
2. Pull requests block on dependency vulnerability and code-scanning
   findings before merge, not on a weekly cron.
3. Installer scripts (`install.sh`, `install.ps1`) optionally verify
   the Cosign signature on `checksums.txt` so users with `cosign`
   installed get end-to-end attestation, not just SHA-256 verification.
4. We have a documented, reproducible answer to "how do I verify this
   binary?" published at a stable URL.

## Non-goals

- Switching to keyed Cosign signing. Keyless OIDC remains the chosen
  trust model.
- Adding an internal artifact registry. We continue to publish to
  GitHub Releases, npm, and GHCR.
- Reproducible builds beyond what GoReleaser already provides.

## Proposed changes

### 1. Add `actions/attest-build-provenance` to release workflow

Currently `release.yml` produces npm provenance (via `--provenance` and
`id-token: write`) but the GitHub Release zips themselves carry no
attestation. Add a step after GoReleaser that attests every archive,
checksum file, and SBOM.

```yaml
# .github/workflows/release.yml (sketch)
- name: Attest release artifacts
  uses: actions/attest-build-provenance@v1
  with:
    subject-path: |
      dist/*.tar.gz
      dist/*.zip
      dist/checksums.txt
      dist/*.spdx.json
```

Result: every release artifact gets a SLSA-compatible provenance entry
visible in the GitHub UI and verifiable with `gh attestation verify`.

### 2. Add CodeQL workflow

```yaml
# .github/workflows/codeql.yml
name: CodeQL
on:
  pull_request:
    branches: [main]
  push:
    branches: [main]
  schedule:
    - cron: '37 9 * * 1'
permissions:
  actions: read
  contents: read
  security-events: write
jobs:
  analyze:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: github/codeql-action/init@v3
        with:
          languages: go
      - uses: github/codeql-action/autobuild@v3
      - uses: github/codeql-action/analyze@v3
```

Result: GitHub Security tab gets Go CodeQL findings; PRs surface
findings as inline annotations.

### 3. Add Dependency Review on PRs

```yaml
# .github/workflows/dependency-review.yml
name: Dependency Review
on:
  pull_request:
permissions:
  contents: read
  pull-requests: write
jobs:
  review:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/dependency-review-action@v4
        with:
          fail-on-severity: high
          comment-summary-in-pr: on-failure
```

Result: PRs introducing high-severity vulnerable dependencies are
blocked at review time, not at the next weekly govulncheck cron.

### 4. Add OSV-Scanner SARIF upload

Augment the existing `govulncheck.yml` with a parallel OSV-Scanner job
that uploads SARIF to the Security tab so non-Go transitive issues
(e.g. JS in npm wrapper) are also visible.

```yaml
# .github/workflows/osv-scanner.yml (sketch)
jobs:
  scan:
    permissions:
      security-events: write
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: google/osv-scanner-action/osv-scanner-action@v1
        with:
          scan-args: |-
            --recursive
            --skip-git
            --format=sarif
            --output=osv.sarif
            ./
      - uses: github/codeql-action/upload-sarif@v3
        with:
          sarif_file: osv.sarif
```

### 5. Add gitleaks pre-commit and CI secret scan

```yaml
# .github/workflows/secret-scan.yml
name: Secret scan
on:
  pull_request:
  push:
    branches: [main]
jobs:
  gitleaks:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0
      - uses: gitleaks/gitleaks-action@v2
        env:
          GITLEAKS_LICENSE: ${{ secrets.GITLEAKS_LICENSE }}
```

Note: gitleaks-action is free for public repos; no license key needed
for this repo. Document that contributors should also install the
gitleaks pre-commit hook locally.

### 6. Have installers consume Cosign verification

`install.sh` already verifies SHA-256. Optionally consume the existing
Cosign signature when the user has `cosign` installed:

```sh
# install.sh sketch
verify_cosign_optional() {
  if command -v cosign >/dev/null 2>&1; then
    say_step "Verifying checksums.txt signature with cosign..."
    cosign verify-blob \
      --certificate "${CHECKSUMS_PATH}.pem" \
      --signature   "${CHECKSUMS_PATH}.sig" \
      --certificate-identity-regexp "https://github.com/AgoraIO/cli/.github/workflows/release.yml@refs/tags/v.*" \
      --certificate-oidc-issuer "https://token.actions.githubusercontent.com" \
      "$CHECKSUMS_PATH" >/dev/null
    say_ok "cosign signature verified"
  fi
}
```

Result: users with cosign get end-to-end OIDC trust verification with
zero extra steps. Users without it keep the SHA-256-only path.

### 7. Add `make verify-release` target

A local convenience target so contributors and downstream consumers
can replay the full verification chain (SHA-256 + Cosign + SBOM
inspection) against a published version.

```make
# Makefile sketch
verify-release:
	@scripts/verify-release.sh "$${VERSION:?VERSION required, e.g. VERSION=0.2.0 make verify-release}"
```

### 8. Document the trust chain

Add a `Verifying releases` section to [docs/install.md](../install.md)
that walks through:

1. `cosign verify-blob` against `checksums.txt`.
2. `sha256sum -c` against an extracted archive.
3. `syft` to confirm the SBOM matches the binary.
4. `gh attestation verify` for the new build provenance.

## Reasoning per item

| # | Why we need it |
|---|----------------|
| 1 | npm provenance is a great precedent, but enterprise customers also consume the GitHub Release zips directly. Without attestation those zips are effectively unsigned from the consumer's point of view. |
| 2 | Govulncheck catches Go vulnerabilities in shipped deps but not Go code-quality issues like SQL injection patterns or path traversal. CodeQL is the industry default and surfaces in the Security tab GitHub uses for supply-chain scoring. |
| 3 | A weekly govulncheck cron is too late for security-conscious downstream consumers. Block at PR time. |
| 4 | npm wrapper / install scripts can pull JS/Python tooling into the supply chain over time. OSV-Scanner sees those; govulncheck does not. |
| 5 | We have OAuth flows, a hardcoded Sentry DSN (planned wire-in), and example config snippets. Secret scanning prevents future regressions where a real key lands in Git history. |
| 6 | We already sign `checksums.txt` keyless with Cosign, but no consumer verifies that signature today. Wiring it into the installer (best-effort, optional) raises the trust ceiling at zero UX cost. |
| 7 | Makes the verification story a single command, removing "I tried but the cosign incantation was wrong" friction. |
| 8 | Documents the trust chain so security questionnaires get a published URL instead of a back-and-forth email thread. |

## Risks / open questions

- **CodeQL false positives.** Go CodeQL has a high signal-to-noise
  ratio compared to JS/Python but we should expect to triage and
  suppress 5-15 findings on the first run.
- **OSV-Scanner SARIF noise.** The `packaging/npm/agoraio-cli/`
  wrapper has minimal deps but every transitive bump will surface.
  Consider scoping the scan to the Go module on day one and adding
  npm later.
- **Installer change risk.** The Cosign verify branch must remain
  best-effort. A failed verify-blob on a flaky network must not break
  the install. Existing SHA-256 path stays mandatory.
- **Workflow runtime.** Adding CodeQL adds ~5 minutes per PR. Acceptable.

## Rollout plan

1. Land items 2 (CodeQL), 3 (Dependency Review), 4 (OSV-Scanner), 5
   (gitleaks) in one PR. These are pure-add CI workflows.
2. Land item 1 (attest-build-provenance) in the next release PR so it
   exercises against a real tag.
3. Land items 6 (installer cosign), 7 (`make verify-release`), 8 (docs)
   together so the documented verification flow matches what the
   installer does.
4. Update [docs/install.md](../install.md) "Security" section and add
   a callout to the README and `SECURITY.md`.

## Out of scope

- Replacing GoReleaser with `slsa-github-generator` for the binary
  build step. The current GoReleaser build is well-tuned and
  attest-build-provenance gives equivalent SLSA coverage at lower
  switching cost.
- Adding signed git tags. Useful but orthogonal.
