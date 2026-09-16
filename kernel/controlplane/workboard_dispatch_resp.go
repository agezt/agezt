// SPDX-License-Identifier: MIT
//
// Workboard dispatch HTTP-response helper (workboardWriteResp).
// Extracted from workboard_dispatch.go during Day 211 god-file refactor (#70).
// Public API unchanged.
package controlplane

import (
	"errors"
	"net"

	"github.com/agezt/agezt/kernel/workboard"
)

func workboardWriteResp(s *Server, conn net.Conn, req Request, task workboard.Task, err error) {
	if err != nil {
		msg := err.Error()
		if errors.Is(err, workboard.ErrNotFound) {
			msg = "unknown workboard task: " + stringArg(req.Args, "id")
		}
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: msg})
		return
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{"task": workboardTaskView(task)}})
}
