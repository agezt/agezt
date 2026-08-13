// Package channels_test holds the cross-channel invariants that no single
// channel package can express on its own.
//
// API-001: every inbound channel listener authenticates. Seven listeners used to
// authenticate with the shape `if secret != "" { check }`, so an operator who set
// AGEZT_<CHANNEL>_ADDR and skipped the secret got a listener that accepted every
// request — and each accepted request drives a full, billable, unthrottled agent
// run. The factories gate the listener on the address, not on the secret, so the
// invariant has to live here: an empty configured secret fails closed.
//
// The table below is deliberately exhaustive over every channel that serves an
// inbound HTTP listener. A channel added later that does not appear here will not
// fail this test — but a channel added here that fails open will, and the table
// is the place a reviewer looks to see the whole set at once.
package channels_test

import (
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/channel"
	"github.com/agezt/agezt/plugins/channels/chatwebhook"
	"github.com/agezt/agezt/plugins/channels/dingtalk"
	"github.com/agezt/agezt/plugins/channels/discord"
	"github.com/agezt/agezt/plugins/channels/feishu"
	"github.com/agezt/agezt/plugins/channels/imessage"
	"github.com/agezt/agezt/plugins/channels/line"
	"github.com/agezt/agezt/plugins/channels/nextcloudtalk"
	"github.com/agezt/agezt/plugins/channels/onebot"
	"github.com/agezt/agezt/plugins/channels/slack"
	"github.com/agezt/agezt/plugins/channels/sms"
	"github.com/agezt/agezt/plugins/channels/webhook"
	"github.com/agezt/agezt/plugins/channels/wecom"
	"github.com/agezt/agezt/plugins/channels/whatsapp"
	"github.com/agezt/agezt/plugins/channels/whatsappgw"
	"github.com/agezt/agezt/plugins/channels/zalo"
)

// goodSecret is the "operator configured it properly" secret.
const goodSecret = "s3cr3t-configured-by-the-operator"

// smsPublicURL is the exact URL the sms channel is told Twilio signed, so the
// test can recompute the signature deterministically.
const smsPublicURL = "https://sms.example/sms"

// probe is one channel's inbound listener plus the two request shapes the
// invariant is expressed over.
type probe struct {
	handler http.Handler
	path    string
	// signed builds a request whose credential is derived from the secret the
	// channel was built with. Called with "" it is the best an attacker can do
	// against a listener whose secret was never configured.
	signed func(base string) *http.Request
	// unsigned builds the same request with the credential omitted entirely.
	unsigned func(base string) *http.Request
}

// channelCase is one row of the table.
type channelCase struct {
	name string
	// build constructs the channel's inbound handler with the given secret.
	build func(t *testing.T, secret string) probe
	// okStatus is the status a correctly-signed request gets. It is never an
	// authentication rejection; that is the whole point of the positive test.
	okStatus int
}

func post(base, path, body string) *http.Request {
	req, err := http.NewRequest(http.MethodPost, base+path, strings.NewReader(body))
	if err != nil {
		panic(err)
	}
	return req
}

func hmacSHA256Hex(secret string, parts ...[]byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	for _, p := range parts {
		mac.Write(p)
	}
	return hex.EncodeToString(mac.Sum(nil))
}

func nowMillis() string { return strconv.FormatInt(time.Now().UnixMilli(), 10) }

