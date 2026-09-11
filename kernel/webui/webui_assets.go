// SPDX-License-Identifier: MIT

package webui

// Static asset handlers (SPA shell, assets/, favicon) plus the
// content-type detector. Carved out of webui.go during the Day 26 god
// file split #1 so the main file can focus on routing + security.

import (
	"io"
	"io/fs"
	"net/http"
	"strings"
)

func (s *Server) handleSPA(w http.ResponseWriter, _ *http.Request) {
	body, err := fs.ReadFile(s.dist, "index.html")
	if err != nil {
		http.Error(w, "web ui bundle missing", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(body)
}

// handleAssets serves the content-hashed bundle under /assets/ straight from the
// embedded FS. Hashes make each file immutable, so it gets a long immutable
// cache; a missing asset 404s rather than falling through to the SPA shell. The
// Content-Type is set EXPLICITLY by extension rather than via the stdlib's
// mime.TypeByExtension, which on Windows reads the registry and can return
// text/plain for .css (browsers then refuse the stylesheet under nosniff).
func (s *Server) handleAssets() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(r.URL.Path, "/")
		f, err := s.dist.Open(name)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		defer f.Close()
		st, err := f.Stat()
		if err != nil || st.IsDir() {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", contentType(name))
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		if rs, ok := f.(io.ReadSeeker); ok {
			http.ServeContent(w, r, name, st.ModTime(), rs)
			return
		}
		_, _ = io.Copy(w, f)
	}
}

// handleFavicon serves the SPA's icon from the bundle if present, else 204 (so a
// browser's automatic /favicon.ico probe doesn't 404/401-noise the console).
func handleFavicon(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusNoContent)
}

// contentType maps a bundle filename to a stable MIME type, independent of the
// host OS mime registry (see handleAssets).
func contentType(name string) string {
	switch {
	case strings.HasSuffix(name, ".js"), strings.HasSuffix(name, ".mjs"):
		return "text/javascript; charset=utf-8"
	case strings.HasSuffix(name, ".css"):
		return "text/css; charset=utf-8"
	case strings.HasSuffix(name, ".html"):
		return "text/html; charset=utf-8"
	case strings.HasSuffix(name, ".json"), strings.HasSuffix(name, ".map"):
		return "application/json"
	case strings.HasSuffix(name, ".svg"):
		return "image/svg+xml"
	case strings.HasSuffix(name, ".woff2"):
		return "font/woff2"
	case strings.HasSuffix(name, ".woff"):
		return "font/woff"
	case strings.HasSuffix(name, ".png"):
		return "image/png"
	default:
		return "application/octet-stream"
	}
}

// handleEvents streams the bus as Server-Sent Events. It subscribes to the
// whole firehose and relays each event as one `data: {json}` frame, flushing
// per event, until the client disconnects (request ctx) or the bus closes.
