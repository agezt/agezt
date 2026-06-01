// SPDX-License-Identifier: MIT

package main

import (
	"net"
	"testing"
)

func TestClassifyIPs_DefaultGuard(t *testing.T) {
	g := guardFromEnv(func(string) string { return "" }) // strict default

	ips := []net.IP{
		net.ParseIP("8.8.8.8"),         // public → allow
		net.ParseIP("127.0.0.1"),       // loopback → block
		net.ParseIP("10.1.2.3"),        // private → block
		net.ParseIP("169.254.169.254"), // link-local metadata → block
	}
	v := classifyIPs(g, ips)
	got := map[string]ipVerdict{}
	for _, r := range v {
		got[r.IP] = r
	}
	if !got["8.8.8.8"].Allowed {
		t.Errorf("public 8.8.8.8 should be allowed")
	}
	for _, blocked := range []string{"127.0.0.1", "10.1.2.3", "169.254.169.254"} {
		if got[blocked].Allowed {
			t.Errorf("%s should be blocked by default", blocked)
		}
		if got[blocked].Reason == "" {
			t.Errorf("%s blocked without a reason", blocked)
		}
	}
	// Metadata reason should mention metadata.
	if r := got["169.254.169.254"].Reason; r == "" {
		t.Errorf("metadata IP missing reason")
	}
}

func TestGuardFromEnv_Relaxations(t *testing.T) {
	// AllowPrivate=1 should let 10/8 through but NOT metadata.
	env := map[string]string{"AGEZT_HTTP_ALLOW_PRIVATE": "1"}
	g := guardFromEnv(func(k string) string { return env[k] })
	v := classifyIPs(g, []net.IP{net.ParseIP("10.0.0.5"), net.ParseIP("169.254.169.254")})
	byIP := map[string]bool{}
	for _, r := range v {
		byIP[r.IP] = r.Allowed
	}
	if !byIP["10.0.0.5"] {
		t.Errorf("AllowPrivate should permit 10.0.0.5")
	}
	if byIP["169.254.169.254"] {
		t.Errorf("metadata must stay blocked even with AllowPrivate")
	}

	// AllowLoopback=1 permits 127.0.0.1.
	env2 := map[string]string{"AGEZT_HTTP_ALLOW_LOOPBACK": "1"}
	g2 := guardFromEnv(func(k string) string { return env2[k] })
	v2 := classifyIPs(g2, []net.IP{net.ParseIP("127.0.0.1")})
	if !v2[0].Allowed {
		t.Errorf("AllowLoopback should permit 127.0.0.1")
	}
}