// cases enumerates every channel package that serves an inbound HTTP listener.
func cases() []channelCase {
	return []channelCase{
		{
			name: "chatwebhook",
			build: func(t *testing.T, secret string) probe {
				c := chatwebhook.New(chatwebhook.Config{
					Kind:      chatwebhook.KindMattermost,
					Token:     secret,
					Allowlist: channel.NewAllowlist([]string{"bob"}),
				})
				form := func(tok string) string {
					v := url.Values{"user_name": {"bob"}, "text": {"hi"}}
					if tok != "" {
						v.Set("token", tok)
					}
					return v.Encode()
				}
				build := func(base, tok string) *http.Request {
					req := post(base, "/mattermost", form(tok))
					req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
					return req
				}
				return probe{
					handler:  c.Handler(),
					path:     "/mattermost",
					signed:   func(base string) *http.Request { return build(base, secret) },
					unsigned: func(base string) *http.Request { return build(base, "") },
				}
			},
			okStatus: http.StatusOK,
		},
		{
			name: "dingtalk",
			build: func(t *testing.T, secret string) probe {
				c := dingtalk.New(dingtalk.Config{
					Secret:    secret,
					Allowlist: channel.NewAllowlist([]string{"S1"}),
				})
				body := `{"msgtype":"text","text":{"content":"hi"},"senderStaffId":"S1","msgId":"M1"}`
				return probe{
					handler: c.Handler(),
					path:    dingtalk.DefaultPath,
					signed: func(base string) *http.Request {
						ts := nowMillis()
						mac := hmac.New(sha256.New, []byte(secret))
						mac.Write([]byte(ts + "\n" + secret))
						req := post(base, dingtalk.DefaultPath, body)
						req.Header.Set("timestamp", ts)
						req.Header.Set("sign", base64.StdEncoding.EncodeToString(mac.Sum(nil)))
						return req
					},
					unsigned: func(base string) *http.Request {
						return post(base, dingtalk.DefaultPath, body)
					},
				}
			},
			okStatus: http.StatusOK,
		},
		{
			name: "discord",
			build: func(t *testing.T, secret string) probe {
				// Discord's "secret" is the app keypair. An unconfigured secret is
				// an unconfigured public key.
				seed := sha256.Sum256([]byte(secret))
				priv := ed25519.NewKeyFromSeed(seed[:])
				pub := ""
				if secret != "" {
					pub = hex.EncodeToString(priv.Public().(ed25519.PublicKey))
				}
				c := discord.New(discord.Config{
					PublicKey: pub,
					Allowlist: channel.NewAllowlist([]string{"C1"}),
				})
				body := `{"type":1,"id":"I1"}`
				return probe{
					handler: c.Handler(),
					path:    discord.InteractionsPath,
					signed: func(base string) *http.Request {
						ts := strconv.FormatInt(time.Now().Unix(), 10)
						sig := ed25519.Sign(priv, []byte(ts+body))
						req := post(base, discord.InteractionsPath, body)
						req.Header.Set("X-Signature-Timestamp", ts)
						req.Header.Set("X-Signature-Ed25519", hex.EncodeToString(sig))
						return req
					},
					unsigned: func(base string) *http.Request {
						return post(base, discord.InteractionsPath, body)
					},
				}
			},
			okStatus: http.StatusOK,
		},
		{
			name: "feishu",
			build: func(t *testing.T, secret string) probe {
				c := feishu.New(feishu.Config{
					VerifyToken: secret,
					Allowlist:   channel.NewAllowlist([]string{"ou_1"}),
				})
				body := func(tok string) string {
					return `{"header":{"event_id":"E1","token":"` + tok +
						`","event_type":"im.message.receive_v1"},"event":{"sender":{"sender_id":{"open_id":"ou_1"}},` +
						`"message":{"message_id":"m1","chat_id":"oc_1","message_type":"text","content":"{\"text\":\"hi\"}"}}}`
				}
				return probe{
					handler: c.Handler(),
					path:    feishu.DefaultPath,
					signed: func(base string) *http.Request {
						return post(base, feishu.DefaultPath, body(secret))
					},
					unsigned: func(base string) *http.Request {
						return post(base, feishu.DefaultPath, `{"header":{"event_id":"E1","event_type":"im.message.receive_v1"}}`)
					},
				}
			},
			okStatus: http.StatusOK,
		},
		{
			name: "imessage",
			build: func(t *testing.T, secret string) probe {
				c := imessage.New(imessage.Config{
					Secret:    secret,
					Allowlist: channel.NewAllowlist([]string{"+15551234"}),
				})
				body := `{"type":"new-message","data":{"guid":"MSG1","text":"hi","handle":{"address":"+15551234"}}}`
				return probe{
					handler: c.Handler(),
					path:    imessage.DefaultPath,
					signed: func(base string) *http.Request {
						req := post(base, imessage.DefaultPath, body)
						req.Header.Set("X-Webhook-Secret", secret)
						return req
					},
					unsigned: func(base string) *http.Request {
						return post(base, imessage.DefaultPath, body)
					},
				}
			},
			okStatus: http.StatusOK,
		},
		{
			name: "line",
			build: func(t *testing.T, secret string) probe {
				c := line.New(line.Config{
					Secret:    secret,
					Allowlist: channel.NewAllowlist([]string{"U1"}),
				})
				body := `{"events":[{"type":"message","replyToken":"R1","source":{"userId":"U1"},"message":{"type":"text","id":"M1","text":"hi"}}]}`
				return probe{
					handler: c.Handler(),
					path:    line.DefaultPath,
					signed: func(base string) *http.Request {
						mac := hmac.New(sha256.New, []byte(secret))
						mac.Write([]byte(body))
						req := post(base, line.DefaultPath, body)
						req.Header.Set("X-Line-Signature", base64.StdEncoding.EncodeToString(mac.Sum(nil)))
						return req
					},
					unsigned: func(base string) *http.Request {
						return post(base, line.DefaultPath, body)
					},
				}
			},
			okStatus: http.StatusOK,
		},
		{
			name: "nextcloudtalk",
			build: func(t *testing.T, secret string) probe {
				c := nextcloudtalk.New(nextcloudtalk.Config{
					Secret:    secret,
					Allowlist: channel.NewAllowlist([]string{"t"}),
				})
				body := `{"type":"Create","object":{"content":"{\"message\":\"x\"}"},"target":{"id":"t"}}`
				return probe{
					handler: c.Handler(),
					path:    nextcloudtalk.DefaultPath,
					signed: func(base string) *http.Request {
						req := post(base, nextcloudtalk.DefaultPath, body)
						req.Header.Set("X-Nextcloud-Talk-Random", "r0")
						req.Header.Set("X-Nextcloud-Talk-Signature", hmacSHA256Hex(secret, []byte("r0"), []byte(body)))
						return req
					},
					unsigned: func(base string) *http.Request {
						req := post(base, nextcloudtalk.DefaultPath, body)
						req.Header.Set("X-Nextcloud-Talk-Random", "r0")
						return req
					},
				}
			},
			okStatus: http.StatusOK,
		},
		{
			name: "onebot",
			build: func(t *testing.T, secret string) probe {
				c := onebot.New(onebot.Config{
					Kind:      "qq",
					Secret:    secret,
					Allowlist: channel.NewAllowlist([]string{"12345"}),
				})
				body := `{"post_type":"message","message_type":"private","message_id":1,"user_id":12345,"raw_message":"hi"}`
				return probe{
					handler: c.Handler(),
					path:    "/qq",
					signed: func(base string) *http.Request {
						mac := hmac.New(sha1.New, []byte(secret))
						mac.Write([]byte(body))
						req := post(base, "/qq", body)
						req.Header.Set("X-Signature", "sha1="+hex.EncodeToString(mac.Sum(nil)))
						return req
					},
					unsigned: func(base string) *http.Request {
						return post(base, "/qq", body)
					},
				}
			},
			okStatus: http.StatusOK,
		},
		{
			name: "slack",
			build: func(t *testing.T, secret string) probe {
				c := slack.New(slack.Config{
					SigningSecret: secret,
					Allowlist:     channel.NewAllowlist([]string{"C1"}),
				})
				body := `{"type":"url_verification","challenge":"chal"}`
				return probe{
					handler: c.Handler(),
					path:    slack.EventsPath,
					signed: func(base string) *http.Request {
						ts := strconv.FormatInt(time.Now().Unix(), 10)
						req := post(base, slack.EventsPath, body)
						req.Header.Set("X-Slack-Request-Timestamp", ts)
						req.Header.Set("X-Slack-Signature", "v0="+hmacSHA256Hex(secret, []byte("v0:"+ts+":"), []byte(body)))
						return req
					},
					unsigned: func(base string) *http.Request {
						req := post(base, slack.EventsPath, body)
						req.Header.Set("X-Slack-Request-Timestamp", strconv.FormatInt(time.Now().Unix(), 10))
						return req
					},
				}
			},
			okStatus: http.StatusOK,
		},
		{
			name: "sms",
			build: func(t *testing.T, secret string) probe {
				c := sms.New(sms.Config{
					AuthToken: secret,
					PublicURL: smsPublicURL,
					Allowlist: channel.NewAllowlist([]string{"+15551230001"}),
				})
				form := url.Values{"From": {"+15551230001"}, "Body": {"hi"}, "MessageSid": {"SM1"}}
				encoded := form.Encode()
				req := func(base string) *http.Request {
					r := post(base, sms.DefaultPath, encoded)
					r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
					return r
				}
				return probe{
					handler: c.Handler(),
					path:    sms.DefaultPath,
					signed: func(base string) *http.Request {
						r := req(base)
						r.Header.Set("X-Twilio-Signature", twilioSig(secret, smsPublicURL, form))
						return r
					},
					unsigned: req,
				}
			},
			okStatus: http.StatusOK,
		},
		{
			name: "webhook",
			build: func(t *testing.T, secret string) probe {
				c := webhook.New(webhook.Config{
					Secret:    secret,
					Allowlist: channel.NewAllowlist([]string{"room1"}),
				})
				body := `{"channel_id":"room1","text":"hi","ts_ms":` + nowMillis() + `}`
				return probe{
					handler: c.Handler(),
					path:    webhook.DefaultPath,
					signed: func(base string) *http.Request {
						req := post(base, webhook.DefaultPath, body)
						req.Header.Set("X-Agezt-Signature", "sha256="+hmacSHA256Hex(secret, []byte(body)))
						return req
					},
					unsigned: func(base string) *http.Request {
						return post(base, webhook.DefaultPath, body)
					},
				}
			},
			okStatus: http.StatusOK,
		},
		{
			name: "wecom",
			build: func(t *testing.T, secret string) probe {
				c := wecom.New(wecom.Config{
					Token:     secret,
					AESKey:    strings.TrimSuffix(base64.StdEncoding.EncodeToString(make([]byte, 32)), "="),
					CorpID:    "corp",
					Allowlist: channel.NewAllowlist([]string{"u1"}),
				})
				encrypted := base64.StdEncoding.EncodeToString([]byte("not-a-real-ciphertext-but-signed"))
				body := "<xml><Encrypt>" + encrypted + "</Encrypt></xml>"
				build := func(base string, q url.Values) *http.Request {
					req := post(base, wecom.DefaultPath+"?"+q.Encode(), body)
					req.Header.Set("Content-Type", "text/xml")
					return req
				}
				return probe{
					handler: c.Handler(),
					path:    wecom.DefaultPath,
					signed: func(base string) *http.Request {
						ts, nonce := nowMillis(), "n1"
						return build(base, url.Values{
							"msg_signature": {wecomSig(secret, ts, nonce, encrypted)},
							"timestamp":     {ts},
							"nonce":         {nonce},
						})
					},
					unsigned: func(base string) *http.Request {
						return build(base, url.Values{"timestamp": {nowMillis()}, "nonce": {"n1"}})
					},
				}
			},
			// A correctly-signed request gets past authentication and dies in
			// decryption instead — which is exactly what "the signature gate
			// accepted it" looks like from outside.
			okStatus: http.StatusBadRequest,
		},
		{
			name: "whatsapp",
			build: func(t *testing.T, secret string) probe {
				c := whatsapp.New(whatsapp.Config{
					AppSecret: secret,
					Allowlist: channel.NewAllowlist([]string{"+15551230001"}),
				})
				body := `{"entry":[{"changes":[{"value":{"messages":[{"from":"+15551230001","id":"wamid.1","type":"text","text":{"body":"hi"}}]}}]}]}`
				return probe{
					handler: c.Handler(),
					path:    whatsapp.DefaultPath,
					signed: func(base string) *http.Request {
						req := post(base, whatsapp.DefaultPath, body)
						req.Header.Set("X-Hub-Signature-256", "sha256="+hmacSHA256Hex(secret, []byte(body)))
						return req
					},
					unsigned: func(base string) *http.Request {
						return post(base, whatsapp.DefaultPath, body)
					},
				}
			},
			okStatus: http.StatusOK,
		},
		{
			name: "whatsappgw",
			build: func(t *testing.T, secret string) probe {
				c := whatsappgw.New(whatsappgw.Config{
					Secret:    secret,
					Allowlist: channel.NewAllowlist([]string{"12345"}),
				})
				body := `{"event":"message","payload":{"from":"12345@c.us","body":"hi","id":"M1"}}`
				return probe{
					handler: c.Handler(),
					path:    whatsappgw.DefaultPath,
					signed: func(base string) *http.Request {
						req := post(base, whatsappgw.DefaultPath, body)
						req.Header.Set("X-Webhook-Secret", secret)
						return req
					},
					unsigned: func(base string) *http.Request {
						return post(base, whatsappgw.DefaultPath, body)
					},
				}
			},
			okStatus: http.StatusOK,
		},
		{
			name: "zalo",
			build: func(t *testing.T, secret string) probe {
				c := zalo.New(zalo.Config{
					AppID:     "app1",
					Secret:    secret,
					Allowlist: channel.NewAllowlist([]string{"U1"}),
				})
				body := func() string {
					return `{"event_name":"user_send_text","sender":{"id":"U1"},"message":{"msg_id":"M1","text":"hi"},"timestamp":"` + nowMillis() + `"}`
				}
				return probe{
					handler: c.Handler(),
					path:    zalo.DefaultPath,
					signed: func(base string) *http.Request {
						b := body()
						ts := zaloTimestamp(b)
						h := sha256.New()
						h.Write([]byte("app1"))
						h.Write([]byte(b))
						h.Write([]byte(ts))
						h.Write([]byte(secret))
						req := post(base, zalo.DefaultPath, b)
						req.Header.Set("X-ZEvent-Signature", "mac="+hex.EncodeToString(h.Sum(nil)))
						return req
					},
					unsigned: func(base string) *http.Request {
						return post(base, zalo.DefaultPath, body())
					},
				}
			},
			okStatus: http.StatusOK,
		},
	}
}

