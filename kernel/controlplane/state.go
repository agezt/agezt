// SPDX-License-Identifier: MIT

package controlplane

// State store inspection handlers. Read-only — surfaces the
// kernel's mutable state to operators who want to debug
// "what does the agent think it knows?" without shelling
// into the data directory.

import (
	"encoding/json"
	"net"
)

func (s *Server) handleStateList(conn net.Conn, req Request) {
	nsRaw := req.Args["namespace"]
	ns, _ := nsRaw.(string)
	if ns == "" {
		// No namespace → enumerate them all (sorted, courtesy of
		// FileStore.Namespaces).
		out := make([]any, 0)
		for _, n := range s.k.State().Namespaces() {
			out = append(out, n)
		}
		s.writeResp(conn, Response{
			ID:   req.ID,
			Type: RespResult,
			Result: map[string]any{
				"namespaces": out,
				"namespace":  "",
			},
		})
		return
	}
	// Namespace set → enumerate keys within it.
	keys, err := s.k.State().Keys(ns)
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}
	out := make([]any, 0, len(keys))
	for _, k := range keys {
		out = append(out, k)
	}
	s.writeResp(conn, Response{
		ID:   req.ID,
		Type: RespResult,
		Result: map[string]any{
			"keys":      out,
			"namespace": ns,
		},
	})
}

func (s *Server) handleStateGet(conn net.Conn, req Request) {
	nsRaw := req.Args["namespace"]
	ns, _ := nsRaw.(string)
	keyRaw := req.Args["key"]
	key, _ := keyRaw.(string)
	if ns == "" || key == "" {
		s.writeResp(conn, Response{
			ID: req.ID, Type: RespError,
			Error: "args.namespace and args.key required",
		})
		return
	}

	raw, found, err := s.k.State().Get(ns, key)
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}
	// Decode the stored RawMessage so the response uses the value's
	// native JSON type (number/string/object/...) rather than a
	// double-encoded string. Missing keys come back as `null` with
	// found=false so the client distinguishes "exists but null"
	// from "absent".
	var value any
	if found {
		if err := json.Unmarshal(raw, &value); err != nil {
			s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "state value corrupt: " + err.Error()})
			return
		}
	}
	s.writeResp(conn, Response{
		ID:   req.ID,
		Type: RespResult,
		Result: map[string]any{
			"namespace": ns,
			"key":       key,
			"found":     found,
			"value":     value, // nil → JSON null when found=false
		},
	})
}
