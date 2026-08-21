# courrier-cli — roadmap

Terminal client for [Courrier](https://github.com/FacileStudio/Courrier), the suite's
self-hosted email client. Go, cobra, one binary named `courrier`.

Conventions come from [`Wiki/CLI-STANDARD.md`](https://github.com/FacileStudio/Wiki/blob/main/CLI-STANDARD.md),
which is normative. When this repo disagrees with it, this repo is wrong.

## Why this exists

Courrier exposes 58 HTTP routes and already accepts a porte bearer token on every
authenticated route, but has no terminal or agent surface: reaching mail from a shell means
hand-rolling curl with a token. Every comparable suite service has a CLI — `journal-cli`,
`antenne-cli`, `capsule-cli` — and there was no `courrier` entry in `facile`'s catalog.

## Shape

`journal-cli` is the reference implementation. It targets a porte v0.2.10 app that mounts
porte under `/api`, which is exactly Courrier's configuration, so the login flow is
identical rather than merely similar.

```
main.go            hands off to cmd
cmd/               one file per command; root.go owns flags, exit codes and account
                   resolution, format.go the rendering shared between them
internal/
  client/          the HTTP surface — returns typed values, never formatted strings
  config/          the instance URL, the session token, the default account
  loopback/        the porte SSO one-time-code listener
  ui/              CLI-STANDARD §7 output vocabulary
integrations/      SKILL.md, the AI agent registration
install.sh         the four-line facile shim
```

Dependencies are cobra, `fatih/color`, `golang.org/x/term` and `gopkg.in/yaml.v3`. A fifth
needs a reason — a client for one API does not need a framework.

## The rule that keeps the TUI cheap

**Nothing under `internal/` prints.** Rendering lives in `cmd/format.go`. A later
Charm/Bubble Tea front end imports `internal/client` and renders the same structs, instead
of shelling out to this binary or reimplementing its HTTP layer.

## v1 — shipped scope

- [ ] `courrier login` / `logout` — porte loopback SSO, password fallback, `--no-browser`
- [ ] `courrier accounts` — list accounts, resolve and store the default
- [ ] `courrier sync` — account and per-folder IMAP sync
- [ ] `courrier folders` — folder list with unread and total counts
- [ ] `courrier inbox` / `courrier list <folder-type>` — collapsed conversations
- [ ] `courrier read <thread-id>` — expand one conversation
- [ ] `courrier search <query>` — trigram search over subject, sender and body
- [ ] `courrier mark <ids...>` — read, unread, star, archive, delete
- [ ] `courrier send` — to/cc/subject/body/attachments
- [ ] `--json` on every command carrying data
- [ ] `integrations/SKILL.md` — agent registration
- [ ] GoReleaser, the install shim, and a catalog entry in `FacileStudio/facile`

## Deliberately not in v1

- **A TUI.** Deferred on purpose; the `internal/` rule above is what keeps it cheap later.
- **MCP.** Agent access over MCP belongs in Courrier's own API as a mounted route, not in
  this binary. Different decision, different repo.
- **Drafts, templates, spaces, contacts, attachment download, connection test.** The routes
  exist; none earns a v1 command. Add on demand.
- **A self-updater.** CLI-STANDARD §3.1: no new CLI ships one. `facile update` owns it.
- **A shared CLI kit in tronc.** Four copies of `ui.go` is the standard's explicit choice.

## Known instance behaviour worth remembering

- **Folder listings return collapsed conversations, not messages**, and the path segment is
  a folder *type* (`inbox`, `sent`, …), never a folder id. Rows carry `message_count`,
  `unread_count` and `email_ids`; expand one with the thread route.
- **A thread id is a Message-ID** and must be percent-encoded into the path.
- **Emails carry both `body_text` and `body_html`**, so a terminal client never needs an
  HTML renderer.
- **Courrier serves its dashboard from the same origin** via an SPA catch-all, so a mistyped
  path answers 200 with HTML. The client parses defensively for that reason.
- **`SSO_ONLY=true` unregisters the local-auth routes**, so `POST /api/auth/login` answers
  404 rather than 403. The login command reads a 404 there as "no password login here".
- **Rate limits**: sync 5/min, folder sync 10/min, send 10/min, bulk-action 30/min.
- **`COURRIER_TOKEN` is single-occupancy.** A dashboard API token is a *labelled* porte
  session and Courrier keeps at most one per user, so minting a second revokes the first.
  Two agents sharing one token will evict each other. `courrier login` is unaffected: the
  SSO exchange issues an *unlabelled* session, so every machine holds its own.
