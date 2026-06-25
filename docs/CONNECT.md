# Connecting things to Agezt

How to connect **LLM providers** and **messaging channels** — including
subscription sign-in (ChatGPT), OAuth ("Connect with Slack/Mastodon"), and
running **multiple accounts of the same channel** at once.

Everything here is operable from both the **Web UI** and the **`agt` CLI**, and
nothing is auto-selected: Agezt ships with **no default provider/model** — a
provider only goes live once you connect it.

- [Sign in with ChatGPT (subscription provider)](#sign-in-with-chatgpt)
- [API-key providers](#api-key-providers)
- [Messaging channels — multiple accounts + guided Connect](#messaging-channels)
- [Channel OAuth — "Connect with Slack / Mastodon"](#channel-oauth)
- [Two-way email (IMAP/POP)](#two-way-email)

---

## Sign in with ChatGPT

Use a **ChatGPT Plus/Pro subscription** as an LLM provider — **no API key**. This
is the same "Sign in with ChatGPT" that the Codex CLI offers, exposed as a
first-class Agezt provider (catalog id `chatgpt`, models `gpt-5-codex`, `gpt-5`,
`gpt-5-mini`).

> ⚠️ **Important — this is an unofficial path.** There is no public OpenAI product
> for "use my subscription via API". Agezt obtains it the way Codex does: it
> reuses the **Codex CLI's public OAuth client** and calls the **ChatGPT backend
> Responses API**. This backend is **undocumented and unsupported** — it may stop
> working at any time, and using it outside the official Codex clients **may
> violate OpenAI's Terms of Service and put your account at risk**. It only ever
> authenticates **your own** account. The Web UI and CLI both require you to
> acknowledge this before connecting. The fully-supported alternative is an
> [OpenAI API key](#api-key-providers).

### Connect from the Web UI

1. Open the console → **Models**.
2. In the **"Sign in with ChatGPT"** card, click **Sign in with ChatGPT** and
   accept the warning.
3. A browser tab opens at `auth.openai.com`. Sign in to ChatGPT and approve.
4. The card flips to **connected · `<your email>`**. Done — `chatgpt` is now a
   live provider.

Already use the Codex CLI on this machine? Click **Import from Codex CLI** instead
to reuse your existing `~/.codex/auth.json` login — no browser round-trip.

### Connect from the CLI

```bash
agt provider chatgpt login      # prints an authorize URL, then waits for you to approve
agt provider chatgpt import     # OR: reuse a local `codex login` (~/.codex/auth.json)
agt provider chatgpt status     # show connected account
agt provider chatgpt logout     # disconnect (clears stored tokens)
```

`login` prints a URL to open **in a browser on the daemon host** (see
[remote daemons](#remote-daemons-and-port-1455) below), then polls until the
sign-in completes.

### Using it

ChatGPT registers with auth mode **subscription**, which the governor *prefers*
over API-key providers. To actually route runs to it:

- Set it as the default provider/model:
  ```bash
  AGEZT_PROVIDER=chatgpt AGEZT_MODEL=gpt-5-codex agezt   # (or set in your config)
  ```
- …or leave your existing primary and route specific tasks/agents to a ChatGPT
  model via per-task **routing** / **fallback chains** (System → Routing in the
  UI) — a request naming `gpt-5-codex` is sent to the `chatgpt` provider.

### How it works (for the curious / for debugging)

- **Auth**: OAuth 2.0 **PKCE** against `auth.openai.com` using the Codex public
  client id. The redirect URI is fixed to `http://localhost:1455/auth/callback`,
  so the **daemon** runs a one-shot listener on `127.0.0.1:1455` during sign-in,
  captures the code, and exchanges it. (`kernel/chatgptauth`)
- **Storage**: the token set (access + refresh + id token + account id) is stored
  as **one encrypted secret in the vault** (`AGEZT_CHATGPT_OAUTH`, same
  AES-256-GCM at-rest as every other credential). The access token is refreshed
  **proactively** (before its JWT `exp`) and **reactively** (on a `401`).
- **Wire**: requests go to `https://chatgpt.com/backend-api/codex/responses` using
  the OpenAI **Responses API** (not chat/completions), with the `chatgpt-account-id`
  header and Codex's own system instructions (the backend validates them; the
  prompt is vendored under Apache-2.0). Agezt translates its chat-shaped requests
  and tool calls to/from Responses items and streams the reply.
  (`plugins/providers/openairesponses`)
- **Live application**: a successful sign-in triggers a kernel reload, so the
  provider appears **without restarting** the daemon.

### Troubleshooting

| Symptom | Cause / fix |
|---|---|
| `cannot bind 127.0.0.1:1455` | Something else holds the port (e.g. a running `codex login`). Close it and retry, or use `agt provider chatgpt import`. |
| Browser can't reach the redirect | The listener is on the **daemon** host. See [remote daemons](#remote-daemons-and-port-1455). |
| `not signed in` when routing to `gpt-5-codex` | Connect first (UI card / `agt provider chatgpt login`). The provider is only registered while signed in. |
| Worked before, now `400`/`403` from the backend | OpenAI changed the unofficial backend or required prompt. This is expected fragility; an update to `plugins/providers/openairesponses` may be needed. |
| `AGEZT_PROVIDER=chatgpt but not signed in` at boot | You pinned ChatGPT as primary but no tokens are stored — sign in, or unset `AGEZT_PROVIDER`. |

#### Remote daemons and port 1455

The OAuth redirect lands on `127.0.0.1:1455` **on the machine running `agezt`**.
For a local daemon this just works. For a remote daemon, either:

- run a browser on the daemon host, or
- forward the port to your laptop while signing in:
  `ssh -L 1455:127.0.0.1:1455 user@daemon-host`, then open the printed URL
  locally, or
- use **Import from Codex CLI** on the daemon host (`codex login` there first).

---

## API-key providers

The fully-supported path for any models.dev provider (Anthropic, OpenAI, Google,
Groq, DeepSeek, …). Keys live in the encrypted vault.

```bash
agt catalog sync                                   # pull the model catalog (offline-friendly)
agt provider setup                                 # what still needs a key? prompts on stdin
agt provider connect <id> --url <base> --model <m> --env <ENV> --key <k> [--default]
agt provider check --all                           # live roundtrip: creds + latency + cost
```

In the UI: **Models** lists every provider with a *keyed / no-key* badge; expand
one to manage **multiple keys** ("store many, pick active").

---

## Messaging channels

Agezt speaks ~27 channels (Telegram, WhatsApp, Slack, Discord, Matrix, Signal,
SMS, IRC, Twitch, email, the push family, regional platforms, …). Two things make
connecting them painless:

### Multiple accounts per channel

Every channel can run **several accounts at once** — 10 email mailboxes (each its
own SMTP + IMAP/POP), several Telegram bots, multiple Slack workspaces — all live
simultaneously.

- An account is a short **label**. Its settings are the normal `AGEZT_*` env names
  with a `#<label>` suffix: non-secret values in the config store, secrets in the
  vault — e.g. `AGEZT_EMAIL_SMTP_ADDR#work`, `AGEZT_EMAIL_PASSWORD#work`.
- The **unlabelled** keys are the "default" account, so existing single-account
  setups keep working **byte-for-byte**.
- There is **no "active" account** — every configured account runs. Send fan-out:
  a bare kind (`agt send --channel telegram`) goes to **all** Telegram accounts;
  `--channel telegram#bot2` targets exactly one.

### Guided Connect pages (Web UI)

**Channels** → pick a channel → **Add account**:

1. **"What you'll need"** — step-by-step help (where to get the token, which app
   to create) plus a docs link, pulled from the channel's manifest.
2. An optional **account name** (label) so you can add more than one.
3. The credential fields, then **Save**. Secrets go to the vault; restart (or it
   applies on the daemon's next start) to bring the account live.
4. **Send test** verifies a live account end-to-end.

Channels advertise a **connect method** so the page adapts: `token` (paste),
`qr` (WhatsApp gateway shows a QR to scan), `gateway` (needs an operator-run
bridge, e.g. Signal/QQ/WeChat/iMessage), or `oauth` (below).

---

## Channel OAuth

Some channels support **"Connect with X"** instead of hunting for a token —
currently **Slack** and **Mastodon**.

1. **Channels** → the channel → **Add account**.
2. In the **Connect with `<X>`** panel, paste the OAuth app's **client id +
   client secret** (and your **instance URL** for Mastodon). The page shows the
   **redirect URL** to register in your OAuth app.
3. Click **Connect with `<X>`**, authorize in the new tab; the resulting token is
   stored in the account's `#label` vault slot. Manual token entry stays available
   as a fallback below.

How it works: `channel/oauth/start` mints a state, builds the provider's
authorize URL, and the browser is redirected back to the **daemon's public
`/oauth/callback`**, which exchanges the code (over an SSRF-guarded client) and
writes the token. (Discord uses a static bot token and Google Chat a service
account, so those stay on token entry.)

---

## Two-way email

Email is **fully bidirectional**, per account:

- **Outbound**: SMTP (`AGEZT_EMAIL_SMTP_ADDR`, `AGEZT_EMAIL_FROM`, …).
- **Inbound**: set `AGEZT_EMAIL_INBOX_*` (protocol `imap` or `pop3`, server,
  username, password, TLS, poll interval). Agezt polls the mailbox, and any
  message from an **allowlisted** sender drives the agent; the answer is sent back
  by SMTP. POP3 with `starttls` upgrades the connection before sending credentials.
- All inbox fields are `#label`-aware, so each mailbox account has its own server.

Add several email accounts the same way as any channel (each with its own
SMTP + IMAP/POP), and they all run at once.

## Voice (speech in and out)

Agezt can hear voice and speak back. Two independent halves, each opt-in:

- **Speech-to-text (hearing)** — point an OpenAI-compatible transcription endpoint
  at the daemon. Inbound channel voice notes are auto-transcribed, the chat **mic
  button** dictates into the composer, and `agt transcribe <file>` / `agt listen`
  work from the CLI.
- **Text-to-speech (speaking)** — set `AGEZT_TTS_URL` + `AGEZT_TTS_MODEL` (and
  optionally `AGEZT_TTS_VOICE` / `AGEZT_TTS_KEY`). Agents can then speak replies,
  voice-in→voice-out keeps a conversation in voice, and the Web UI exposes a new
  `POST /api/tts` route the console uses.

Any OpenAI-compatible server works: `api.openai.com`, a gateway, or a **local**
model (faster-whisper / Kokoro / Piper behind an OpenAI shim) on loopback — the
adapter allows loopback and private addresses while netguard still blocks
link-local / cloud-metadata.

### Hands-free Voice mode (Web UI)

The console has a dedicated **Voice** page (under *Converse*) for a true
conversational loop — "talk to Jarvis":

- **Hands-free listening** with voice-activity detection (no push-to-talk); it
  waits for you to speak, then for a trailing silence that ends your turn.
- **Streaming speech** — the answer is spoken sentence-by-sentence *as it streams*,
  not after the whole reply lands.
- **Barge-in** — start talking while it's speaking and it stops instantly to listen.
- **Wake word** (optional toggle): say `agezt` / `jarvis` to start a turn.

Voice mode prefers the server `AGEZT_TTS_*` voice and the server transcription
endpoint for quality, and **degrades gracefully** to the browser's built-in
speech engines when neither is configured.

> **Env-name note:** the Web UI transcription route reads `AGEZT_STT_API_URL` /
> `AGEZT_STT_API_KEY` / `AGEZT_STT_MODEL`, while the agent-facing voice adapter
> (inbound notes, the `voice` tool, TTS) reads `AGEZT_STT_URL` / `AGEZT_STT_KEY`
> and `AGEZT_TTS_URL` / `AGEZT_TTS_MODEL` / `AGEZT_TTS_VOICE` / `AGEZT_TTS_KEY`.
> Set both STT spellings to the same endpoint for now if you want every surface
> hearing through the same backend.
