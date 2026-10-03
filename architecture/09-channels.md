# 09 — Communication channels (`plugins/channels/*`, `plugins/builtinchannels`)

**Scope:** the 25 transport packages under `plugins/channels/` (which back 34 registered channel *kinds*),
the cross-channel invariant test at `plugins/channels/inbound_failclosed_test.go`, and the registration/factory
layer `plugins/builtinchannels`. The vocabulary they implement (`kernel/channel`) and the factory wiring
(`kernel/channelwire`) are owned by [07-autonomy-and-extensibility.md](07-autonomy-and-extensibility.md); they are
summarized here only as far as needed to explain the contract. Daemon boot order is in
[01-daemon-boot-cmd-agezt.md](01-daemon-boot-cmd-agezt.md); the Channels wizard / OAuth / account HTTP handlers are in
[03-control-plane-and-http.md](03-control-plane-and-http.md); the `notify` / `send_media` agent tools are in
[10-tools.md](10-tools.md); the console's Channels page is in [11-frontend-console.md](11-frontend-console.md).

---

## Responsibilities at a glance

- **Normalize** every platform's inbound message into one `channel.UnifiedMessage`, so the agent loop, the Unified
  Inbox and Pulse never see platform specifics.
- **Authenticate** inbound traffic (HMAC / Ed25519 / schnorr / shared secret / AES decrypt) and **fail closed** when
  the verifying secret is empty (API-001 invariant, enforced by `inbound_failclosed_test.go`).
- **Gate** who may drive the agent with a fail-closed `channel.Allowlist` (empty allowlist = outbound-only).
- **Hand** allowlisted messages to the single daemon-supplied `channel.InboundHandler` (built once in
  `cmd/agezt/main_channels_handler.go`), which folds conversation history, handles vision/STT/TTS and calls
  `Kernel.RunWith`.
