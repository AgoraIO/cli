# Security Policy

We take the security of Agora CLI seriously. Thank you for helping us keep
developers and their Agora projects safe.

## Supported Versions

Security fixes are issued against the latest minor release line. Older
minor lines are best-effort only.

| Version | Supported          |
| ------- | ------------------ |
| 0.2.x   | Yes                |
| < 0.2.0 | Not supported. Please upgrade with `agora upgrade` or reinstall via [docs/install.md](docs/install.md). |

## Reporting a Vulnerability

Please do **not** file a public GitHub issue for suspected security
vulnerabilities. Instead:

1. Email **security@agora.io** with the subject line
   `[agora-cli] vulnerability report`.
2. Include:
   - A description of the issue and the impact you observed.
   - Steps to reproduce, ideally with `agora --version` output and the
     `agora project doctor --json` envelope when relevant.
   - Any proof-of-concept code or logs (avoid attaching real Agora App
     Certificates or OAuth tokens; redact them as `[REDACTED]`).
3. We will acknowledge your report within **3 business days** and aim to
   provide a remediation plan or status update within **10 business days**.

If you do not receive an acknowledgement within 3 business days, please
follow up at **devrel@agora.io** so we can route your report internally.

## Disclosure Process

- We coordinate disclosure with reporters. Once a fix is available, we
  publish a release with security notes in
  [`CHANGELOG.md`](CHANGELOG.md) and credit reporters who wish to be
  acknowledged.
- For high-severity issues we may publish a GitHub Security Advisory and
  request a CVE.
- Please give us a reasonable window (typically **90 days** or until a
  fix ships, whichever is sooner) before public disclosure.

## Safe Harbor

We support good-faith security research. As long as you:

- avoid privacy violations, destruction of data, and interruption or
  degradation of Agora services,
- only test against accounts and projects you own (or have explicit
  permission to test), and
- give us a reasonable opportunity to respond before public disclosure,

we will not pursue or support legal action against you for your research.
This safe harbor applies to the Agora CLI binary and the install scripts
distributed from this repository. Other Agora services have their own
disclosure programs at [agora.io/security](https://www.agora.io/en/about-us/security/).

## Verifying Release Artifacts

Every release ships with a SHA-256 `checksums.txt` and a Cosign keyless
signature. The official installers verify the SHA-256 automatically. To
verify Cosign attestations manually, see the **Security** section of
[docs/install.md](docs/install.md).

## Out of Scope

The following are out of scope for security reports against this repository:

- Vulnerabilities in third-party Agora services (use the channels listed
  at [agora.io/security](https://www.agora.io/en/about-us/security/)).
- Vulnerabilities in user-cloned quickstart applications. Those repos
  have their own SECURITY.md files.
- Issues that require physical access to the user's machine, root access
  granted by the user, or a compromised package mirror outside the
  GitHub Releases / npm / Homebrew channels we publish to.
