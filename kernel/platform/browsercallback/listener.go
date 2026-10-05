// SPDX-License-Identifier: MIT

package browsercallback

import (
	"net"
	"net/http"
	"time"
)

// Listener holds a bound callback socket and its prepared HTTP server.
// Hosts publish their session ownership before launching Serve.
type Listener struct {
	ln  net.Listener
	srv *http.Server
}

func Prepare(addr string, complete Complete, closeListener func()) (*Listener, error) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/auth/callback", func(w http.ResponseWriter, r *http.Request) {
		Handle(w, r, complete, closeListener)
	})
	return &Listener{ln: ln, srv: &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}}, nil
}

func (l *Listener) Serve() error { return l.srv.Serve(l.ln) }

// Close retains the existing server-close behavior; lifetime refinements follow
// separately from this mechanical listener move.
func (l *Listener) Close() error { return l.srv.Close() }
