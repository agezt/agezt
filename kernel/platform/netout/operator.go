// SPDX-License-Identifier: MIT

package netout

import (
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/agezt/agezt/kernel/netguard"
)

// OperatorClient returns a client for an endpoint the operator configured — a
// provider or embedding base URL, a channel server, an outbound webhook, a
// peer node. timeout bounds each request (0 = none, as http.DefaultClient).
//
// Its transport is http.DefaultTransport's, unchanged in every respect an
// operator could observe — HTTP(S)_PROXY, connection pooling, HTTP/2, dial and
// TLS timeouts — except one: the dialer refuses link-local addresses
// (169.254.0.0/16 including the cloud metadata endpoint, fe80::/10) and the
// unspecified address. Loopback and private networks stay reachable; a local
// Ollama or a LAN Matrix server is a legitimate destination. These clients used
// to be bare http.Clients that would dial the metadata service if a configured
// or synced URL pointed there.
//
// All operator clients share one transport, so callers that build a client per
// request keep connection reuse.
func OperatorClient(timeout time.Duration) *http.Client {
	return &http.Client{Timeout: timeout, Transport: OperatorTransport()}
}

// OperatorTransport is the shared transport behind OperatorClient, for callers
// that wrap it (a signing or auth RoundTripper).
func OperatorTransport() http.RoundTripper { return operatorTransport() }

var operatorTransport = sync.OnceValue(func() *http.Transport {
	return guardedClone(netguard.New(netguard.AllowLoopback(), netguard.AllowPrivate()))
})

// MetadataClient is for the credential code whose job is to query the cloud
// metadata service: AWS IMDS (169.254.169.254) and the GCE metadata server.
// It is http.DefaultTransport with no address guard. Nothing else may use it.
func MetadataClient(timeout time.Duration) *http.Client {
	return &http.Client{Timeout: timeout, Transport: metadataTransport()}
}

var metadataTransport = sync.OnceValue(func() *http.Transport {
	return http.DefaultTransport.(*http.Transport).Clone()
})

// guardedClone clones http.DefaultTransport and installs g on its dialer,
// keeping the default dialer's timeouts.
func guardedClone(g *netguard.Guard) *http.Transport {
	tr := http.DefaultTransport.(*http.Transport).Clone()
	d := &net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second, Control: g.Control}
	tr.DialContext = d.DialContext
	return tr
}
