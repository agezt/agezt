// SPDX-License-Identifier: MIT

package webui

import (
	"errors"
	"net/http"

	"github.com/agezt/agezt/kernel/app/files"
	"github.com/agezt/agezt/kernel/controlplane"
)

func fileMutationBody(w http.ResponseWriter, r *http.Request) (map[string]any, bool) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return nil, false
	}
	return readJSONBody(w, r)
}

func (s *Server) callFileMutation(w http.ResponseWriter, r *http.Request, command string, args map[string]any) {
	output, err := s.client.Call(r.Context(), command, args)
	if err == nil {
		writeJSON(w, http.StatusOK, output)
		return
	}
	status, message := http.StatusBadGateway, err.Error()
	var remote *controlplane.ErrServerError
	if errors.As(err, &remote) {
		message = remote.Msg
		switch remote.Code {
		case files.InvalidPath:
			status = http.StatusBadRequest
		case files.NotFound:
			status, message = http.StatusNotFound, "not found"
		case files.Symlink, files.Denied:
			status = http.StatusForbidden
		case files.IOFailure:
			status = http.StatusInternalServerError
		}
	}
	http.Error(w, message, status)
}
