// SPDX-License-Identifier: MIT

package netout

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/agezt/agezt/kernel/netguard"
)

// GatewayGET performs the bounded guarded GET used by provider and gateway checks.
// It retains a fresh transport, a ten-second background timeout, local/LAN access
// and dial-time refusal of link-local, multicast and unspecified targets.
// The max parameter is the caller's positive body-size bound. Partial reads retain
// the legacy best-effort result; caller-context adaptation is a separate migration.
func GatewayGET(fullURL, keyHeader, key string, max int64) (body []byte, status int, contentType string, err error) {
	u, perr := url.Parse(fullURL)
	if perr != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, 0, "", &url.Error{Op: "parse", URL: fullURL, Err: perr}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	hreq, err := http.NewRequestWithContext(ctx, http.MethodGet, fullURL, nil)
	if err != nil {
		return nil, 0, "", err
	}
	if key != "" {
		hreq.Header.Set(keyHeader, key)
	}
	client := netguard.New(netguard.AllowLoopback(), netguard.AllowPrivate()).HTTPClient(10 * time.Second)
	resp, err := client.Do(hreq)
	if err != nil {
		return nil, 0, "", err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, max))
	return b, resp.StatusCode, resp.Header.Get("Content-Type"), nil
}
