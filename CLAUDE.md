# courrier-cli

Terminal client for a [Courrier](https://github.com/FacileStudio/Courrier) instance. Go,
cobra, one binary named `courrier`.

## Commands

| Task | Command |
|---|---|
| Build | `mise run build` |
| Quality gate | `mise run check` |
| Format Go | `mise run format` |
| Enable git hooks | `mise run hooks` |

## Structure

```
main.go            hands off to cmd
cmd/               one file per command; root.go owns flags, exit codes and account
                   resolution, format.go the rendering shared between them
internal/
  client/          the HTTP surface (mail, auth, keys)
  config/          the instance URL, the session token, the default account
  ui/              CLI-STANDARD §7 output vocabulary
integrations/      SKILL.md, the AI agent registration
install.sh         the four-line facile shim; nothing else may go in it
scripts/check.sh   the quality gate
```

Dependencies are cobra, `fatih/color`, `golang.org/x/term`, `gopkg.in/yaml.v3` and
`FacileStudio/porte` for `porte/loopback`. Adding a sixth needs a reason — a client for one
API does not need a framework. `porte/loopback` is standard library only by design, so it
brings none of porte's server half with it; `go list -deps ./...` must stay free of chi,
pgx, oauth2 and go-oidc.

## Conventions

These come from `CLI-STANDARD.md`, which is normative. When this repo disagrees with it,
this repo is wrong. It lived in `FacileStudio/Wiki`, removed on 2026-08-22; until it has a
permanent home, the history is in `~/backups/FacileStudio-Wiki-20260822.bundle`.

- **`Short` and flag help: capitalized, imperative, no trailing period.** `"List mail
  accounts"`, never `"Lists mail accounts."`.
- **No emoji, anywhere.** Not in help, not at runtime. The `▸ ✓ ! ✗` glyphs are the
  standard's severity vocabulary, not emoji; `● *` mark unread and starred, and are data.
- **No inline comments inside function bodies.** Explanation is hoisted into the godoc
  above the declaration. Courrier itself ran a pass to enforce this.
- **All output through `internal/ui`.** Warnings and errors go to stderr; everything else
  to stdout, so a piped command emits only data.
- **`--json` is global**, prints one document and nothing else, and forces colour off.
- **Exit codes**: `0` success, `1` failure, `2` usage, `130` SIGINT. `root.go` maps them;
  `commandStarted` is what distinguishes a usage error from a failed one.
- Errors are lowercase, name what failed, and end with the fix after an em dash — the
  glyph is added by the printer, never baked into the message.
- `--version` prints exactly `courrier <semver>`; the installer parses that line.

## Nothing under internal/ prints

`internal/` returns typed values. `cmd/format.go` is the only place that formats. This is
what keeps a later Charm/Bubble Tea front end cheap: it imports `internal/client` and
renders the same structs, rather than shelling out to this binary or reimplementing its
HTTP layer.

## The login flow

`courrier login` asks the instance what it accepts (`GET /api/auth/config`), then either:

- **SSO**: binds a loopback listener, opens `…/api/auth/oidc?flow=cli&port=N&cli_state=S`,
  waits for the redirect with the one-time code, and trades it at
  `/api/auth/oidc/exchange`. The state nonce is echoed and verified, so a callback from a
  different login is refused — and the listener keeps waiting rather than failing, because
  a browser requests `/favicon.ico` unprompted. The listener is `porte/loopback`, shared
  with the other suite CLIs, and it renders the suite's login page on `127.0.0.1` rather
  than a page this repo owns. `cmd/login.go` supplies the two things that are Courrier's:
  the `/api` mount point and the app name on the page.
- **Password**: prompts for an address and a password, exchanged at `/api/auth/login`.

Both end with a bearer token in `${XDG_CONFIG_HOME:-~/.config}/courrier/config.yml`
(0600), and `logout` revokes it server-side.

## Gotchas

- **Courrier mounts porte under `/api`**, so every auth path carries the prefix — the login
  URL is `/api/auth/oidc`, not `/auth/oidc`.
- **`SSO_ONLY=true` unregisters the local-auth routes**, so `POST /api/auth/login` answers
  **404, not 403**. Read a 404 there as "this instance has no password login".
- **A folder listing takes a folder *type***, not a folder id, and returns **collapsed
  conversations** carrying `message_count`, `unread_count` and `email_ids`.
- **A thread id is a Message-ID** and must be percent-encoded into the path.
- **The dashboard is served from the same origin** by an SPA catch-all, so a mistyped path
  answers 200 with HTML. `internal/client` parses defensively so that reads as a wrong URL.
- **Emails carry both `body_text` and `body_html`.** Never reach for an HTML renderer.
- **`COURRIER_TOKEN` holding a dashboard API token is single-occupancy.** Courrier keeps at
  most one *labelled* porte session per user, so minting a second revokes the first. A
  `courrier login` session is unlabelled and unaffected.
- **API key management is mounted at `/api/apikeys`**. `courrier keys` creates, lists, and revokes them.
- Rate limits: sync 5/min, folder sync 10/min, send 10/min, bulk-action 30/min.
