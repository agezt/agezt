// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/channel"
	"github.com/agezt/agezt/kernel/pulse"
)

type startRecordingChannel struct{ started chan struct{} }

func (c startRecordingChannel) Start(ctx context.Context) error {
	select {
	case c.started <- struct{}{}:
	default:
	}
	<-ctx.Done()
	return ctx.Err()
}

func (c startRecordingChannel) Send(context.Context, channel.Outbound) error { return nil }
func (c startRecordingChannel) Name() string                                 { return "recording" }

func TestCoverageHelperSinksAndInstances(t *testing.T) {
	if got := combineSinks(nil, nil); got != nil {
		t.Fatalf("combineSinks(nil,nil) = %#v, want nil", got)
	}
	var buf bytes.Buffer
	single := pulse.LogSink{W: &buf}
	if got := combineSinks(nil, single); got == nil {
		t.Fatal("combineSinks single non-nil returned nil")
	}
	if got := combineSinks(single, single); got == nil {
		t.Fatal("combineSinks multiple non-nil returned nil")
	}

	if got := instanceKey("email", "work"); got != "email#work" {
		t.Fatalf("instanceKey labelled = %q", got)
	}
	if got := instanceKey("email", ""); got != "email" {
		t.Fatalf("instanceKey default = %q", got)
	}

	started := make(chan struct{}, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	insts := []chanInstance{{key: "slack", desc: "ready", ch: startRecordingChannel{started: started}, sink: single}}
	buf.Reset()
	startInstances(ctx, &buf, "slack", "Slack", "disabled", insts)
	if out := buf.String(); !strings.Contains(out, "ready") || !strings.Contains(out, "default") {
		t.Fatalf("startInstances output = %q", out)
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("startInstances did not start channel")
	}

	buf.Reset()
	startInstances(ctx, &buf, "slack", "Slack", "disabled", nil)
	if out := buf.String(); !strings.Contains(out, "disabled") {
		t.Fatalf("disabled startInstances output = %q", out)
	}

	if sinks := instanceSinks(insts, []chanInstance{{key: "empty"}}); len(sinks) != 1 {
		t.Fatalf("instanceSinks length = %d, want 1", len(sinks))
	}
	live := map[string]channel.Channel{}
	registerInstances(live, insts)
	if live["slack"] == nil {
		t.Fatalf("registerInstances did not register slack: %#v", live)
	}
	if keys := liveChannelKeys(live); len(keys) != 1 || keys[0] != "slack" {
		t.Fatalf("liveChannelKeys = %#v", keys)
	}
}

func TestCoverageHelperWebAndBriefFormatting(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	if got := bannerColor("AGEZT", "32"); got != "AGEZT" {
		t.Fatalf("bannerColor with NO_COLOR = %q", got)
	}

	if got := effectiveWebPassword("127.0.0.1:8080"); got != defaultLoopbackWebPassword {
		t.Fatalf("loopback default password = %q", got)
	}
	t.Setenv(brand.EnvPrefix+"WEB_PASSWORD_DEFAULT", "off")
	if got := effectiveWebPassword("127.0.0.1:8080"); got != "" {
		t.Fatalf("disabled default password = %q", got)
	}
	t.Setenv(brand.EnvPrefix+"WEB_PASSWORD", "secret")
	if got := effectiveWebPassword("0.0.0.0:8080"); got != "secret" {
		t.Fatalf("explicit password = %q", got)
	}

	t.Setenv(brand.EnvPrefix+"WEB_OPEN", "false")
	if shouldOpenWebUI() {
		t.Fatal("shouldOpenWebUI should be false when disabled by env")
	}

	t.Setenv(brand.EnvPrefix+"WEB_ALLOWED_HOSTS", " ui.example.com, ,api.example.com ")
	gotHosts := webAllowedHosts("127.0.0.1:8080")
	wantHosts := []string{"127.0.0.1", "ui.example.com", "api.example.com"}
	if strings.Join(gotHosts, ",") != strings.Join(wantHosts, ",") {
		t.Fatalf("webAllowedHosts = %#v, want %#v", gotHosts, wantHosts)
	}

	var buf bytes.Buffer
	sink := briefSink(&buf, nil)
	if err := sink.Deliver(pulse.Brief{Title: "Heads up"}); err != nil {
		t.Fatalf("brief log sink deliver: %v", err)
	}
	if !strings.Contains(buf.String(), "Heads up") {
		t.Fatalf("briefSink log output = %q", buf.String())
	}
	if got := formatBrief(pulse.Brief{Title: "T", Body: "B"}); got != "📣 T\nB" {
		t.Fatalf("formatBrief with body = %q", got)
	}
	if got := formatBrief(pulse.Brief{Title: "T"}); got != "📣 T" {
		t.Fatalf("formatBrief without body = %q", got)
	}
}

func TestCoverageHelperTunnelAndRedaction(t *testing.T) {
	if got := tunnelTargetFromEnv(webUISurface{localURL: "http://127.0.0.1:9999"}); got != "http://127.0.0.1:9999" {
		t.Fatalf("tunnel target from web surface = %q", got)
	}
	t.Setenv(brand.EnvPrefix+"TUNNEL_TARGET", " http://localhost:7777 ")
	if got := tunnelTargetFromEnv(webUISurface{}); got != "http://localhost:7777" {
		t.Fatalf("explicit tunnel target = %q", got)
	}
	if !sameURLTarget("HTTP://LOCALHOST:7777/", "http://localhost:7777") {
		t.Fatal("sameURLTarget should ignore case and trailing slash")
	}
	if got := publicURLHost(" https://public.example/path "); got != "public.example" {
		t.Fatalf("publicURLHost = %q", got)
	}
	if got := publicURLHost("not a url"); got != "" {
		t.Fatalf("invalid publicURLHost = %q", got)
	}
	if got := tunnelPublicURL("https://public.example/app", webUISurface{token: "tok", passwordStrict: true}, true); !strings.Contains(got, "token=tok") {
		t.Fatalf("tunnelPublicURL token = %q", got)
	}
	if got := urlWithToken("not a url", "tok"); got != "not a url" {
		t.Fatalf("urlWithToken invalid = %q", got)
	}
	if got := addrToURL(":8080"); got != "http://127.0.0.1:8080" {
		t.Fatalf("addrToURL shorthand = %q", got)
	}

	t.Setenv(brand.EnvPrefix+"REDACT_EXTRA", " alpha ; ; beta ")
	if got := strings.Join(extraRedactLiterals(), ","); got != "alpha,beta" {
		t.Fatalf("extraRedactLiterals = %q", got)
	}
}
