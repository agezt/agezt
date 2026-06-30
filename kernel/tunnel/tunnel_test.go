// SPDX-License-Identifier: MIT

package tunnel

import (
	"context"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestBuildCommand(t *testing.T) {
	cases := []struct {
		name    string
		cfg     Config
		want    []string
		err     bool
		errText string
	}{
		{name: "cloudflared preset", cfg: Config{Provider: "cloudflared", TargetURL: "http://127.0.0.1:8787"},
			want: []string{"cloudflared", "tunnel", "--url", "http://127.0.0.1:8787"}},
		{name: "cloudflare alias", cfg: Config{Provider: "cloudflare", TargetURL: "http://127.0.0.1:8787"},
			want: []string{"cloudflared", "tunnel", "--url", "http://127.0.0.1:8787"}},
		{name: "ngrok preset strips scheme", cfg: Config{Provider: "ngrok", TargetURL: "http://127.0.0.1:8800/api"},
			want: []string{"ngrok", "http", "127.0.0.1:8800", "--log=stdout", "--log-format=logfmt"}},
		{name: "explicit command wins", cfg: Config{Command: []string{"mytunnel", "--port", "9000"}, Provider: "cloudflared"},
			want: []string{"mytunnel", "--port", "9000"}},
		{name: "custom provider requires explicit command", cfg: Config{Provider: "custom", TargetURL: "http://127.0.0.1:8787"}, err: true,
			errText: "AGEZT_TUNNEL=custom requires AGEZT_TUNNEL_CMD"},
		{name: "custom provider with explicit command", cfg: Config{Command: []string{"mytunnel", "--port", "9000"}, Provider: "custom"},
			want: []string{"mytunnel", "--port", "9000"}},
		{name: "preset needs target", cfg: Config{Provider: "cloudflared"}, err: true},
		{name: "unknown provider", cfg: Config{Provider: "frpc", TargetURL: "http://x"}, err: true},
		{name: "empty provider, no command", cfg: Config{TargetURL: "http://x"}, err: true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := buildCommand(c.cfg)
			if c.err {
				if err == nil {
					t.Fatalf("want error, got cmd %v", got)
				}
				if c.errText != "" && !strings.Contains(err.Error(), c.errText) {
					t.Fatalf("error %q does not contain %q", err.Error(), c.errText)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("got %v, want %v", got, c.want)
			}
		})
	}
}

func TestExtractURL(t *testing.T) {
	cases := []struct {
		line, want string
	}{
		{"|  https://happy-tree-1234.trycloudflare.com  |", "https://happy-tree-1234.trycloudflare.com"},
		{`t=2026-06-08 lvl=info msg="started tunnel" url=https://abcd.ngrok-free.app`, "https://abcd.ngrok-free.app"},
		// Prefer the tunnel domain over a docs/banner link on the same line.
		{"visit https://developers.cloudflare.com/ or use https://x.trycloudflare.com now", "https://x.trycloudflare.com"},
		// Custom binary: no known domain → first https wins, trailing punctuation trimmed.
		{"forwarding to https://my.custom.example/path).", "https://my.custom.example/path"},
		{"no url here", ""},
		{"http://insecure.example only http is ignored", ""},
	}
	for _, c := range cases {
		if got := extractURL(c.line); got != c.want {
			t.Errorf("extractURL(%q) = %q, want %q", c.line, got, c.want)
		}
	}
}

func TestStripScheme(t *testing.T) {
	for in, want := range map[string]string{
		"http://127.0.0.1:8787":      "127.0.0.1:8787",
		"https://host:9000/path?q=1": "host:9000",
		"127.0.0.1:8800":             "127.0.0.1:8800",
	} {
		if got := stripScheme(in); got != want {
			t.Errorf("stripScheme(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNew_ErrorsWithoutConfig(t *testing.T) {
	if _, err := New(Config{}); err == nil {
		t.Error("New with empty config must error")
	}
	tu, err := New(Config{Provider: "cloudflared", TargetURL: "http://127.0.0.1:8787"})
	if err != nil {
		t.Fatal(err)
	}
	if len(tu.cmd) == 0 || tu.cmd[0] != "cloudflared" {
		t.Errorf("cmd = %v, want cloudflared command", tu.cmd)
	}
}

// The supervisor captures the URL the process advertises.
func TestStart_CapturesURL(t *testing.T) {
	tu := &Tunnel{cmd: []string{"fake"}, run: func(ctx context.Context, _ string, _ []string, onLine func(string)) error {
		onLine("starting…")
		onLine("|  https://demo.trycloudflare.com  |")
		<-ctx.Done() // stay "running" until shutdown
		return nil
	}}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { tu.Start(ctx); close(done) }()

	waitFor(t, func() bool { return tu.URL() == "https://demo.trycloudflare.com" })
	cancel()
	<-done
	if tu.URL() != "" {
		t.Errorf("URL should clear after shutdown, got %q", tu.URL())
	}
}

// The supervisor restarts the binary when it exits unexpectedly, and the backoff
// sleep is injected so the test doesn't actually wait.
func TestStart_RestartsOnExit(t *testing.T) {
	defer func() { afterFunc = time.After }()
	afterFunc = func(time.Duration) <-chan time.Time {
		ch := make(chan time.Time, 1)
		ch <- time.Time{} // fire immediately — no real sleep
		return ch
	}

	var calls int32
	ctx, cancel := context.WithCancel(context.Background())
	tu := &Tunnel{cmd: []string{"fake"}, run: func(ctx context.Context, _ string, _ []string, onLine func(string)) error {
		n := atomic.AddInt32(&calls, 1)
		if n < 3 {
			return nil // exit immediately → supervisor should restart
		}
		onLine("url=https://stable.ngrok-free.app")
		<-ctx.Done()
		return nil
	}}
	done := make(chan struct{})
	go func() { tu.Start(ctx); close(done) }()

	waitFor(t, func() bool { return atomic.LoadInt32(&calls) >= 3 && tu.URL() == "https://stable.ngrok-free.app" })
	cancel()
	<-done
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("condition not met within deadline")
}

// Guard against a data race on url across the supervisor goroutine and URL().
func TestURL_ConcurrentAccess(t *testing.T) {
	tu := &Tunnel{}
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 1000; i++ {
			tu.setURL("https://x")
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 1000; i++ {
			_ = tu.URL()
		}
	}()
	wg.Wait()
}
