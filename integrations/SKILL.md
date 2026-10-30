---
name: courrier
description: >
  Facile self-hosted email CLI. Use when the user asks to read, search, triage
  or send email from a Courrier instance, or mentions their inbox, a thread, an
  unread count, or sending a mail from the terminal.
---

# courrier — Facile self-hosted email

Binary: `courrier`
Config: `${XDG_CONFIG_HOME:-~/.config}/courrier/config.yml` (instance URL, session token, default account)

Courrier connects to the user's own IMAP and SMTP servers and serves them over a
JSON API. This CLI reads, searches, triages and sends without the dashboard.

## When to apply

Use when the user mentions email, their inbox, a message or thread, an unread
count, a sender, or wants to send or reply to mail from the terminal.
Triggers: "email", "inbox", "mail", "unread", "thread", "send an email",
"reply", "who emailed", "search my mail", "courrier"

## Commands

### Setup
```
courrier login [url]            Authenticate (browser SSO, or password)
courrier logout                 Revoke the stored session
courrier accounts               List mail accounts; marks the default
```

### Reading
```
courrier triage [--all|--account N] [--unread] [--class known,unknown_human,bulk]
                [--since 2h] [--limit 50] [--page 1]    Classified conversations across accounts
courrier contacts [--all|--account N] [--limit 50]      Addresses the user has written to
courrier inbox [--unread] [--since 2h] [--limit 50]     Collapsed conversations, newest first
courrier list <folder-type> [--since 1d] [filters]      inbox|sent|drafts|trash|junk|archive
courrier folders                          Folder list with unread and total counts
courrier read <thread-id>                 Every message in one conversation
courrier read --id <email-id>             One message by id
courrier search <query> [--limit 30]      Subject, sender and body
```

### Writing
```
courrier send --to a@b.c --subject "..." [--body "..." | --body-file -] [--html "..." | --html-file path]
              [--cc x@y.z] [--references <id>] [--in-reply-to <id>] [--attach path] --yes
courrier reply <email-id> --body "..." [--all] [--html ...] [--to/--cc override] [--attach path] --yes
courrier forward <email-id> --to a@b.c [--body "note"] [--no-attachments] --yes
courrier draft create [--to --cc --subject --body|--body-file --html|--html-file --in-reply-to --references]
courrier draft update <id> [same flags; omitted fields are kept; the id changes]
courrier draft delete <id> --yes
courrier attachment <email-id> <attachment-id> [-o path|-] [--force]
courrier template list|show <id>|create|update <id>|delete <id> --yes   (--name --subject --body --html --space)
courrier contacts --query <text>          Autocomplete addresses on one account
courrier accounts check                   Probe IMAP and SMTP credentials
courrier mark <ids...> --read|--unread|--star|--unstar|--archive|--delete
courrier sync [--folder <id>]             Pull new mail from IMAP
```

### API keys
```
courrier keys list [--app <name>]         List API keys
courrier keys create --app <name> [flags] Create an API key (secret or public)
courrier keys revoke <id> [--yes]         Revoke an API key by id
```

### Global flags
```
--json           One JSON document on stdout, nothing else. Forces colour off
--account <id>   Act on a specific mail account
--url <url>      Override the stored instance
--no-color       Disable colour
```

## Rules

- **A session is required.** If none is stored the CLI says so; run `courrier login`
  once, or set `COURRIER_TOKEN` in a headless or CI context.
- **Use `--json` for anything you are going to parse.** Human output is a table and
  its columns are not a contract.
- **Prefer `courrier triage --all --unread --json` over looping `courrier inbox` per
  account.** One call covers every account and each row carries a `class`:
  `known` (a person the user has written to), `unknown_human` (personal, no history)
  or `bulk` (automated or marketing, with a `reason`). Skip `bulk` rows unless asked;
  `counts` follows `--unread`, `--since` and `--account` but not `--class`; `total` and
  `items` follow `--class`. "15 automated mails ignored" needs no paging, but check `truncated`: when true the server only looked at the newest 5000 conversations. `--all` is implied when no default account is set; with one, pass it.
- **`--since` takes `30m`, `2h`, `1d`, `1w` or an RFC 3339 time.** On `inbox` and `list`
  it filters on the returned `date` after the fetch, so a page can come back short and `total` counts only the rows kept on that page.
- **`triage` and `contacts` need a recent server.** On an older one they exit 1 with
  "this Courrier instance does not support triage (or contacts); upgrade the server". Fall back to
  `courrier inbox` per account.
- **Folder listings return collapsed conversations, not messages.** A row is a thread;
  `message_count` says how many messages it holds. Expand it with `courrier read <thread-id>`.
- **`courrier list` takes a folder *type*** (`inbox`, `sent`, …), never a folder id.
- **There is no Bcc, no move-to-folder and no folder management.** The instance exposes
  none of them. Archive and delete go through `mark`.
- **Drafts take no attachments and cannot be edited in place**; `draft update` re-creates
  the draft under a new id. Read drafts with `courrier list drafts` and `courrier read --id`.
- **Attachment ids** are in `courrier read --id <email-id> --json`, under `attachments`.
- **Ids for `mark` come from a listing.** `courrier inbox --json | jq '.emails[].id'`.
- **`mark` takes at most 200 ids** in one call; the instance refuses more.
- **Never send mail without showing the user the recipient, subject and body first**,
  and never invent a recipient address — read it from `courrier search` or from what
  the user gave you. A sent mail is not recallable.
- Rate limits worth pacing around: sync 5/min, folder sync 10/min, send 10/min,
  mark 30/min. A tight loop will earn a 429.
- Exit codes: 0 success, 1 failure, 2 usage, 130 interrupted.

## Environment

```
COURRIER_TOKEN        the credential; wins over the stored session
COURRIER_SERVER_URL   the instance; wins over the stored URL
COURRIER_ACCOUNT      the mail account id to act on
```

`COURRIER_TOKEN` holding a dashboard API token is single-occupancy: Courrier keeps
at most one named token per user, so minting a second revokes the first. Two agents
sharing one token will log each other out. A `courrier login` session does not have
this problem.