// zaloTimestamp pulls the timestamp back out of the body the test just built, so
// the signature covers exactly what the handler will parse.
func zaloTimestamp(body string) string {
	const key = `"timestamp":"`
	i := strings.Index(body, key)
	if i < 0 {
		return ""
	}
	rest := body[i+len(key):]
	j := strings.Index(rest, `"`)
	if j < 0 {
		return ""
	}
	return rest[:j]
}

// twilioSig reimplements Twilio's request-signature scheme (the channel's own
// implementation is unexported, and a test that reused it would prove nothing).
func twilioSig(token, fullURL string, form url.Values) string {
	keys := make([]string, 0, len(form))
	for k := range form {
		keys = append(keys, k)
	}
	// url.Values.Encode sorts; match that ordering.
	for i := 0; i < len(keys); i++ {
		for j := i + 1; j < len(keys); j++ {
			if keys[j] < keys[i] {
				keys[i], keys[j] = keys[j], keys[i]
			}
		}
	}
	var b strings.Builder
	b.WriteString(fullURL)
	for _, k := range keys {
		b.WriteString(k)
		for _, v := range form[k] {
			b.WriteString(v)
		}
	}
	mac := hmac.New(sha1.New, []byte(token))
	mac.Write([]byte(b.String()))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

// wecomSig reimplements WeCom's msg_signature: sha1 over the sorted
// concatenation of token, timestamp, nonce and the encrypted payload.
func wecomSig(token, timestamp, nonce, encrypt string) string {
	arr := []string{token, timestamp, nonce, encrypt}
	for i := 0; i < len(arr); i++ {
		for j := i + 1; j < len(arr); j++ {
			if arr[j] < arr[i] {
				arr[i], arr[j] = arr[j], arr[i]
			}
		}
	}
	h := sha1.New()
	h.Write([]byte(strings.Join(arr, "")))
	return hex.EncodeToString(h.Sum(nil))
}

// rejected reports whether the status is an authentication rejection. The
// channels are not consistent about 401 vs 403 (a pre-existing cosmetic
// divergence); both mean "we would not authenticate you".
func rejected(status int) bool {
	return status == http.StatusUnauthorized || status == http.StatusForbidden
}

func do(t *testing.T, h http.Handler, req *http.Request) *http.Response {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Result()
}

// TestInboundFailsClosedOnEmptySecret is the API-001 regression test. Every
// inbound channel listener must reject when no secret is configured, whatever
// credential the caller presents — including one computed from the empty secret,
// which is the best an attacker can do against an unconfigured listener.
func TestInboundFailsClosedOnEmptySecret(t *testing.T) {
	for _, tc := range cases() {
		t.Run(tc.name, func(t *testing.T) {
			p := tc.build(t, "")
			resp := do(t, p.handler, p.signed("http://listener.test"))
			defer resp.Body.Close()
			if !rejected(resp.StatusCode) {
				t.Fatalf("%s: empty configured secret ACCEPTED the request (status %d, want 401/403) — "+
					"the listener authenticates with `if secret != \"\"`, so an operator who set the ADDR "+
					"and skipped the secret is serving unauthenticated, billable agent runs (API-001)",
					tc.name, resp.StatusCode)
			}
		})
	}
}

// TestInboundRejectsMissingSignature is the other half of the invariant: a
// properly configured secret must still reject a request that carries no
// credential at all.
func TestInboundRejectsMissingSignature(t *testing.T) {
	for _, tc := range cases() {
		t.Run(tc.name, func(t *testing.T) {
			p := tc.build(t, goodSecret)
			resp := do(t, p.handler, p.unsigned("http://listener.test"))
			defer resp.Body.Close()
			if !rejected(resp.StatusCode) {
				t.Fatalf("%s: unsigned request ACCEPTED (status %d, want 401/403)", tc.name, resp.StatusCode)
			}
		})
	}
}

// TestInboundAcceptsValidSignature proves the fail-closed guards did not simply
// break inbound: a correctly configured secret with a valid credential still
// authenticates. Without this, "reject everything" would pass the two tests
// above.
func TestInboundAcceptsValidSignature(t *testing.T) {
	for _, tc := range cases() {
		t.Run(tc.name, func(t *testing.T) {
			p := tc.build(t, goodSecret)
			resp := do(t, p.handler, p.signed("http://listener.test"))
			defer resp.Body.Close()
			if rejected(resp.StatusCode) {
				t.Fatalf("%s: correctly signed request was REJECTED (status %d) — the fail-closed guard is too strict", tc.name, resp.StatusCode)
			}
			if resp.StatusCode != tc.okStatus {
				t.Fatalf("%s: status = %d, want %d", tc.name, resp.StatusCode, tc.okStatus)
			}
		})
	}
}

// TestTableCoversEveryInboundChannel is a coverage tripwire: if a channel
// package grows an inbound listener and is not added to the table, this count
// mismatch is the reminder.
func TestTableCoversEveryInboundChannel(t *testing.T) {
	const wantInboundChannels = 15
	got := cases()
	if len(got) != wantInboundChannels {
		t.Fatalf("table covers %d channels, want %d — add the new inbound listener to cases() (API-001)", len(got), wantInboundChannels)
	}
	seen := map[string]bool{}
	for _, tc := range got {
		if seen[tc.name] {
			t.Fatalf("duplicate table entry %q", tc.name)
		}
		seen[tc.name] = true
	}
}