- **Deliver** replies, Pulse briefs, alert notifications and `agt send` messages via `Channel.Send` (text, chunked to
  each platform's limit; media where the platform supports it).
- **Journal** every exchange as `channel.inbound` / `channel.outbound` / `channel.error` events on the bus (BLAKE3
  journal), keyed by a per-exchange correlation id `chan-<ULID>`.
- **Describe** themselves to the console (`channel.Manifest`: transport, duplex, media caps, required env, setup
  steps, connect method) and **build** themselves from env per account (`channelwire.Factory`, `ENV#label`
  multi-account model).

---

## 1. The contract (`kernel/channel`, `kernel/channelwire`)

### 1.1 `channel.Channel` (kernel/channel/channel.go:97)

```go
type Channel interface {
    Name() string                                   // channel kind ("telegram"); == Manifest.Kind
    Start(ctx context.Context) error                // listen until ctx is cancelled
    Send(ctx context.Context, out Outbound) error   // deliver one outbound message
}
type InboundHandler func(ctx context.Context, msg UnifiedMessage, corr string) (Reply, error)
```

De-facto invariants every implementation follows (not enforced by the type system):

| Rule | Detail |
|---|---|
| `Start` blocks for the daemon lifetime | Outbound-only channels (no `Addr`, push family, Teams, HA) just `<-ctx.Done()`. A clean cancel returns `nil`. |
| `Send` with empty/whitespace text and no attachments is a no-op | Avoids platform 400s (Telegram M236, Discord). |
| `Send` journals `channel.outbound` only after the platform accepted the message | Payload shape varies; see Gotchas §8.4. |
| Inbound correlation id is minted by the channel | `corr := "chan-" + ulid.New()`; passed to the handler so the agent run and the inbound/outbound events share one arc (`agt why`). |
| `channel.inbound` is emitted **before** the allowlist decision | Payload carries `allowed: bool`, so refused senders are still auditable. |
| Media is fetched **only for allowlisted senders** | Never dereference a file id/URL supplied by an unauthorized sender. |
| Handler error → user-visible text | `"sorry — that failed: " + err.Error()` is sent back as the reply. |

### 1.2 Message shapes

| Type | Fields | Notes |
|---|---|---|
| `UnifiedMessage` | `ChannelKind`, `ChannelID` (conversation/reply target), `ThreadID` (M885: Slack `thread_ts`, Telegram forum topic; empty = main stream), `Sender`, `Text`, `Images []string` (data: URLs, M247), `Audio []string` (data: URLs, auto-transcribed), `PlatformTSMS`, `PlatformMeta map[string]string` | JSON-tagged; mirrors `.project/agezt.proto`. |
| `Outbound` | `ChannelID`, `ThreadID`, `Text`, `Priority` (`info`/`notify`/`urgent`), `Attachments []Attachment` (json:"-") | `Priority` only changes behaviour in email (subject prefix). |
| `Attachment` | `Kind` (`image`/`audio`/`file`), `Data []byte` (json:"-"), `MIME`, `Filename` | Daemon resolves artifact refs to bytes first, so channel packages never import the artifact store. |
| `Reply` | `Text`, `Attachments` | Empty text + no attachments = "send nothing". |
| `Allowlist` | private `map[string]struct{}`; `NewAllowlist`, `Allows`, `Empty` | Exact, whitespace-trimmed, **case-sensitive** string match. Empty denies everyone. |

### 1.3 Shared helpers in `kernel/channel`

| Symbol | File | Purpose |
|---|---|---|
| `Guard(b, name, fn)` | guard.go | `recover()` wrapper; journals a panic as `channel.<name>.error` (`event.KindChannelError`). Used per message by telegram, slack, discord, matrix, signal, nextcloudtalk, nostr. |
| `SplitText(text, limit)` | split.go | Lossless split into ≤`limit` **UTF-16 code units**, preferring the last newline/space; concatenation reproduces input. Every chunking channel uses it. |
| `ConversationHistory(r, kind, channelID, threadID, sender, limit)` | history.go | Read-only journal fold producing a `user:`/`assistant:` transcript for the run intent; per-(kind, channel, thread) and **per-sender** (assistant turns included only when they share a correlation id with one of this sender's inbound events). Returns `""` when ≤1 turn. 2000 bytes per message cap. |
| `Manifest`, `MediaCaps`, `RegisterManifest`, `Manifests` (sorted by Display), `LookupManifest` | registry.go | Process-global manifest registry (RWMutex — VULN-002 fix: boot writes race control-plane reads). |
| `SetLive`/`IsLive`, `SetLiveInstances`/`IsLiveInstance`, `InstanceKey(kind,label)` | registry.go | Which kinds / `kind#label` instances actually started this process (Channels page "live" vs "restart to start"). |

### 1.4 `kernel/channelwire` (factory layer)

- `Deps{Ctx, Bus, Handler, Get func(baseEnv) string, Label}` — the **entire** kernel surface a factory may touch.
  `Get` = `settings.FieldGetter(label)` → `os.Getenv(base + "#" + label)` (or bare `base` for the default account).
- `Built{Channels []channel.Channel, Sink pulse.BriefSink, Desc string}`; `Desc == ""` (`NotConfigured`) = skip.
- `Register(kind, Factory)` / `Lookup(kind)` — RWMutex-guarded map.
- `Labels(kind)` — scans `os.Environ()` for `<base>#<label>` keys whose base belongs to the kind's Config Center
  section (`settings.SectionEnvs(kind)`), label regex `^[a-z0-9][a-z0-9_-]{0,31}$`.
- `BuildKind(ctx, kind, bus, handler) []Instance` — calls the factory for `""` + every label; keys each instance
  `kind` or `kind#label`; the first channel of a Built carries the Sink. (Lives in channelwire, not channel, because
  a factory returns a `pulse.BriefSink` and kernel/channel must not import pulse → agent.)

---

## 2. Boot, routing and reply flow

### 2.1 Boot wiring (cmd/agezt)

```
main.go:294   builtinchannels.RegisterAll()        // 34 manifests + 34 channelwire factories
main.go:309   notifyTargets  ← telegram/slack/discord manifests (RequiredEnv set AND AllowlistEnv non-empty)
              buildTools(..., notifyTargets)       // notify/send_media registered unbound
   ... kernel, control plane, HTTP servers start ...
main.go:1173  chanHandler := makeChannelHandler(k) // ONE shared InboundHandler
main.go:1187  for m := range channel.Manifests() { // sorted by Display name
                 insts := channelwire.BuildKind(ctx, m.Kind, k.Bus(), chanHandler)
                 startInstances(...)               // `go in.ch.Start(ctx)` + banner line per instance
              }                                    // or m.DisabledHint when nothing configured
main.go:1196  channelSinks := combineSinks(instance sinks...)  → buildPulse(..) + buildAlertNotify(..)
main.go:1334  registerInstances(liveChannels, ...) // map["telegram" | "email#work"]Channel
main.go:1344  channel.SetLive(base kinds); channel.SetLiveInstances(keys)
main.go:1360  channelSend / channelSendMedia       // target "email" fans out to all email instances,
              srv.Bind(LateDeps{ChannelSend})      // "email#work" hits exactly one
              toolSet.ConfigureLate(... ChannelSend, ChannelSendMedia ...)  // notify / send_media
```

Before boot, `main_pulse_artifacts.go` injects every non-empty config-store value and every `AGEZT_*` vault secret
into the process env (existing env wins) — that is how Config-Center-entered channel credentials (including
`#label` keys) become visible to `d.Get`.

### 2.2 Inbound → run → reply (per message)

```
platform ──(poll / webhook / socket)──▶ channel package
   1. read + bound body (1 MiB typical), verify signature/secret (fail closed if secret empty)
   2. parse → platform struct; drop bots/self/non-message events; dedup by message id (bounded FIFO ring)
   3. corr = "chan-"+ULID ; allowed = Allowlist.Allows(key)
   4. if allowed: fetch media → data: URLs into msg.Images / msg.Audio
   5. bus.Publish(channel.inbound.<kind>, KindChannelInbound, corr, {channel_kind, channel_id, sender, text, allowed[, thread_id]})
   6. if !allowed: (some kinds) reply "not authorized"; return
   7. rep, err := handler(ctx, msg, corr)  ─────────────▶ cmd/agezt makeChannelHandler:
                                                          a. intent = ConversationHistory(journal, kind, id, thread, sender, AGEZT_CHANNEL_HISTORY=10) or msg.Text
                                                          b. images: runtime.AdmitImages → confirmed vision | sidecar caption | correlated rejection; persist artifacts with caption even on rejection
                                                          c. audio: STT transcribe (k.Voice()), append transcript; persist audio artifacts
                                                          d. text, err := k.RunWith(ctx, corr, intent)   // governed agent run
                                                          e. voice-in → TTS voice-out attachment unless AGEZT_VOICE_REPLY=off
   8. reply via Send/internal send (chunked), ThreadID echoed where supported
   9. bus.Publish(channel.outbound.<kind>, KindChannelOutbound, [corr], {...})
```

### 2.3 Inbound execution model per transport family

| Model | Channels | Where the agent run executes | Consequence |
|---|---|---|---|
| Async ACK-then-run | slack, discord | `go channel.Guard(...)` on `baseCtx` (daemon ctx set in `Start`) after a 200/deferred response | Platform 3 s deadline met; run survives the HTTP request; cancelled on daemon shutdown. |
| Synchronous inside the HTTP handler | chatwebhook, dingtalk, feishu, imessage, line, nextcloudtalk, onebot, wecom, whatsapp, whatsappgw, zalo | `c.dispatch(r.Context(), m)` | Run is tied to the inbound request ctx; header write is buffered so the "prompt ACK" comments are aspirational (see §8.2). |
| Synchronous reply in the response body | sms (TwiML), webhook (JSON `{reply, correlation_id}`) | `c.handler(r.Context(), ...)` | By design; subject to the caller's HTTP timeout (Twilio ~15 s). |
| Sequential in the poll loop | telegram, matrix, signal, email, mastodon | Inline in the `Start` loop (Guard-wrapped except email/mastodon) | One message at a time per instance; a long run delays the next poll. |
| Sequential on the socket reader | irc/twitch, nostr | Inline on the read goroutine / relay pump | IRC: no PONG while a run is in progress (ping-timeout risk); nostr: that relay's writes stall. |

### 2.4 Outbound paths

| Caller | Path |
|---|---|
| Inbound reply | the channel's own `Send`/`send` (corr passed only by telegram, slack, discord, matrix, signal, mastodon, nextcloudtalk, nostr, sms, webhook, whatsapp) |
| Pulse briefs, M782 alert notifications | `Built.Sink` closures (`pulse.SinkFunc`) → `ch.Send(d.Ctx, Outbound{ChannelID: <each allowlisted id>, Text: "📣 "+Title+"\n"+Body, Priority: notify})`; combined into `pulse.MultiSink` |
| `agt send --channel <kind[#label]> --to <id>` | control plane `ChannelSend` → `channelTargets` → `Send` on each matching instance |
| `notify` / `send_media` agent tools | same `channelSend`/`channelSendMedia` closures; targets restricted to telegram/slack/discord **default** accounts with a non-empty allowlist |

---

## 3. Channel matrix (all 34 kinds)

Legend — Gate: what the factory requires before it builds anything (`d.Get`); Listener: what must additionally be set
for inbound. Allowlist key: what `Allows()` is checked against.

### 3.1 Two-way chat channels

| Kind (pkg) | Inbound transport | Outbound API | Gate → Listener | Inbound auth | Allowlist key (env) | Media in / out | Chunk |
|---|---|---|---|---|---|---|---|
| telegram (telegram) | long-poll `getUpdates` (25 s) | Bot API `sendMessage`, `sendPhoto/Voice/Audio/Document` (multipart) | `TELEGRAM_TOKEN` → always polls | bot token (in URL path; scrubbed from errors) | chat id (`TELEGRAM_CHAT_ID`) | photo+voice / image, voice(ogg)/audio, file | 4096 UTF-16 |
| slack (slack) | Events API webhook `POST /slack/events` | `chat.postMessage`; `files.getUploadURLExternal`→upload→`files.completeUploadExternal` | `SLACK_TOKEN` → `SLACK_ADDR` (+`SLACK_SIGNING_SECRET`) | HMAC-SHA256 `v0:ts:body`, 5-min window, retry header ignored, dedup `channel:ts` (4096) | channel id (`SLACK_CHANNELS`) | image+audio files (bot-token download) / files | 40000 |
| discord (discord) | Interactions webhook `POST /discord/interactions` (slash command; no Gateway WS) | REST `channels/{id}/messages` (Bot token); follow-up `webhooks/{app}/{token}` | `DISCORD_TOKEN` → `DISCORD_ADDR` + `DISCORD_PUBLIC_KEY` (+`DISCORD_APP_ID` for follow-ups) | Ed25519 over `ts‖body`, 5-min window | channel id (`DISCORD_CHANNELS`) | image+audio attachments (https Discord-CDN host only, H-001) / attachments **only on interaction follow-ups** | 2000 |
| matrix (matrix) | long-poll `/_matrix/client/v3/sync` (30 s, filtered to `m.room.message`, 20/room) | `PUT rooms/{id}/send/m.room.message/{txn}`; `/_matrix/media/v3/upload` | `MATRIX_HOMESERVER` + `MATRIX_TOKEN` → always | access token; own MXID (whoami) skipped | room id (`MATRIX_ROOMS`) | m.image/m.audio/m.video (mxc download) / m.image, m.audio, m.file | 32 KiB |
| signal (signal) | long-poll `GET /v1/receive/{number}` (10 s) on signal-cli-rest-api | `POST /v2/send` | `SIGNAL_API_URL` + `SIGNAL_NUMBER` → always | optional bearer `SIGNAL_TOKEN`; own number skipped | sender E.164 (`SIGNAL_RECIPIENTS`) | text only | 2000 |
| email (email) | IMAP (UNSEEN search, marks `\Seen`; go-imap v2) or POP3 (UIDL, hand-rolled) every `EMAIL_INBOX_POLL` (60 s) | SMTP `net/smtp.SendMail`, AUTH PLAIN | `EMAIL_SMTP_ADDR` → `EMAIL_INBOX_ADDR` | mailbox login only; **From header trusted** | sender address (`EMAIL_RECIPIENTS`, also outbound allowlist) | none / none | none (subject = first line ≤120) |
| whatsapp (whatsapp) | Meta webhook `GET` verify + `POST /whatsapp` | Graph v21.0 `/{phone}/messages`, `/{phone}/media` | `WHATSAPP_APP_SECRET` + `WHATSAPP_ACCESS_TOKEN` → `WHATSAPP_ADDR` | `X-Hub-Signature-256` HMAC (app secret); dedup msg id; `WHATSAPP_VERIFY_TOKEN` for GET | sender number (`WHATSAPP_NUMBERS`) | image+voice / image, audio, document | 4000 |
| whatsappgw (whatsappgw) | webhook `POST /whatsappgw` from WAHA or Evolution | WAHA `POST /api/sendText` (`X-Api-Key`) or Evolution `POST /message/sendText/{instance}` (`apikey`) | `WHATSAPPGW_URL` → `WHATSAPPGW_ADDR` | `X-Webhook-Secret` == `WHATSAPPGW_SECRET` | bare number (`WHATSAPPGW_NUMBERS`) | text only | 4000 |
| sms (sms) | Twilio webhook `POST /sms`, reply as TwiML | `POST /2010-04-01/Accounts/{SID}/Messages.json` (Basic) | `SMS_ACCOUNT_SID` + `SMS_AUTH_TOKEN` → `SMS_ADDR` | `X-Twilio-Signature` = b64 HMAC-SHA1(url+sorted params); `SMS_PUBLIC_URL` for tunnels; dedup `MessageSid` | sender number (`SMS_NUMBERS`) | text | 1500 |
| imessage (imessage) | webhook `POST /imessage` from BlueBubbles (Mac) | `/api/v1/message/text`, `/api/v1/message/attachment` (`?password=`) | `IMESSAGE_URL` → `IMESSAGE_ADDR` | `X-Webhook-Secret` == `IMESSAGE_SECRET` | handle address (`IMESSAGE_ADDRESSES`) | image+audio / attachments | 10000 |
| webhook (webhook) | `POST /webhook` signed JSON; reply in response body | `POST WEBHOOK_OUTBOUND_URL` (signed same way) | `WEBHOOK_SECRET` **or** `WEBHOOK_OUTBOUND_URL` → `WEBHOOK_ADDR` | `X-Agezt-Signature: sha256=` HMAC; optional `ts_ms` 5-min window; dedup `channel_id:id` | `channel_id` (`WEBHOOK_CHANNELS`) | caller-supplied `images` data URLs / none | none |
| irc (irc) | TCP/TLS socket (`IRC_TLS=true` or `:6697`), PRIVMSG | PRIVMSG lines | `IRC_SERVER` + `IRC_NICK` → always | optional `PASS` | `#channel` or nick of the reply target (`IRC_CHANNELS` ∪ `IRC_ALLOWLIST`) | text | 480 bytes/line, split on newlines |
| twitch (irc, `Kind:"twitch"`) | `irc.chat.twitch.tv:6697` TLS | PRIVMSG | `TWITCH_USERNAME` + `TWITCH_TOKEN` → always | `oauth:` token as PASS | `#channel` (`TWITCH_CHANNELS` ∪ `TWITCH_ALLOWLIST`) | text | as IRC |
| nostr (nostr) | WebSocket to each relay; `REQ` kinds 1,4 `#p`=own pubkey, `since`=now | signed kind-1 (NIP-10 threaded) / NIP-04 kind-4 DM, broadcast to all relays | `NOSTR_PRIVKEY` (hex or nsec) + `NOSTR_RELAYS` → always | BIP340 schnorr verify of every event; own pubkey skipped; dedup event id | author pubkey hex (`NOSTR_AUTHORS`, hex or npub, normalized) | text | 8000 (truncate) |
| mastodon (mastodon) | poll `/api/v1/notifications?types[]=mention` (`MASTODON_POLL`, 60 s), `since_id` cursor | `POST /api/v1/statuses` (self-threaded chunks) | `MASTODON_USERS` + `SERVER` + `TOKEN`; else push fallback | bearer token | acct (`MASTODON_USERS`) | text | 500 |
| nextcloudtalk (nextcloudtalk) | webhook `POST /nextcloudtalk` (Talk Bot API) | `POST /ocs/v2.php/apps/spreed/api/v1/bot/{token}/message` (signed) | `NEXTCLOUDTALK_URL` + `SECRET` → `NEXTCLOUDTALK_ADDR` | hex HMAC-SHA256(secret, random‖body); dedup `token:id`; replies only to configured URL (never `X-Nextcloud-Talk-Backend`) | conversation token (`NEXTCLOUDTALK_TOKENS`) | text | 32000 |
| zalo (zalo) | webhook `POST /zalo` (OA events, `user_send_text*`) | `POST /v3.0/oa/message` (`access_token` header) | **`ZALO_ADDR`** (nothing builds without it) | sha256(appId+body+ts+secret), ±5 min, dedup msg_id | user id (`ZALO_USERS`) | text | 2000 |
| qq / wechat (onebot, `Kind`) | webhook `POST /qq` or `/wechat` from a OneBot v11 gateway | gateway `POST /send_msg` (`private:`/`group:` target) | `<P>_ADDR` → always (listener) | `X-Signature: sha1=` HMAC-SHA1 (`<P>_SECRET`); optional bearer `<P>_TOKEN` | sender user_id (`<P>_USERS`) | CQ `image`/`record` URLs fetched via **netguard SSRF-guarded** client / none | 4000 |
| line (line; push fallback) | webhook `POST /line` | reply token `/v2/bot/message/reply`; push `/v2/bot/message/push`; content from `api-data.line.me` | `LINE_SECRET` (else push via `LINE_TOKEN`+`LINE_TO`) → `LINE_ADDR` | `X-Line-Signature` b64 HMAC-SHA256 | userId (`LINE_USERS`) | image+audio / none | 5000 × max 5 msgs |
| googlechat / mattermost (chatwebhook; push fallback) | webhook `POST /<kind>` (Google Chat app event JSON / Mattermost outgoing-webhook form) | incoming webhook URL (`<P>_WEBHOOK`; Mattermost `channel` override) | `<P>_ADDR` (else push via `<P>_WEBHOOK`) | Google: `?token=`; Mattermost: form `token`; constant-time vs `<P>_TOKEN` | Google sender email→displayName→name; Mattermost `user_name` (`<P>_USERS`) | text | 4000 |
| dingtalk (dingtalk; push fallback) | enterprise robot outgoing webhook `POST /dingtalk` | per-message `sessionWebhook` (only `https://*.dingtalk.com`, SSRF guard) else robot webhook | `DINGTALK_ADDR` (else push via `DINGTALK_WEBHOOK`) | `timestamp`+`sign` = b64 HMAC-SHA256(secret, ts\nsecret), ±5 min | senderStaffId→senderNick (`DINGTALK_USERS`) | text | 4000 |
| feishu (feishu; push fallback) | event subscription `POST /feishu` (`url_verification`, `im.message.receive_v1` schema 2.0) | IM API `/open-apis/im/v1/messages?receive_id_type=chat_id` with cached tenant token | `FEISHU_ADDR` + `FEISHU_APP_ID` (else push via `FEISHU_WEBHOOK`) | event `token` == `FEISHU_VERIFY_TOKEN` (no Encrypt-Key support) | open_id (`FEISHU_USERS`) | image+audio (resources API) / none | 4000 |
| wecom (wecom; push fallback) | app callback `GET` echostr + `POST` AES-256-CBC XML `/wecom` | `/cgi-bin/message/send` with cached `access_token` | `WECOM_ADDR` + `WECOM_CORP_ID` (else push via `WECOM_WEBHOOK`) | SHA-1 `msg_signature` (token) + WXBizMsgCrypt decrypt + receive_id == corp id | userid (`WECOM_USERS`) | image+voice (`media/get`) / none | 2000 |

### 3.2 Outbound-only channels

| Kind (pkg) | API | Gate (factory) | Destination selection | Notes |
|---|---|---|---|---|
| teams (teams) | Teams Incoming Webhook, `MessageCard` | `TEAMS_WEBHOOKS=name=url,...` | `ChannelID` = webhook **name**; unknown name refused | Brief sink fans out to every name. |
| homeassistant (homeassistant) | `POST {URL}/api/services/notify/{service}` Bearer | `HOMEASSISTANT_URL` + `TOKEN` | `ChannelID` = notify service, must be in `HOMEASSISTANT_SERVICES` | Driving the agent from HA = HA automation → generic webhook channel. |
| ntfy (push) | `POST {server}/{topic}` raw text, optional Bearer | `NTFY_TOPIC` (`NTFY_SERVER` default `https://ntfy.sh`) | fixed | |
| pushover (push) | `api.pushover.net/1/messages.json` form | `PUSHOVER_TOKEN` (+`PUSHOVER_USER` required by `push.New`) | fixed | |
| gotify (push) | `{server}/message?token=` JSON | `GOTIFY_TOKEN` (+`GOTIFY_SERVER`) | fixed | |
| pushbullet (push) | `api.pushbullet.com/v2/pushes` note, `Access-Token` | `PUSHBULLET_TOKEN` | fixed | |
| rocketchat (push) | incoming webhook `{"text"}` | `ROCKETCHAT_WEBHOOK` | fixed | |
| zulip (push) | `{server}/api/v1/messages` stream msg, Basic(email, key) | `ZULIP_APIKEY` (+SERVER, EMAIL, STREAM; TOPIC default `agezt`) | fixed stream/topic | |
| synology (push) | incoming webhook, form `payload={"text"}` | `SYNOLOGY_WEBHOOK` | fixed | |
| (fallbacks) googlechat, mattermost, dingtalk, feishu, wecom, mastodon, line | push family `KindX` | when the two-way gate is not met | fixed | The two-way package "wins the kind name" when configured (guarded by `TestDualKindFactories_TwoWayWinsOverPushFallback`). |

### 3.3 External dependencies

All channels are stdlib `net/http` + `crypto/*` except:

| Package | Third-party module | Why |
|---|---|---|
| email | `github.com/emersion/go-imap/v2` (+ `imapclient`) | IMAP inbound (POP3 is hand-rolled on `net/textproto`). |
| nostr | `github.com/coder/websocket`, `github.com/btcsuite/btcd/btcec/v2` (+ `schnorr`) | Relay WebSocket transport; BIP340 signing, NIP-04 ECDH. Documented exception to the stdlib-only convention. |

Operator-run external processes required: signal-cli-rest-api (signal), WAHA/Evolution (whatsappgw), BlueBubbles on a
Mac (imessage), a OneBot v11 gateway — go-cqhttp/NapCat/Lagrange or wcf/wechatbot (qq/wechat). Intra-repo kernel deps
are uniformly `kernel/bus`, `kernel/channel`, `kernel/event`, `kernel/ulid`; plus `kernel/netguard` (onebot) and
`internal/strutil` (nostr); push has no `ulid`.

---

## 4. `plugins/builtinchannels`

**Purpose.** The single place that describes every shipped channel (manifests) and binds each kind to a
`channelwire.Factory`. Kept out of the kernel (kernel never imports plugins) and out of the transport packages
(which stay transport-only). Imported only by `cmd/agezt` (`main.go:294`); it imports `internal/brand`,
`kernel/channel`, `kernel/channelwire`, `kernel/pulse` and all 25 transport packages.

**Key API.** `RegisterAll()` — idempotent; `channel.RegisterManifest` for each of the 34 `manifests`, then 34
`channelwire.Register(kind, factory)` calls. Everything else is unexported.

**Persistence / concurrency.** None of its own. Factories run on the boot goroutine; brief-sink closures capture
`d.Ctx` and call `Send` from Pulse/alert goroutines.

**Env.** All reads go through `d.Get(brand.EnvPrefix + "<NAME>")` (`brand.EnvPrefix` = `AGEZT_`), which resolves the
`#label` variant for labelled accounts. Full per-kind list in §3; extra env read by factories but **absent from the
Config Center schema** (so not settable from the UI, and invisible to label discovery):
`AGEZT_SLACK_API_BASE`, `AGEZT_DISCORD_API_BASE`, `AGEZT_SMS_PATH`, `AGEZT_SMS_PUBLIC_URL`, `AGEZT_WHATSAPP_PATH`,
`AGEZT_SIGNAL_POLL_SECS`, `AGEZT_WEBHOOK_PATH`, and every `AGEZT_QQ_*`, `AGEZT_WECHAT_*`, `AGEZT_ZALO_*`.

### 4.1 Factory conventions

1. Read the gate env; return `channelwire.NotConfigured` when unset.
2. `splitNonEmpty(comma list)` → `channel.NewAllowlist`.
3. Construct the transport with `Bus: d.Bus, Handler: d.Handler` (email passes the handler only when an inbox is
   set; teams/homeassistant/push get no handler).
4. Sink: a `pulse.SinkFunc` that sends `formatBrief(b)` (`"📣 " + Title [+ "\n" + Body]`) to every allowlisted id,
   returning the first error; `nil` sink when no destination is known.
5. `Desc`: the boot-banner text, including actionable hints ("NO allowlist (outbound-only; set …)").
6. Dual-kind factories (dingtalk, feishu, wecom, mastodon, line, googlechat, mattermost) fall back to
   `pushBuilt(d, push.Config{...})` when the two-way gate is unmet. `pushBuilt` swallows `push.New` validation
   errors as `NotConfigured` (silent).
7. Nostr is the only factory that logs a construction failure: a malformed private key makes `nostr.New` fail and
   the factory prints `agezt: nostr channel disabled: …` to stderr before returning `NotConfigured`.

### 4.2 Manifest metadata (Connect wizard)

Each `channel.Manifest` row drives the console Channels page via the control plane's `handleChannelList`
(`kernel/controlplane/channels.go`): display, description, `Transport`, `Duplex`, `Media` caps, `SetupSteps`,
`ConnectMethod` (`token` default / `oauth` for slack, mastodon — flows in `controlplane/channel_oauth.go` that write
`AGEZT_SLACK_TOKEN` / `AGEZT_MASTODON_TOKEN` / `qr` for whatsappgw / `gateway` for signal, qq, wechat, imessage),
`ConfigSection` (fields come from `kernel/settings/schema_builtin.go`), `RequiredEnv` (the "configured" predicate,
evaluated per `#label` account), and `AddrEnv`/`AllowlistEnv`/`InboundEnv` (feed `collectChannels()` →
`agt status`), `BannerLabel`/`DisabledHint` (boot banner). Account add/remove: `controlplane/channel_accounts.go`.

Transports declared: `long-poll` (telegram, matrix), `webhook` (most), `rest` (signal, homeassistant, push family,
mastodon, zulip, whatsappgw, imessage), `smtp` (email), `socket` (irc, twitch, nostr — not listed in the
`Manifest.Transport` doc comment).

### 4.3 Multi-account (`ENV#label`)

- Non-default accounts store values under `AGEZT_X#<label>` (non-secret in the config store, secret in the vault);
  the unlabelled key is the immortal default account.
- `channelwire.Labels(kind)` discovers labels from env keys whose base is in the kind's schema section; every kind is
  built once per label with `Get = FieldGetter(label)`. Instance key: `kind#label`.
- Live state per instance → `channel.SetLiveInstances`; `agt send --channel email` fans out to every email instance,
  `--channel email#work` targets one.
- **qq, wechat, zalo have no Config Center section**, so `Labels()` returns nil: they are default-account-only and
  configurable only via raw env.

### 4.4 File table

| File | What it does |
|---|---|
| `doc.go` | Package doc: single registration point for built-in channels. |
| `builtinchannels.go` | `RegisterAll()` + the 34-entry `manifests` slice (display, transport, duplex, required/addr/allowlist/inbound env, media caps, setup steps, connect method, banner text). |
| `factories.go` | `formatBrief`, `splitNonEmpty`; `buildTelegram`, `buildSlack`. |
| `factories_comms.go` | `buildEmail`, `buildDiscord`, `buildMatrix`, `buildWhatsApp`. |
| `factories_chat.go` | `buildIRC`, `buildTwitch` (IRC engine with `Kind:"twitch"`, fixed Twitch server, `oauth:` prefixing). |
| `factories_chat_sms.go` | `buildSMS`, `buildSignal` (`SIGNAL_POLL_SECS`). |
| `factories_chat_apps.go` | `buildWhatsAppGateway` (WAHA/Evolution), `buildIMessage` (BlueBubbles). |
| `factories_chat_collaboration.go` | `buildNextcloudTalk`, `buildHomeAssistant`, `buildTeams`, `buildLine` (push fallback). |
| `factories_bot.go` | `oneBotFactory(kind, prefix)` (qq/wechat), `buildZalo`, `buildNostr` (npub→hex normalization). |
| `factories_bot_platforms.go` | `buildDingTalk`, `buildFeishu`, `buildWeCom`, `buildMastodon` — each with push fallback. |
| `factories_bot_webhook.go` | `parseNamedWebhooks` (Teams `name=url`), `chatWebhookFactory(kind, prefix)` (googlechat/mattermost). |
| `factories_push.go` | `pushBuilt` helper; `buildNtfy`, `buildPushover`, `buildGotify`, `buildPushbullet`, `buildRocketChat`, `buildZulip`, `buildSynology`. |

Tests: `builtinchannels_test.go` (registry seeded, idempotent, every manifest has Transport + RequiredEnv),
`factories_push_test.go` (two-way wins over push fallback; pure push factories build), `factory_ratchet_test.go`
(`TestEveryManifestHasFactoryOrTODO` — the manifest↔factory drift alarm; `factoryTODO` is empty and must stay so).

---

## 5. Per-package codemaps (`plugins/channels/*`)

Common to every package unless stated: depends on `kernel/bus`, `kernel/channel`, `kernel/event`, `kernel/ulid`;
imported only by `plugins/builtinchannels` (and the root invariant test); no files under `AGEZT_HOME`; all state
(cursor, dedup ring, cached access tokens) is in memory and lost on restart; emits `channel.inbound.<kind>`,
`channel.outbound.<kind>` (`KindChannelInbound` / `KindChannelOutbound`), and `channel.<kind>.error` via `Guard`;
reads no env directly (config arrives via the factory). Webhook listeners use `http.Server` with
`ReadHeaderTimeout 10s / ReadTimeout 30s / IdleTimeout 60s` (slow-loris, M431), no `WriteTimeout`, 5 s graceful
shutdown, `io.LimitReader(body, 1 MiB)`. Dedup rings are mutex-guarded bounded FIFO sets (2048–4096).

### 5.1 telegram
Long-poll Bot API channel; the reference implementation. Types `Config{Token, BaseURL, HTTPClient, Allowlist, Bus,
Handler, PollTimeoutSecs}`, `Channel`, `DefaultBaseURL`. HTTP client timeout 60 s (> poll). Forum topics
(`is_topic_message`) become `ThreadID` (M885). Non-allowlisted chat gets "not authorized". `scrubToken` strips the
bot token from transport errors. Response caps: 8 MiB API, 12 MiB raw photo/voice.

| File | What it does |
|---|---|
| `doc.go` | Package doc (long-poll duplex, allowlist security model). |
| `telegram.go` | Config/Channel/New/Name, Bot API wire types (`tgUpdate`, `tgMessage`, photo/voice), `Start` poll loop with 2 s backoff and Guard per update, `dispatchable` (text, caption, photo or voice — M476), `scrubToken`. |
| `telegram_inbound.go` | `getUpdates` (offset cursor), `handleInbound` (normalize, thread id, allowlist, media fetch, handler, reply into topic). |
| `telegram_outbound.go` | `fetchPhotoDataURL` (getFile + download → data URL), `tgMediaType`, `Send`, `send` (SplitText 4096, `message_thread_id`, then attachments, then journal). |
| `telegram_send.go` | `sendAttachment` (sendVoice/sendAudio/sendPhoto/sendDocument multipart), `emitInbound`, `emitOutbound`. (File names are swapped relative to content.) |

Tests: chunking, media in/out, inbound image size caps, coverage supplements.

### 5.2 slack
Events API webhook channel; async ACK. `EventsPath="/slack/events"`, `DefaultBaseURL`, `Config{Token, SigningSecret,
Addr, BaseURL, …}`. Drops bot messages, subtypes, empty text; `X-Slack-Retry-Num` deliveries are ACKed but not
reprocessed. `ThreadID = thread_ts`; replies go into the thread. Exported `Handler()` for embedding.

| File | What it does |
|---|---|
| `doc.go` | Package doc (HMAC signing, async ACK pattern). |
| `slack.go` | Constants, Config/Channel, bounded `dedup`, New, Name, `Handler`, `Start` (sets `baseCtx`), `newHTTPServer`, `verify` (v0 HMAC + integer-seconds freshness). |
| `slack_inbound.go` | Envelope/event/file wire types, `handleEvents` (verify, url_verification echo, ACK, filters, dedup, `go Guard(process)`), `process` (normalize, file fetch, allowlist, handler, threaded reply). |
| `slack_send.go` | `sendFile` (3-step external upload), `postMessage` (checks app-level `ok`), `fetchFileDataURL` (12 MiB cap, bot-token auth), `slackTSMillis`, `Send`/`send` (SplitText 40000). |
| `slack_emit.go` | `emitInbound` / `emitOutbound` (with `thread_id`). |

Tests: fuzz, slow-loris, dedup, base-ctx lifetime, empty-message, chunk, media, image size.

### 5.3 discord
Interactions-only (slash command) channel. `InteractionsPath="/discord/interactions"`, `DefaultBaseURL=…/api/v10`.
PING → PONG; command → deferred response (type 5) then `go Guard(runAndFollowUp)` on `baseCtx`. Empty reply becomes
"(no output)". Non-allowlisted → ephemeral "not authorized".

| File | What it does |
|---|---|
| `doc.go` | Package doc (duplicates the comment in discord.go). |
| `discord.go` | Constants, Config/Channel (`attachURLOK` seam), New (hex Ed25519 key; invalid → fail closed), `Handler`, `Start`, `newHTTPServer`, `handleInteractions`, `handleCommand`. |
| `discord_interactions.go` | `discordInteraction` wire types; `senderID`, `text` (string options), `imageAttachments`/`audioAttachments` (option type 11 + resolved attachments). |
| `discord_outbound.go` | `validDiscordAttachmentURL` (H-001), `fetchAttachmentDataURL` (12 MiB), `runAndFollowUp`, `verify` (Ed25519 + window), `Send` (text only, 2000), `followUp`/`followUpMedia`, `do`, `writeJSON`, `ephemeral`, `emitInbound`/`emitOutbound`. |

Tests: fuzz, slow-loris, base-ctx, attachment URL validation, chunking, inbound image.

### 5.4 email
SMTP outbound + optional IMAP/POP3 inbound poller. `SendFunc` seam (`net/smtp.SendMail`). Inbox creds fall back to
SMTP creds. `InboxTLS`: `tls` (implicit, default) / `starttls` / `none`. Dedup by `Message-Id` and POP3 UIDL (2048).
Messages >1 MiB truncated. Reply = new mail via `Send` (allowlist re-checked; subject `Agezt [notify]: <first line>`).

| File | What it does |
|---|---|
| `email.go` | Package doc (stale: still says outbound-only), `SendFunc`, Config, Channel, New (PLAIN auth), Name, `Start`, `Send` (allowlist-checked), `buildMessage` (RFC 5322, CRLF normalize), `subjectFor` (CR/LF header-injection guard, M479), `emitOutbound`. |
| `inbound.go` | `startInbound` ticker loop, `prime` (POP3 UIDL snapshot; IMAP none), `poll`, `dispatchInbound`, `emitInbound`, `seenBefore`. |
| `inbound_imap.go` | `dialIMAP` (TLS/STARTTLS/insecure), `pollIMAP` (UID SEARCH unseen, fetch body, mark `\Seen`). |
| `inbound_pop3.go` | Minimal POP3 client (`popConn`, STLS upgrade before USER/PASS, UIDL, RETR), `pollPOP3`, plus shared RFC 5322 parsing `parseMail`, `extractText` (first text/plain part), `decodeBody` (base64 / quoted-printable). |

### 5.5 matrix
`/sync` long-poll channel. Resolves own MXID via whoami (Start returns an error if that fails), primes `next_batch`
to skip backlog. Non-allowlisted room gets "not authorized". 8 MiB sync cap. Transaction ids are ULIDs.

| File | What it does |
|---|---|
| `doc.go` | Package doc (CS API v3, media scope). |
| `matrix.go` | Constants (`syncFilter`), Config/Channel, wire types, `Start`, `dispatchable`, `resolveWhoami`, `prime`, `sync`, `handleInbound`. |
| `matrix_send.go` | `sendMedia` (upload + typed event), `getJSON`, `scrubToken`, `emitInbound`/`emitOutbound`, `Send`/`send` (32 KiB chunks), `fetchMXC` (media download → data URL). |

### 5.6 signal
signal-cli-rest-api long-poll. Queue is drained server-side (no cursor). `signalMinPollInterval` (1 s) prevents
busy-spin against servers ignoring `?timeout`. Text only (attachments explicitly deferred).

| File | What it does |
|---|---|
| `doc.go` | Package doc. |
| `signal.go` | Constants, Config/Channel, `envelope` type, `Start`, `dispatchable` (skip own number), `receive`, `handleInbound`, `Send`. |
| `signal_helpers.go` | `send` (`/v2/send`, 2000-char chunks), `getJSON` (8 MiB cap), `authorize` (bearer), `scrubToken`, `emitInbound`/`emitOutbound`. |

### 5.7 whatsapp (Meta Cloud API)
| File | What it does |
|---|---|
| `doc.go` | Package doc (verify handshake, signature, no sync reply). |
| `whatsapp.go` | Constants (`DefaultPath="/whatsapp"`, Graph v21.0), Config/Channel, New, Name, `Handler`, `Start`, `handleInbound` (POST verify → dispatch all → 200), `handleVerify` (GET hub challenge), `dispatch` (media fetch, handler, reply; failure published as `channel.error.whatsapp` but with `KindChannelOutbound`). |
| `whatsapp_helpers.go` | `fetchMediaDataURL` (media id → URL → bearer download, 16 MiB). |
| `whatsapp_outbound.go` | `verify` (X-Hub-Signature-256), `Send`/`send` (4000-char chunks + attachments), `sendMedia`, `uploadMedia` (multipart `/media`), `waWebhook` types + `messages()` (text/audio/image), emitters, `sign`, `dedup`. |

### 5.8 whatsappgw (WAHA / Evolution)
| File | What it does |
|---|---|
| `whatsappgw.go` | Package doc, constants (`BackendWAHA`/`BackendEvolution`, `DefaultSession="default"`), Config/Channel, New, `Start`, `Handler`, `handleInbound`, `verify` (X-Webhook-Secret), `dispatch`. |
| `whatsappgw_parse.go` | `parseWAHA`, `parseEvolution` (`messages.upsert`, skips `fromMe`), `bareNumber` (strip jid suffix), `wahaChatID`. |
| `whatsappgw_send.go` | `Send` (chunks), `sendOne` (backend-specific URL/body/key header), `emitInbound`, `seenBefore`. |

### 5.9 sms (Twilio)
| File | What it does |
|---|---|
| `doc.go` | Package doc (TwiML sync reply, signature scheme). |
| `sms.go` | Constants, Config/Channel, New, `Handler`, `Start`, `handleInbound` (bounded form parse, verify, dedup MessageSid, run, journal outbound, TwiML), `verify`, `signedURL` (PublicURL or reconstructed from Host/X-Forwarded-Proto). |
| `sms_helpers.go` | `twilioSignature`, `Send`/`sendOne` (Messages.json, 1500-char chunks), emitters, `writeTwiML`, `xmlEscape`, `dedup`. |

### 5.10 imessage (BlueBubbles)
| File | What it does |
|---|---|
| `doc.go` | Package doc. |
| `imessage.go` | Constants (`DefaultMethod="private-api"`), Config/Channel, New, `Start`, `Handler`, `handleInbound`, `verify`, `dispatch` (allowlist keyed by handle; attachments), `Send` (text chunks 10000 + attachments). |
| `imessage_emit.go` | `emitInbound`; `scrubURLError` (removes `?password=` from URL errors). |
| `imessage_helpers.go` | `fetchAttachmentData` (16 MiB), `seenBefore`, `parseWebhook` (`new-message` events), `chatGUID` (address → `iMessage;-;<addr>`). |
| `imessage_send.go` | `sendOne` (`/api/v1/message/text`), `sendAttachment` (multipart `/api/v1/message/attachment`). |

### 5.11 webhook (generic signed JSON)
Signature scheme matches the outbound dispatcher in `kernel/webhook` so the two compose. Non-allowlisted → 403 JSON.

| File | What it does |
|---|---|
| `webhook.go` | Package doc (wire format), constants, Config/Channel, New, `Handler`, `Start`, `newHTTPServer`, `inboundEnvelope`, `handleInbound` (verify, require channel_id+text, ts window, dedup, run, JSON reply). |
| `webhook_helpers.go` | `sign` (hex HMAC-SHA256), `writeJSON`, `dedup`. |
| `webhook_ops.go` | `verify`, `Send`/`send` (signed POST to OutboundURL with `ts_ms`), emitters. |

### 5.12 irc (also Twitch)
Reconnect with exponential backoff 1 s→30 s; JOIN after `001`; PING/PONG; mutex-guarded writes with 20 s deadline.

| File | What it does |
|---|---|
| `irc.go` | Package doc, Config (`Kind` override), Channel, New, `kind`/`Name`, `Start`, `session` (dial TLS/plain, register, read loop), `handleLine`, `handlePrivmsg` (DM replies to nick, channel msg replies to channel), `Send`, `writeLine` (480-byte clamp), `emitInbound`, `parseLine`, `splitPrivmsg`, `nickOf`, `splitLines`. |

### 5.13 nostr
One goroutine per relay (`relayLoop` → `serveRelay`: dial 15 s, read limit 1 MiB, single writer draining a 16-slot
`out` chan, reader goroutine feeding a 32-slot chan, 5 s reconnect gap). `broadcast` is non-blocking (full queue =
skip relay); publish errors only if zero relays accepted.

| File | What it does |
|---|---|
| `doc.go` | Package doc (dependency exception, signature trust model). |
| `nostr.go` | Constants, Config/Channel, `New` (hex or nsec key), `PubHex`, Name, `Start`, `relayLoop`, `serveRelay`, `handleFrame` (verify before trust), `dispatch` (kind-1 mention vs kind-4 DM decrypt, threaded/encrypted reply). |
| `nostr_event.go` | `nostrEvent`, NIP-01 `serialize`, `sign`, `verify`, `seenBefore`. |
| `nostr_publish.go` | `Send` (standalone kind-1), `publishKind1`, `publishKind4`, `signAndBroadcast`. |
| `nostr_helpers.go` | `truncate` (8000), `parseXOnly`, `broadcast`, `register`/`unregister`, emitters. |
| `nip04.go` | NIP-04 AES-256-CBC with ECDH x-coordinate key; PKCS#7 pad/unpad. |
| `nip19.go` | Bech32 decode of `npub`/`nsec` (`DecodePubkey` exported for the factory). |

### 5.14 mastodon
| File | What it does |
|---|---|
| `mastodon.go` | Package doc, Config/Channel, New, `Start` (prime cursor then ticker; blocks if no handler), `prime`, `poll` (oldest-first, string `since` cursor), `dispatch` (acct allowlist, `@acct` threaded reply, inherits visibility, default `unlisted`), `Send`, `postStatus` (500-char self-thread), `fetchMentions`, emitters, `stripHTML`. |

### 5.15 nextcloudtalk
| File | What it does |
|---|---|
| `nextcloudtalk.go` | Package doc (SSRF note), constants, Config/Channel, New, `Handler`, `Start`, `handleInbound` (verify, Guard(dispatch)), `dispatch`, `verify`, `sign`. |
| `nextcloudtalk_helpers.go` | `parseActivity` (Activity-Streams Create), `randomHex`, `sha256Sum`, `seenBefore`. |
| `nextcloudtalk_send.go` | `Send`, `send` (signed bot message, 32000-char chunks, `OCS-APIRequest`), emitters. |

### 5.16 onebot (qq, wechat)
| File | What it does |
|---|---|
| `doc.go` | Package doc (one engine, `Config.Kind` names it). |
| `onebot.go` | Constants, Config/Channel (`mediaClient = netguard.New().HTTPClient(30s)`), New, `Start`, `Handler`, `handleInbound`, `dispatch`, `Send`/`sendOne` (`/send_msg`, numeric ids as int64), `emitInbound`, `seenBefore`, `splitTarget`. |
| `onebot_event.go` | `validSignature` (sha1 HMAC), `parseEvent` (message events → `private:`/`group:` target), CQ-code regexes, `fetchMedia` (SSRF-guarded, size-capped), `extractCQMedia`, `cqUnescape`. |

### 5.17 line
| File | What it does |
|---|---|
| `line.go` | Package doc, constants, Config/Channel, New, `Start`, `Handler`, `handleInbound`, `dispatch` (reply token preferred, push fallback). |
| `line_helpers.go` | `emitInbound`, `seenBefore`, `textMessages` (≤5 × 5000), `fetchContent` (`api-data.line.me`), `validSignature`, `parseWebhook`. |
| `line_send.go` | `Send` (push API), `send` (bearer POST). |

### 5.18 chatwebhook (googlechat, mattermost)
| File | What it does |
|---|---|
| `doc.go` | Package doc (supersedes push entries when an Addr is set). |
| `chatwebhook.go` | Constants (`KindGoogleChat`, `KindMattermost`), Config/Channel, New (path `/<kind>`), `Start`, `Handler`, `handleInbound`, `verify` (per-dialect token), `dispatch`, `Send`/`sendOne` (incoming webhook, Mattermost `channel` override), `emitInbound`, `seenBefore`. |
| `chatwebhook_parsers.go` | `parseInbound`, `parseMattermost` (form, strips trigger word), `parseGoogleChat` (MESSAGE events only). |

### 5.19 dingtalk
| File | What it does |
|---|---|
| `doc.go` | Package doc (sessionWebhook reply model). |
| `dingtalk.go` | Constants, Config/Channel, New, `Start`, `Handler`, `handleInbound`, `dispatch` (reply to `sessionWebhook` iff `safeReplyURL`, else robot webhook), `Send` (robot webhook). |
| `dingtalk_helpers.go` | `post`, `emitInbound`, `seenBefore`, `validSign`, `safeReplyURL`, `parseInbound`. |

### 5.20 feishu
| File | What it does |
|---|---|
| `doc.go` | Package doc. |
| `feishu.go` | Constants, Config/Channel (tenant-token cache under `tmu`), New, `Start`, `Handler`, `handleInbound` (challenge + event), `validToken`, `dispatch`. |
| `feishu_wire.go` | `urlVerification`, `parseEvent` (`im.message.receive_v1`: text/image/audio). |
| `feishu_send.go` | `Send` (default chat fallback), `sendOne`, `fetchResource` (16 MiB), `tenantToken` (cached, expiry −60 s). |
| `feishu_helpers.go` | `emitInbound`, `seenBefore`. |

### 5.21 wecom
| File | What it does |
|---|---|
| `doc.go` | Package doc (WXBizMsgCrypt). |
| `wecom.go` | Constants, Config/Channel (access-token cache, decoded AES key), New, `Start`, `Handler`, `emitInbound`, `seenBefore`. |
| `wecom_crypto.go` | `signature` (sorted SHA-1), `decrypt` (AES-256-CBC, returns receive id), `pkcs7Unpad`, `parseMessage` (text/image/voice XML). |
| `wecom_message.go` | `handleInbound` (fail closed on empty token, GET echostr, POST decrypt + corp check), `dispatch`. |
| `wecom_outbound.go` | `Send` (2000-char chunks), `sendOne`, `fetchMedia`, `accessToken` (cached). |

### 5.22 zalo
| File | What it does |
|---|---|
| `zalo.go` | Package doc, constants (`DefaultPath="/zalo"`, `openapi.zalo.me`), Config/Channel, New, `Start`, `Handler`, `handleInbound`, `dispatch`, `Send`/`sendOne`, `emitInbound`, `seenBefore`, `freshTimestamp`, `validSignature`, `parseEvent`. |

### 5.23 push, teams, homeassistant (outbound-only)
| File | What it does |
|---|---|
| `push/push.go` | 14 provider kinds (`KindNtfy` … `KindSynology`), `Config`, `New` (per-kind validation, defaults ntfy server and zulip topic), Name = provider kind, blocking `Start`, `Send` (journals under subject `channel.<kind>`, Actor `<kind>`), `buildRequest` per provider. `out.ChannelID` is ignored. |
| `teams/teams.go` | Named-webhook map, `Names`, `Send` (MessageCard to the named URL; unknown name refused), `emitOutbound`. |
| `homeassistant/homeassistant.go` | `Send` to `notify/<service>` with allowlisted service names, `emitOutbound`. |

### 5.24 Root invariant test (`plugins/channels/inbound_failclosed_test.go`, package `channels_test`)

API-001 guard over the 15 HTTP inbound listeners (chatwebhook, dingtalk, discord, feishu, imessage, line,
nextcloudtalk, onebot, slack, sms, webhook, wecom, whatsapp, whatsappgw, zalo): empty secret ⇒ 401/403 even for a
request signed with the empty secret; configured secret ⇒ unsigned request rejected; valid signature ⇒ accepted.
`TestTableCoversEveryInboundChannel` pins the count at 15 — a new listener must be added to `cases()`.

---

## 6. Events

| Subject | Kind | Emitted by | Payload |
|---|---|---|---|
| `channel.inbound.<kind>` | `channel.inbound` | every duplex channel | `channel_kind, channel_id, sender, text, allowed[, thread_id]`; `CorrelationID = chan-<ULID>` |
| `channel.outbound.<kind>` | `channel.outbound` | all but push | varies (see §8.4) |
| `channel.<kind>` | `channel.outbound` | push family | `channel_kind, channel_id, text` |
| `channel.<name>.error` | `channel.error` | `channel.Guard` on panic | `channel, panic` |
| `channel.error.whatsapp` | `channel.outbound` (sic) | whatsapp reply failure | `error, channel_id` |

Consumers: `kernel/channel.ConversationHistory` (run intent), `kernel/controlplane/inbox.go` (Unified Inbox: groups by
correlation, filters by `channel_kind`), journal readers (`agt why`, `agt journal`).

---

## 7. How to add a new channel

1. **Transport package** `plugins/channels/<kind>/`: implement `Name/Start/Send`; take `Config{..., Allowlist, Bus,
   Handler, Addr, Path, HTTPClient}`; follow §1.1 rules (mint `chan-<ULID>`, emit inbound with `allowed` before
   gating, fetch media only when allowed, `channel.SplitText` for chunking, journal outbound with `channel_kind` and
   the inbound corr, Guard per message on long-lived goroutines, bounded bodies/responses, slow-loris server
   timeouts, `Start` blocks when outbound-only). Import only `kernel/bus`, `kernel/channel`, `kernel/event`,
   `kernel/ulid` (+ `kernel/netguard` for any URL supplied by inbound data).
2. **Inbound auth fails closed**: reject when the configured secret is empty (the factory gates the listener on the
   ADDR, not on the secret). Add the listener to `cases()` in `plugins/channels/inbound_failclosed_test.go` and bump
   `wantInboundChannels`.
3. **Config Center section** in `kernel/settings/schema_builtin.go` with every env the factory reads (secrets via
   `pw(...)`/`Secret: true`). Without it the UI shows no fields and `#label` accounts are never discovered.
4. **Manifest** entry in `plugins/builtinchannels/builtinchannels.go` (`Kind` == `Name()`, `ConfigSection` == section
   id, `RequiredEnv` matching the factory's real gate, `AddrEnv`/`AllowlistEnv`/`InboundEnv`, `Media`,
   `SetupSteps`, `ConnectMethod`, `BannerLabel`, `DisabledHint`).
5. **Factory** in the matching `factories_*.go`: read only through `d.Get(brand.EnvPrefix+...)`, return
   `channelwire.NotConfigured` when unset, build a brief sink, return a descriptive `Desc`; register it in
   `RegisterAll()`. `TestEveryManifestHasFactoryOrTODO` fails if you forget either half.
6. If it should be an OAuth connect, add a provider to `oauthProviders` in `kernel/controlplane/channel_oauth.go`.
7. If the agent's `notify`/`send_media` tools should target it, extend the kind list at `cmd/agezt/main.go:310`.
8. Help/console: the Channels page is data-driven from the manifest + schema; no frontend change is needed for a
   standard token channel.
9. Tests: signature accept/reject, allowlist refusal, dedup, chunking, outbound-only `Start` blocking.

---

## 8. Gotchas / invariants

### 8.1 Security
- **Email inbound trusts the `From:` header** (no DKIM/SPF/ARC check) — a spoofed From of an allowlisted address
  drives a full agent run. Allowlists are also case-sensitive (`Alice@x.com` ≠ `alice@x.com`).
- **IRC/Twitch allowlist the reply target**, which is the `#channel` for channel messages — every user present in an
  allowlisted channel (every Twitch viewer of `TWITCH_CHANNELS`, which are auto-allowlisted) can trigger billable
  runs. Slack/Discord/Matrix/Nextcloud Talk likewise allowlist the channel/room, not the user; conversation history
  is still isolated per sender.
- IMAP inbound does not skip the backlog: the first poll processes **all existing UNSEEN mail** (only allowlisted
  senders drive runs, but all fetched mail is marked `\Seen`).
- Replay windows exist for slack, discord, dingtalk, zalo, webhook (only if `ts_ms` supplied); whatsapp,
  whatsappgw, imessage, nextcloudtalk, onebot, line, sms, chatwebhook, feishu, wecom rely on the in-memory dedup ring
  only (cleared on restart).
- WeCom compares the SHA-1 signature with `!=` (not constant-time) and does not query-escape `corpid/corpsecret` in
  the token URL.
- Good patterns to copy: SSRF guards in dingtalk (`safeReplyURL`), discord (CDN host check), onebot (netguard
  client), nextcloudtalk (reply only to configured server); token scrubbing in telegram/matrix/signal/imessage.

### 8.2 Lifecycle / reliability
- **`Start` errors are discarded** — `startInstances` runs `go in.ch.Start(ctx)`. A port-bind failure, a bad Matrix
  token (whoami error) or similar kills the channel silently while `SetLive` still reports it live and the banner
  printed its `Desc`.
- **Panics**: irc, email and mastodon poll/read loops are not `Guard`-wrapped, so a handler panic there runs on a bare
  goroutine and terminates the daemon. HTTP-handler channels are protected by `net/http`'s per-request recover only
  (no `channel.error` journal entry).
- **"ACK promptly" comments are wrong** for chatwebhook, dingtalk, feishu, imessage, line, onebot, wecom, whatsappgw,
  zalo: `WriteHeader(200)` is buffered until the handler returns, and the agent run executes inside the handler on
  `r.Context()`. WhatsApp writes 200 only after all runs finish. Platforms with short webhook deadlines will time out
  and retry (dedup absorbs the retry; the run may be cancelled if the client disconnects). Only slack and discord are
  truly async.
- SMS replies synchronously via TwiML — long runs exceed Twilio's webhook timeout and the reply is lost.
- IRC handles PRIVMSG on the read goroutine, so no PONG is sent during a long run (server ping timeout →
  reconnect). `writeLine` truncates at 480 bytes without rune awareness.
- Telegram's `getUpdates` offset is in memory; Matrix/Mastodon cursors too (both re-prime to "now" on restart, so
  messages that arrived while down are skipped).
- LINE silently drops text beyond 5 × 5000 chars; nostr truncates at 8000; mastodon threads 500-char chunks.

### 8.3 Config drift between manifest, factory and schema
- `Manifest.RequiredEnv` (used by `agt status`, the Channels page "configured" flag and `notifyTargets`) does not
  match the factory gate for: whatsapp (manifest `ACCESS_TOKEN+PHONE_NUMBER_ID`, factory `APP_SECRET+ACCESS_TOKEN`),
  email (manifest also needs `FROM`), pushover/gotify (factory gates on token only, `push.New` then silently rejects),
  zulip, mastodon, line, googlechat/mattermost/dingtalk/feishu/wecom (two-way gate is ADDR, manifest lists only
  WEBHOOK), qq/wechat (factory gates on `_ADDR` only), zalo (manifest `ZALO_TOKEN`, factory requires `ZALO_ADDR`),
  webhook (special-cased in `collectChannels`).
- qq, wechat, zalo have **no Config Center section** (not configurable from the console, no multi-account).
- Several factory env vars are not in the schema (§4).
- The `config-envvars guard` (controlplane `configEnvVars`) only covers env read in `cmd/agezt`; env read in
  `plugins/builtinchannels` is unguarded.

### 8.4 Journal / history consistency
`ConversationHistory` folds an assistant turn only if the outbound payload has `channel_kind` **and** shares the
inbound correlation id. Therefore multi-turn context (assistant side) works only for **telegram, slack, discord,
matrix, signal**:
- no `channel_kind` in outbound payload: email, sms, webhook, whatsapp, mastodon, nextcloudtalk, nostr (also journals
  `channel_id` = own pubkey), teams, homeassistant;
- reply journaled without the inbound corr (reply goes through public `Send`): chatwebhook, feishu, imessage, irc,
  onebot, wecom, whatsappgw, zalo, line push fallback;
- reply not journaled at all: dingtalk (sessionWebhook/robot reply path), line reply-token path;
- email, homeassistant, teams, mastodon `Send`, nostr `Send` and webhook `Send` mint a fresh `chan-` corr per send.
Mastodon uses the status id as `ChannelID`, so every mention is a new conversation. The Unified Inbox (grouped by
correlation) shows those replies as separate single-message threads, and filters out events without
`channel_kind`.

### 8.5 Media
- Discord `Send` ignores `Attachments`; media out only works for interaction follow-ups — `send_media` to Discord
  sends nothing if there is no text.
- Manifest `Media` caps are not verified against code: the generic webhook accepts `images` though its manifest
  declares none; feishu/wecom/line/qq/wechat are inbound-media only.
- Inbound media caps: 12 MiB raw (telegram, slack, discord), 16 MiB (whatsapp, feishu, imessage, line, wecom), sized
  so base64 data URLs fit the 16 MiB control-plane cap.

### 8.6 Misc
- `push` journals under subject `channel.<kind>` (not `channel.outbound.<kind>`) with Actor `<kind>`.
- `Manifest.Transport` doc lists four values; `socket` is also used.
- `discord` has two package doc comments (discord.go and doc.go); `email`'s package doc still says outbound-only;
  telegram's `telegram_send.go`/`telegram_outbound.go` names are inverted relative to their contents; `imessage`
  and `whatsappgw` Config comments call the secret "optional" though empty now fails closed.
- `channelwire.Instance` keys extras of a multi-channel `Built` by their own `Name()`; no current factory returns
  more than one channel.
