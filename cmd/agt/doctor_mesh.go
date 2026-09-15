// SPDX-License-Identifier: MIT
//
// cmd/agt doctor local file/memory/auth/hop-limit checks
// (checkMemoryStoreFile, checkMeshAuth, checkMeshHopLimit).
// Extracted from doctor_mesh.go during Day 211 god-file refactor (#40, #65).
// Public API unchanged.
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/agezt/agezt/kernel/meshctx"
	"github.com/agezt/agezt/plugins/tools/peer"
)

func checkMemoryStoreFile(base string, repair bool) doctorCheck {
	const name = "memory store"
	path := filepath.Join(base, "memory", "memory.json")
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		if !repair {
			return warn(name, "memory.json is missing", "run `agt doctor --repair` to recreate an empty memory store")
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return fail(name, "cannot create memory dir: "+err.Error(), "check filesystem permissions")
		}
		if err := os.WriteFile(path, []byte("{}\n"), 0o600); err != nil {
			return fail(name, "cannot recreate memory.json: "+err.Error(), "check filesystem permissions")
		}
		return ok(name, "recreated missing memory.json as empty store")
	}
	if err != nil {
		return fail(name, "cannot read memory.json: "+err.Error(), "check filesystem permissions")
	}
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		if !repair {
			return warn(name, "memory.json is empty", "run `agt doctor --repair` to reset it to an empty JSON object")
		}
		if err := os.WriteFile(path, []byte("{}\n"), 0o600); err != nil {
			return fail(name, "cannot repair empty memory.json: "+err.Error(), "check filesystem permissions")
		}
		return ok(name, "repaired empty memory.json as empty store")
	}
	clean := bytes.TrimPrefix(trimmed, []byte{0xEF, 0xBB, 0xBF})
	var decoded map[string]json.RawMessage
	if err := json.Unmarshal(clean, &decoded); err != nil {
		if !repair {
			return fail(name, "memory.json is not valid JSON: "+err.Error(), "run `agt doctor --repair` to back it up and recreate an empty store")
		}
		backup := path + ".bad-" + time.Now().Format("20060102-150405")
		if err := os.Rename(path, backup); err != nil {
			return fail(name, "cannot back up corrupt memory.json: "+err.Error(), "check filesystem permissions")
		}
		if err := os.WriteFile(path, []byte("{}\n"), 0o600); err != nil {
			return fail(name, "backed up corrupt memory.json but could not recreate it: "+err.Error(), "restore from "+backup+" after fixing permissions")
		}
		return ok(name, "backed up corrupt memory.json to "+filepath.Base(backup)+" and recreated empty store")
	}
	if !bytes.Equal(clean, trimmed) {
		if !repair {
			return warn(name, "memory.json has a UTF-8 BOM", "daemon tolerates it, but `agt doctor --repair` will normalize the file")
		}
		if err := os.WriteFile(path, append(clean, '\n'), 0o600); err != nil {
			return fail(name, "cannot rewrite memory.json without BOM: "+err.Error(), "check filesystem permissions")
		}
		return ok(name, fmt.Sprintf("normalized memory.json (%d record(s), BOM removed)", len(decoded)))
	}
	return ok(name, fmt.Sprintf("memory.json valid (%d record(s))", len(decoded)))
}
func checkMeshAuth(peers map[string]peer.Peer) doctorCheck {
	var tokenless []string
	for name, p := range peers {
		if p.Token == "" {
			tokenless = append(tokenless, name)
		}
	}
	if len(tokenless) == 0 {
		return ok("mesh-auth", fmt.Sprintf("all %d peer(s) authenticate with a token", len(peers)))
	}
	sort.Strings(tokenless)
	return warn("mesh-auth",
		fmt.Sprintf("%d/%d peer(s) have no token — unauthenticated delegation: %s",
			len(tokenless), len(peers), strings.Join(tokenless, ", ")),
		"add a token: AGEZT_PEERS=\"name=url|token,…\"")
}
func checkMeshHopLimit() doctorCheck {
	eff, raw, valid := meshctx.MaxHopsConfig()
	if valid {
		return ok("mesh-hops", fmt.Sprintf("delegation hop limit = %d (AGEZT_MESH_MAX_HOPS)", eff))
	}
	return warn("mesh-hops",
		fmt.Sprintf("AGEZT_MESH_MAX_HOPS=%q is invalid and ignored; using default %d", raw, eff),
		fmt.Sprintf("set an integer in [1, %d]", meshctx.MaxConfigurableHops))
}
