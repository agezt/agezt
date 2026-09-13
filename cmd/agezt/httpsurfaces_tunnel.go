// SPDX-License-Identifier: MIT

package main

// WebUI tunnel: buildTunnel + the tunnel target / public URL helpers
// (tunnelTargetFromEnv, sameURLTarget, publicURLHost, tunnelPublicURL,
// urlWithToken, addrToURL). Carved out of httpsurfaces.go during the
// Day 161 god-file split.
// Public API unchanged.

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/tunnel"
)
func buildTunnel(ctx context.Context, stdout io.Writer, web webUISurface) string {
	provider := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "TUNNEL"))
	cmdStr := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "TUNNEL_CMD"))
	if provider == "" && cmdStr == "" {
		return ""
	}

	target := tunnelTargetFromEnv(web)
	targetsWebUI := target != "" && sameURLTarget(target, web.localURL)

	cfg := tunnel.Config{
		Provider:  provider,
		TargetURL: target,
		OnURL: func(u string) {
			if targetsWebUI && web.allowHost != nil {
				if h := publicURLHost(u); h != "" {
					web.allowHost(h)
				}
			}
			public := tunnelPublicURL(u, web, targetsWebUI)
			fmt.Fprintf(stdout, "  tunnel URL       : %s  [the service is now reachable through the tunnel]\n", public)
			if targetsWebUI && web.passwordOn && web.passwordStrict {
				fmt.Fprintf(stdout, "  tunnel auth      : Web UI password strict mode is enabled; public host was allowlisted and token+password are required\n")
			} else if targetsWebUI && web.passwordOn {
				fmt.Fprintf(stdout, "  tunnel auth      : Web UI password is enabled; public URL opens password login and host was allowlisted automatically\n")
			} else if targetsWebUI {
				fmt.Fprintf(stdout, "  tunnel auth      : WARNING Web UI password is not set; access is token-only\n")
			}
		},
	}
	if cmdStr != "" {
		cfg.Command = strings.Fields(cmdStr)
	}

	tun, err := tunnel.New(cfg)
	if err != nil {
		fmt.Fprintf(stdout, "  tunnel           : disabled (%v)\n", err)
		return ""
	}
	go tun.Start(ctx)

	what := "custom command"
	if cmdStr == "" {
		what = provider
	}
	desc := fmt.Sprintf("%s → exposing %s (public URL prints here once connected)", what, target)
	if target == "" {
		desc = fmt.Sprintf("%s (custom command; no local target derived)", what)
	}
	return desc
}

func tunnelTargetFromEnv(web webUISurface) string {
	if target := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "TUNNEL_TARGET")); target != "" {
		return target
	}
	if web.localURL != "" {
		return web.localURL
	}
	if webAddr := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "WEB_ADDR")); webAddr != "" && !envDisabled(webAddr) {
		return addrToURL(webAddr)
	}
	if rest := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "REST_ADDR")); rest != "" && !envDisabled(rest) {
		return addrToURL(rest)
	}
	return ""
}

func sameURLTarget(a, b string) bool {
	return strings.EqualFold(strings.TrimRight(strings.TrimSpace(a), "/"), strings.TrimRight(strings.TrimSpace(b), "/"))
}

func publicURLHost(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" {
		return ""
	}
	return u.Host
}

func tunnelPublicURL(raw string, web webUISurface, targetsWebUI bool) string {
	if targetsWebUI && web.token != "" && (!web.passwordOn || web.passwordStrict) {
		return urlWithToken(raw, web.token)
	}
	return raw
}

func urlWithToken(raw, token string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme == "" || u.Host == "" {
		return raw
	}
	if u.Path == "" {
		u.Path = "/"
	}
	q := u.Query()
	q.Set("token", token)
	u.RawQuery = q.Encode()
	return u.String()
}

// addrToURL turns a listen addr (host:port, or :port) into a loopback http URL.
func addrToURL(addr string) string {
	if strings.HasPrefix(addr, ":") {
		addr = "127.0.0.1" + addr
	}
	return "http://" + addr
}

// writeAPIListenToken persists the freshly-minted HTTP listen token to a 0600
// file under the daemon's base directory and returns a short prefix suitable
// for surfacing in the boot banner without leaking the full secret. Banner
// leak audit (VULN banner-token-leak): the FULL token must NEVER appear on
// stdout/stderr or anywhere a log-shipper / `journalctl` / `agt status` will
// scrape it. Callers are expected to embed `prefix` in the banner and point
// operators at the file path for the live secret. On a write failure we
// return the empty prefix AND the error so the caller can fail closed rather
// than print nothing and silently leave the operator without the secret.
