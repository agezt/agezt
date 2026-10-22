# email-tools recipes

The helper (`scripts/mail.py`) covers send/list/read with stdlib only. For OAuth2,
calendar invites, or rich multipart, use `smtplib`/`imaplib`/`email` directly.

## Common providers (use an app password)

| Provider | SMTP host / port | IMAP host / port |
|----------|------------------|------------------|
| Gmail    | smtp.gmail.com / 587 (STARTTLS) | imap.gmail.com / 993 |
| Outlook  | smtp.office365.com / 587 | outlook.office365.com / 993 |
| Yahoo    | smtp.mail.yahoo.com / 587 | imap.mail.yahoo.com / 993 |

Gmail/Outlook require an **app password** (2FA accounts) — generate one in account
settings and use it as `pass`.

## Send a report with an attachment

```sh
export APP_PW=...   # set the secret in the shell first
python scripts/mail.py '{"op":"send",
  "smtp":{"host":"smtp.gmail.com","port":587,"user":"me@gmail.com","pass":"'"$APP_PW"'"},
  "from":"me@gmail.com","to":["boss@corp.com"],"subject":"Weekly report",
  "body":"Numbers attached.","attachments":["report.pdf"]}'
```

## Notify on task completion

```sh
python scripts/mail.py '{"op":"send",
  "smtp":{"host":"smtp.gmail.com","port":587,"user":"me@gmail.com","pass":"'"$APP_PW"'"},
  "from":"me@gmail.com","to":["me@gmail.com"],"subject":"Job done","body":"The pipeline finished."}'
```

## Check for unread mail, then read one

```sh
python scripts/mail.py '{"op":"list",
  "imap":{"host":"imap.gmail.com","user":"me@gmail.com","pass":"'"$APP_PW"'"},
  "search":"UNSEEN","limit":10}'
# then, with a uid from the list:
python scripts/mail.py '{"op":"read",
  "imap":{"host":"imap.gmail.com","user":"me@gmail.com","pass":"'"$APP_PW"'"},"uid":"1234"}'
```

## IMAP search examples

`ALL`, `UNSEEN`, `FROM "alerts@x.com"`, `SUBJECT "invoice"`,
`SINCE 01-Jun-2026`. Combine: `(UNSEEN FROM "boss@corp.com")`.

## HTML email

```sh
python scripts/mail.py '{"op":"send", "smtp":{...}, "from":"...","to":["..."],
  "subject":"Styled","body":"plain fallback","html":"<h1>Hi</h1><p>Rich body.</p>"}'
```

## The reporting pipeline

email-tools is the delivery step: generate a chart with **data-analysis**, a PDF
with **pdf-tools**, or zip a folder with **archive-tools**, then attach and send
it here. Reference secrets via env vars; the helper never echoes the password.
