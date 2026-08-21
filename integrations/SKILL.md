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
courrier inbox [--unread] [--limit 50]    Collapsed conversations, newest first
courrier list <folder-type> [filters]     inbox|sent|drafts|trash|junk|archive
courrier folders                          Folder list with unread and total counts
courrier read <thread-id>                 Every message in one conversation
courrier read --id <email-id>             One message by id
courrier search <query> [--limit 30]      Subject, sender and body
```

### Writing
```
courrier send --to a@b.c --subject "..." [--body "..." | --body-file -] [--cc x@y.z] [--attach path]
courrier mark <ids...> --read|--unread|--star|--unstar|--archive|--delete
courrier sync [--folder <id>]             Pull new mail from IMAP
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
- **Folder listings return collapsed conversations, not messages.** A row is a thread;
  `message_count` says how many messages it holds. Expand it with `courrier read <thread-id>`.
- **`courrier list` takes a folder *type*** (`inbox`, `sent`, …), never a folder id.
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
