// SPDX-License-Identifier: MIT

package controlplane

// Provenance: Control-plane skill: argResources helper + handleSkillFiles +
//             handleSkillReadFile + handleSkillHygiene (file-related surface). Code
//             extracted from skill.go during the Day-139 god-file split. Public API
//             unchanged.

import (
	"context"
	"fmt"
	appskill "github.com/agezt/agezt/kernel/app/skill"
	"net"
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

func (s *Server) handleSkillFiles(conn net.Conn, req Request) {
	id, err := requiredArgString(req.Args, "id")
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	out, err := appskill.NewObservations(s.k.Forge(), s.k.Journal()).Files(context.Background(), appskill.GetInput{ID: id})
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	writeSkillReadResult(s, conn, req, out)
}
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
	out, err := appskill.NewObservations(s.k.Forge(), s.k.Journal()).ReadFile(context.Background(), appskill.ReadFileInput{ID: id, Path: path})
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	writeSkillReadResult(s, conn, req, out)
}
func (s *Server) handleSkillHygiene(conn net.Conn, req Request) {
	out, err := appskill.NewObservations(s.k.Forge(), s.k.Journal()).Hygiene(context.Background(), appskill.HygieneInput{IdleDays: dlInt(req.Args, "idle_days")})
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	writeSkillReadResult(s, conn, req, out)
}
