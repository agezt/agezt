// SPDX-License-Identifier: MIT

package redact

import (
	"encoding/json"
	"strings"
	"testing"
)

// The sink prefixes and their fake secret tails are kept as SEPARATE constants
// and joined at run time, never written as one literal. A complete
// hooks.slack.com URL in a source file — even an obviously invented one — matches
// GitHub's push-protection detector for "Slack Incoming Webhook URL" and blocks
// the push outright. Splitting them keeps the fixtures realistic enough to
// exercise the regexes without shipping a string that reads as a live credential
// to every scanner that walks this repo. (Same class as the gitleaks fixture
// allowlist in 1d2145bb.)
const (
	slackSink   = "https://hooks.slack.com/services/T00000000/B11111111"
	discordSink = "https://discord.com/api/webhooks/123456789012345678"
	teamsSink   = "https://contoso.webhook.office.com/webhookb2"

	fakeSlackSecret   = "NOTAREALSECRETxxxxxxxxxx"
	fakeDiscordSecret = "NOTAREALSECRET_-x9Y8z7W6v5"
	fakeTeamsSecret   = "NOTAREALSECRET"
)

// EXPOSE-003. For Slack, Discord and Teams incoming webhooks the URL PATH *is*
// the credential — there is no separate token, and possession of the URL is full
// authority to post as that integration. None of the token-shaped rules match a
// bare https:// URL, so a configured sink URL reached the journal and
// /api/webhook_log verbatim.
//
// Templated, not full-match: the whole point of the webhook log is "which sink is
// failing?", so the host and the non-secret identifiers have to survive.
func TestRedact_WebhookURLPathIsTheCredential(t *testing.T) {
	r := New()
	cases := []struct {
		name   string
		in     string
		secret string   // must NOT survive
		keep   []string // must survive, or the log answers nothing
	}{
		{
			name:   "slack",
			in:     slackSink + "/" + fakeSlackSecret,
			secret: fakeSlackSecret,
			keep:   []string{"https://hooks.slack.com/services"},
		},
		{
			name:   "discord",
			in:     discordSink + "/" + fakeDiscordSecret,
			secret: fakeDiscordSecret,
			// The numeric id identifies the sink and is not itself a credential.
			keep: []string{discordSink},
		},
		{
			name:   "discord legacy host",
			in:     "https://discordapp.com/api/webhooks/999/" + fakeDiscordSecret,
			secret: fakeDiscordSecret,
			keep:   []string{"discordapp.com/api/webhooks/999"},
		},
		{
			name:   "teams",
			in:     teamsSink + "/aaaa-bbbb@cccc-dddd/IncomingWebhook/" + fakeTeamsSecret + "/eeee-ffff",
			secret: fakeTeamsSecret,
			keep:   []string{teamsSink},
		},
	}
	for _, c := range cases {
		got := r.Redact(c.in)
		if strings.Contains(got, c.secret) {
			t.Errorf("%s: Redact(%q) = %q — still carries the credential %q", c.name, c.in, got, c.secret)
		}
		if !strings.Contains(got, Placeholder) {
			t.Errorf("%s: Redact(%q) = %q — expected %s", c.name, c.in, got, Placeholder)
		}
		for _, keep := range c.keep {
			if !strings.Contains(got, keep) {
				t.Errorf("%s: Redact(%q) = %q — dropped %q, so the webhook log can no longer say which sink failed",
					c.name, c.in, got, keep)
			}
		}
	}
}

// The rules are host-anchored, so ordinary URLs — including other paths on the
// same hosts — must be untouched. Over-redaction here would corrupt every
// journalled URL in the daemon.
func TestRedact_WebhookURLNoFalsePositives(t *testing.T) {
	r := New()
	for _, in := range []string{
		"https://api.slack.com/methods/chat.postMessage",
		"https://discord.com/channels/123/456",
		"https://example.com/api/webhooks/123/abc",
		"https://hooks.example.com/services/T0/B0/secret",
		"GET /webhookb2/thing HTTP/1.1",
	} {
		if got := r.Redact(in); got != in {
			t.Errorf("Redact(%q) = %q — must be left intact", in, got)
		}
	}
}

// The redactor reaches a webhook URL the way it actually appears in production:
// JSON-encoded inside an event payload, which is what bus.redactSpecLocked hands
// to RedactBytes before anything is journaled. This is the assertion that makes
// EXPOSE-003 closed rather than merely detectable — the finding was about
// /api/webhook_log and `agt journal`, both of which read what lands here.
func TestRedactBytes_WebhookURLInsideAnEventPayload(t *testing.T) {
	r := New()
	payload := []byte(`{"ok":false,"url":"` + slackSink + `/` + fakeSlackSecret + `","attempts":3}`)
	got := string(r.RedactBytes(payload))
	if strings.Contains(got, fakeSlackSecret) {
		t.Fatalf("payload = %s — the sink credential survived into the journal", got)
	}
	// The closing quote must bound the match — swallowing it would run the
	// redaction into the rest of the object and produce invalid JSON.
	if !strings.Contains(got, `/`+Placeholder+`","attempts":3`) {
		t.Errorf("payload = %s — the match crossed the closing quote", got)
	}
	for _, keep := range []string{`"ok":false`, `hooks.slack.com/services`} {
		if !strings.Contains(got, keep) {
			t.Errorf("payload = %s — expected to preserve %q", got, keep)
		}
	}
	if !json.Valid([]byte(got)) {
		t.Errorf("payload = %s — no longer valid JSON", got)
	}
}

func TestMatchedCategories_ReportsWebhookURLs(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{slackSink + "/" + fakeSlackSecret, "slack-webhook-url"},
		{discordSink + "/" + fakeDiscordSecret, "discord-webhook-url"},
		{teamsSink + "/a@b/IncomingWebhook/" + fakeTeamsSecret + "/d", "teams-webhook-url"},
	} {
		cats := MatchedCategories(c.in)
		found := false
		for _, got := range cats {
			if got == c.want {
				found = true
			}
		}
		if !found {
			t.Errorf("MatchedCategories(%q) = %v, want to include %q", c.in, cats, c.want)
		}
	}
}
