# courrier-cli

Terminal client for [Courrier](https://github.com/FacileStudio/Courrier), the suite's
self-hosted email client. Go, cobra, one binary named `courrier`.

Lets a shell (or an AI agent) list, read, search, triage and send mail without opening
the dashboard. Authenticates through the same porte SSO flow the browser uses.

## What it does

- `courrier login` — browser SSO flow (one-time code via loopback) or password
- `courrier inbox` — collapsed conversations, newest first, with unread and star markers
- `courrier read` — every message in one thread, plain text, no HTML renderer needed
- `courrier search` — trigram search over subject, sender and body
- `courrier send` — to, cc, subject, plain or HTML body from a string, a file or stdin, references, attachments
- `courrier reply`, `courrier forward` — thread-aware reply (optionally to all) and forward with the original attachments
- `courrier draft` — create, update and delete drafts
- `courrier attachment` — download an attachment
- `courrier template` — list, show, create, update and delete templates
- `courrier mark` — read, unread, star, archive and delete, up to 200 messages at a time
- `courrier move`: move messages to any folder; `courrier folders create|rename|delete`
- `courrier keys` — list, create, and revoke API keys for applications
- `--json` on every command carrying data

## Stack

| Layer | Tech |
|---|---|
| CLI | Go 1.25, cobra, session token in a 0600 YAML config file |
| Auth | porte SSO CLI flow (`?flow=cli` → loopback → `/api/auth/oidc/exchange`), or password |
| Releases | GoReleaser via facile |

## Install

```sh
curl -fsSL https://raw.githubusercontent.com/FacileStudio/courrier-cli/main/install.sh | bash
```

Installs to `~/.local/bin` via [facile](https://github.com/FacileStudio/facile), the suite
installer.

Already have `facile`:

```sh
facile install courrier
```

## Setup

```sh
courrier login                    # opens the browser, SSO when the instance offers it
courrier login --no-browser       # print the URL instead
courrier login --email you@x.com  # password flow, prompts for the password
```

The URL defaults to `https://courrier.facile.studio`; `courrier login <url>` stores
another. `COURRIER_SERVER_URL` overrides the stored URL, `--url` overrides both.

If the instance has exactly one mail account, `login` records it as the default. With
several, pass `--account <id>` or set `COURRIER_ACCOUNT`; `courrier accounts` lists them.

## Usage

```sh
courrier accounts                             # mail accounts, default marked
courrier sync                                 # pull new mail from IMAP
courrier inbox --unread                       # unread conversations
courrier list archive --limit 20              # any folder type
courrier inbox --since 2h                     # mail from the last two hours
courrier triage --all --unread                # unread mail of every account, classified
courrier triage --class known,unknown_human   # skip automated mail
courrier contacts --all                       # addresses you have written to
courrier read '<CAJ8f=abc@mail.example.com>'  # a whole conversation
courrier read --id 4812                       # one message
courrier search "invoice" --limit 10
courrier send --to a@b.com --subject "Re: quote" --body-file draft.txt
courrier send --to a@b.com --subject "Hi" --html-file mail.html --body "Plain fallback"
courrier reply 4812 --body "Thanks" --all --yes
courrier forward 4812 --to c@d.com --body "FYI" --yes
courrier draft create --to a@b.com --subject "Later" --body "..."
courrier draft update 2089 --subject "Sooner" --attach quote.pdf
courrier move 4812 4813 --to archive
courrier folders create Clients
courrier draft delete 2089 --yes
courrier attachment 4812 77 -o invoice.pdf
courrier template create --name Intro --subject "Hello" --body "..."
courrier template update 3 --html-file intro.html
courrier template delete 3 --yes
courrier contacts --query ann --account 2
courrier accounts check --account 2
courrier mark 4812 4813 --read
courrier keys list
courrier keys create --app myapp
courrier keys revoke 1
courrier inbox --json | jq '.emails[].id'
courrier triage --all --unread --json | jq '.counts'
```

`triage` tags each conversation `known`, `unknown_human` or `bulk`, and its footer counts
follow `--unread`, `--since` and `--account` but not `--class`, and cover more than the
page; the total and the rows follow `--class`. Without `--account` and without a default
account it covers every account; with a default set, pass `--all`. `--since` takes `30m`,
`2h`, `1d`, `1w` or an RFC 3339 time; on `inbox` and `list` it filters on each row's
`date` after the fetch, and `total` then counts the rows kept on that page only.
`triage` and `contacts` need a recent server: an older one makes them exit 1 with
"this Courrier instance does not support triage; upgrade the server", or "contacts" for
the other.

`draft update` changes a draft in place: the id stays, fields you do not pass are kept, and
attachments stay unless dropped with `--remove-attachment <id>`. Drafts take `--attach` and
`--bcc` like `send`. `courrier move <id>... --to <type|folder-id|name>` moves messages, and
`courrier folders create|rename|delete` manage custom folders (system folders are refused, and a
folder holding messages needs `--force` to delete). These need a server released with Courrier
commit 0ef767e or later.

## Configuration

| Variable | What it does |
|---|---|
| `COURRIER_TOKEN` | Session or API token, overriding the stored one — the headless and CI path |
| `COURRIER_SERVER_URL` | Instance URL, overriding the stored one |
| `COURRIER_ACCOUNT` | Mail account id to act on |
| `NO_COLOR` | Disables colour, as does `--no-color` |

Config lives at `${XDG_CONFIG_HOME:-~/.config}/courrier/config.yml`, created `0600` in a
`0700` directory because it holds a bearer credential. `courrier logout` revokes the
session server-side and clears the token while keeping the instance URL.

## Rules of the road

- Data on stdout, status on stderr — a piped command emits only data.
- `--json` prints one document and forces colour off; `--no-color` disables it otherwise.
- Exit codes: 0 success, 1 failure, 2 usage, 130 SIGINT.
- Errors are lowercase, name what failed, and end with the fix after an em dash.
- A folder listing returns **collapsed conversations**, not messages, and takes a folder
  *type* (`inbox`, `sent`, …), never a folder id.

## Development

```sh
mise run build      # go build -o bin/courrier .
mise run check      # gofmt + vet + test
mise run format     # rewrite Go sources in place
```

## Documentation

- [ROADMAP.md](ROADMAP.md) — scope, what is deliberately out, and instance behaviour worth knowing
- [integrations/SKILL.md](integrations/SKILL.md) — the AI agent registration
- `CLI-STANDARD.md` — the suite CLI, installer and credential standard, normative for this repo.
  It lived in `FacileStudio/Wiki`, which was removed on 2026-08-22; the link is omitted until it
  has a permanent home again

---

Part of the [Facile Suite](https://facile.studio) — self-hosted tools for creative studios
and freelancers. One login, zero cloud dependency.
