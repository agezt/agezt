// SPDX-License-Identifier: MIT
//
// cmd/agezt WebUI browser / runtime helpers: envDisabled (the generic
// disabled-check used by shouldOpenWebUI) +
// shouldOpenWebUI (the auto-open check) +
// openBrowser (the cross-platform launcher) +
// webAllowedHosts (the bind-address->host list).
// Extracted from httpsurfaces.go during the Day-208 god-file split.
// Public API unchanged.
package main

import (
	"net"
	"os"
	"os/exec"
	stdruntime "runtime"
	"strings"

	"github.com/agezt/agezt/internal/brand"
)

func shouldOpenWebUI() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(brand.EnvPrefix + "WEB_OPEN"))) {
	case "off", "disabled", "none", "no", "0", "false":
		return false
	}
	// `go test` must not launch a desktop browser.
	return !strings.HasSuffix(os.Args[0], ".test")
}

func openBrowser(url string) error {
	var cmd *exec.Cmd
	switch stdruntime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	return cmd.Start()
}

func webAllowedHosts(bindAddr string) []string {
	var hosts []string
	if h, _, err := net.SplitHostPort(bindAddr); err == nil {
		if ip := net.ParseIP(h); ip == nil || !ip.IsUnspecified() {
			hosts = append(hosts, h)
		}
	}
	for _, h := range strings.Split(os.Getenv(brand.EnvPrefix+"WEB_ALLOWED_HOSTS"), ",") {
		if h = strings.TrimSpace(h); h != "" {
			hosts = append(hosts, h)
		}
	}
	return hosts
}

// buildTunnel starts a tunnel to a local HTTP service when AGEZT_TUNNEL
// (cloudflare/cloudflared|ngrok|tailscale|tailscale-funnel) or AGEZT_TUNNEL_CMD
// (a custom command) is set. It targets AGEZT_TUNNEL_TARGET, else the live Web
// UI listener, else the REST addr. The supervised binary's remote URL is printed
// to the daemon log once it connects. Returns "" (disabled) when no tunnel is
// configured.
//
//	AGEZT_TUNNEL         provider preset: cloudflare/cloudflared | ngrok | tailscale | tailscale-funnel
//	AGEZT_TUNNEL_CMD     explicit command (whitespace-split), overrides the preset
//	AGEZT_TUNNEL_TARGET  local URL to expose (default: the Web UI, else REST, addr)
