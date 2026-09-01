# Changelog

All notable changes to this project are documented here. The format is
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and the project
follows [Semantic Versioning](https://semver.org/spec/v2.0.0.html). While on
`0.x`, a breaking change bumps the minor.

Every entry below was reconstructed from git history on 2026-08-24, so they
record what shipped rather than what was written down at the time.

## [Unreleased]

## [0.3.0] - 2026-09-01

### Added

- `courrier keys` command group for managing API keys (`list`, `create`, `revoke`).
- Support for generating secret backend keys and public browser keys with allowed origins and daily request quotas.
- Client methods `ListKeys`, `CreateKey`, and `RevokeKey` on `*client.Client`.

## [0.2.0] — 2026-08-30

### Changed

- The SSO login listener is now `porte/loopback` rather than this repo's
  `internal/loopback`. The behaviour is the same flow with the same guarantees:
  an ephemeral loopback port, an exact state comparison, a refused callback that
  leaves the login open, the two second shutdown grace and the three minute
  timeout. What changes is the page the browser lands on. It is now the suite's
  shared hand-off page instead of eight lines of inline styles, so a login that
  ends in a browser looks the same in every Facile CLI. Refusals render that
  same page instead of `text/plain`.
- `porte/loopback` is standard library only, so the module adds nothing to the
  binary but itself: `go list -deps ./...` names no chi, pgx, oauth2 or go-oidc.

### Removed

- `internal/loopback`. It was lifted into `porte/loopback` and the tests went
  with it.

## [0.1.1] — 2026-08-24

Release plumbing only. No change to the binary's behaviour.

### Fixed

- The Homebrew formula now publishes. The `brews` block resolves
  `{{ .Env.HOMEBREW_TAP_GITHUB_TOKEN }}` and the repo holds the secret, but
  `release.yml` never mapped it into the goreleaser step, so the run would
  publish the GitHub release in full and then die on a template error. The
  default `GITHUB_TOKEN` cannot push to `homebrew-tap`, which is why the
  separate token exists.

### Added

- `release.yml` publishes a Homebrew formula to `FacileStudio/homebrew-tap` on
  a tag.

### Removed

- The filet gate is out of the lefthook config. Seven repos carry a `filet.yml`
  without running filet in their gate, so enabling it here alone was
  inconsistent. It stays an explicit opt-in.

## [0.1.0] — 2026-08-22

### Added

- First release. A terminal client for Courrier, so mail is reachable from a
  shell and from an agent instead of hand-rolled curl. `list`, `read`, `search`,
  `send`, `mark`, `sync`, `folders`, `accounts`, `login` and `logout`, against
  a porte v0.2.10 app mounted under `/api`.
- Sign-in through porte's loopback SSO flow, modelled on journal-cli.
- `--json` on every data command, with data on stdout and everything else on
  stderr. Exit codes 0, 1, 2 and 130.
- XDG config at `config.yml`, created 0600 and tightened on read. Precedence is
  flag, then environment, then file, then default.
- An agent-facing `SKILL.md` under `integrations/`.

### Fixed

- `logout` clears the credential even when the config cannot be parsed.
  `Clear()` called `Load()` first, so a malformed `config.yml` aborted logout
  with a raw parser error and left the token exactly where somebody had just
  tried to delete it. The URL and default account are unrecoverable in that
  case, so they reset. Inherited from journal-cli, where a review turned it up.
- `Save()` re-asserts the file mode on the open handle. `OpenFile`'s perm
  argument applies only at creation, so an existing file kept whatever mode it
  had.

### Changed

- The tracked githooks are replaced by lefthook, using the shared config in
  `FacileStudio/hooks` pinned at v1.0.0. `scripts/check.sh` stays as a local
  pre-push job, and `mise install` wires the hooks, so `mise run hooks` is gone.
- The README and ROADMAP no longer link `CLI-STANDARD.md`. `FacileStudio/Wiki`
  was removed on 2026-08-22 and the document is not anywhere else in the org, so
  the links 404. The standard is still what this repo is built to and is still
  named; only the URL is dropped.

[Unreleased]: https://github.com/FacileStudio/courrier-cli/compare/v0.3.0...HEAD
[0.3.0]: https://github.com/FacileStudio/courrier-cli/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/FacileStudio/courrier-cli/compare/v0.1.2...v0.2.0
[0.1.2]: https://github.com/FacileStudio/courrier-cli/compare/v0.1.1...v0.1.2
[0.1.1]: https://github.com/FacileStudio/courrier-cli/compare/v0.1.0...v0.1.1
[0.1.0]: https://github.com/FacileStudio/courrier-cli/releases/tag/v0.1.0
