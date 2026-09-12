// SPDX-License-Identifier: MIT

// Control-plane skill: argResources helper + handleSkillFiles + handleSkillReadFile + handleSkillHygiene (file-related surface).
// Code extracted from skill.go during the Day-139 god-file split.
// Public API unchanged.
package controlplane



import (
	"fmt"
	"net"
	"time"
)

func argResources(args map[string]any, key string) (map[string][]byte, error) {
	raw, ok := args[key]
	if !ok || raw == nil {
		return nil, nil
	}
	obj, ok := raw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("args.%s must be an object of {path: content}", key)
	}
	if len(obj) == 0 {
		return nil, nil
	}
	out := make(map[string][]byte, len(obj))
	for path, v := range obj {
		content, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("args.%s[%q] must be a string", key, path)
		}
		out[path] = []byte(content)
	}
	return out, nil
}

// handleSkillFiles lists a skill's bundle resources (relative paths) plus the
// absolute bundle directory the agent runs scripts from. Read-only.
func (s *Server) handleSkillFiles(conn net.Conn, req Request) {
	id, err := requiredArgString(req.Args, "id")
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	sk, found, err := s.k.Forge().Get(id)
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	if !found {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "no skill with id " + id})
		return
	}
	bundles := s.k.Forge().Bundles()
	files := sk.Resources
	dir := ""
	if bundles != nil {
		if live, lerr := bundles.List(sk.Name); lerr == nil && live != nil {
			files = live // the on-disk truth, in case the manifest drifted
		}
		dir = bundles.Dir(sk.Name)
	}
	s.writeResp(conn, Response{
		ID: req.ID, Type: RespResult,
		Result: map[string]any{"id": sk.ID, "name": sk.Name, "files": files, "dir": dir, "count": len(files)},
	})
}

// handleSkillReadFile returns the text content of one bundle resource. Read-only;
// the bundle store rejects any path that escapes the skill's directory.
func (s *Server) handleSkillReadFile(conn net.Conn, req Request) {
	id, err := requiredArgString(req.Args, "id")
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	path, err := requiredArgString(req.Args, "path")
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	sk, found, err := s.k.Forge().Get(id)
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	if !found {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "no skill with id " + id})
		return
	}
	bundles := s.k.Forge().Bundles()
	if bundles == nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "skill bundles are not available on this daemon"})
		return
	}
	data, err := bundles.Read(sk.Name, path)
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	s.writeResp(conn, Response{
		ID: req.ID, Type: RespResult,
		Result: map[string]any{"id": sk.ID, "name": sk.Name, "path": path, "content": string(data), "bytes": len(data)},
	})
}

// handleSkillHygiene reports which active skills look idle (never used, or not
// used in idle_days) so an operator can prune dead weight from the retrieval pool
// (M858). Read-only; the cleanup action is the existing CmdSkillQuarantine.
func (s *Server) handleSkillHygiene(conn net.Conn, req Request) {
	days := dlInt(req.Args, "idle_days")
	if days <= 0 {
		days = 30
	}
	cutoff := time.Now().Add(-time.Duration(days) * 24 * time.Hour).UnixMilli()
	rep, err := s.k.Forge().Hygiene(cutoff)
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	idle := make([]any, 0, len(rep.Idle))
	for _, sk := range rep.Idle {
		v := skillView(sk)
		v["uses"] = sk.Metrics.Uses
		v["last_used_ms"] = sk.Metrics.LastUsedMS
		idle = append(idle, v)
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{
		"idle_days": days, "total": rep.Total, "active": rep.Active,
		"idle": idle, "idle_count": len(idle),
	}})
}

