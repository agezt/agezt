# Phase 2.1 — Channel factory (working plan, from live survey 2026-08-06)

Goal: `Manifest.New` factory field; channel section of cmd/agezt/main.go (~1,400 lines,
27 builders) becomes one loop. Closes 34-manifests-vs-27-wirings drift.

## Verified facts
- Manifest in kernel/channel/registry.go (14 fields incl. Phase-0 AddrEnv/AllowlistEnv/InboundEnv).
  Registered by ONE slice literal in plugins/builtinchannels (34 entries), RegisterAll() at main.go:1517.
- Builders' entire kernel surface = k.Bus() + makeChannelHandler(k) → Deps{Ctx, Bus, Handler, Get, Label} suffices.
  makeChannelHandler (main.go:2320) is stateless → build ONCE, pass in.
- 6 modern builders (telegram/slack/email/discord/matrix/whatsapp, `label` param dead — get closure carries it),
  19 legacy funcs / 21 sites (os.Getenv direct, need overlayEnv), 1 outlier buildPushChannels (13 push kinds).
- overlayEnv = process-global os.Setenv hack (main.go:4069), only caller buildAccountsLegacy (4101, has
  reflect typed-nil probe because legacy builders return concrete *T). Test: main_test.go:57 TestOverlayEnv.
- allInsts literal (main.go:1669, 27 groups) feeds: combineSinks→buildPulse, registerInstances→liveChannels
  (agt send / notify tool / briefing), SetLive/SetLiveInstances (UI badge). Omission = silent drift.
- 7 manifests with no chanInstance wiring: ntfy/pushover/gotify/pushbullet/rocketchat/zulip/synology
  (built via buildPushChannels; no #label multi-account, no per-instance sink). That IS the 34-vs-27 gap.
- Dual-path kinds w/ suppression predicates: googlechat/mattermost/mastodon/line/feishu/dingtalk/wecom
  (two-way builder + push entry; twoWay*Configured name-ownership rules must be preserved).
- pulse.BriefSink: acyclic to import into kernel/channel but drags kernel/agent transitively —
  DECIDE in PR 1 (alt: local interface alias in channel).
- Tests pinning wiring: coverage_builders_test.go (all builders by name), TestOverlayEnv,
  coverage_helpers_more_test.go (startInstances/instanceSinks/registerInstances),
  config_inventory_test.go (regex-scans cmd/agezt only — REPOINT at new package in same PR, it
  never fails on extra so moving vars silently unguards them).

## Factory shape (deviation from doc's bare triple, justified)
```go
type Deps struct{ Ctx context.Context; Bus *bus.Bus; Handler InboundHandler; Get func(string) string; Label string }
type Built struct{ Channels []Channel; Sink pulse.BriefSink; Desc string } // Desc=="" → not configured
// Manifest: New func(Deps) Built `json:"-"`   (Manifest is JSON-serialized to UI)
```
Built (not triple) because push family returns N channels and Desc-sentinel kills the
reflect typed-nil landmine.

## PR train (each green/shippable)
1. Scaffolding: Deps/Built/Manifest.New + channel.BuildAll; move chanInstance/startInstances/
   instanceSinks/registerInstances/channelLabels/buildAccounts into kernel/channel (pure).
   Guard test: every manifest has New OR is on shrinking TODO allowlist (the drift alarm).
2. The 6 modern channels → New adapters; delete their buildAccounts sites; update coverage_builders_test.
3. Legacy batch A: webhook, irc, twitch, sms, signal (webhook flushes liveChannels special case main.go:2115).
4. Legacy batch B: whatsappgw, imessage, homeassistant, teams (+parseNamedWebhooks), nextcloudtalk.
5. Legacy batch C: nostr, zalo, qq, wechat (collapse buildOneBot), line (first suppression predicate).
6. Legacy batch D (suppression cluster together): googlechat, mattermost, dingtalk, feishu, wecom, mastodon —
   express ownership rule ONCE in manifest (Supersedes []string or IsLive ordering).
7. Push family (7 kinds get own New → gain #label + per-instance live keys; closes drift) + DELETE:
   overlayEnv+test, buildAccountsLegacy+reflect probe, allInsts literal, buildPushChannels,
   25 plugins/channels imports from main.go; repoint config_inventory_test.
8. Cleanup: notifyTargets derives from AllowlistEnv/RequiredEnv; consider deleting collectChannels (doc 0.7).
   Banner "disabled (set X)" hints: derive from RequiredEnv or add Manifest.DisabledHint.

## Final main.go section ≈ 25 lines
RegisterAll → handler := makeChannelHandler(k) → insts := channel.BuildAll(depsFor) →
loop: Start/liveChannels/sinks/banner → disabled lines from Manifests() → combineSinks → SetLive.
Downstream (channelTargets/channelSend/buildPulse/briefing) consumes liveChannels/channelSinks unchanged.
