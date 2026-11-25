// SPDX-License-Identifier: MIT

package webui

import (
	"errors"
	"net/http"

	"github.com/agezt/agezt/kernel/app/files"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/controlplane"
)

// fileMutationBody reads and checks a file manager mutation's body.
//
// The body is measured against the schema the console's own endpoint list
// names for the endpoint, so a mistyped field is refused in the registry's
// words instead of becoming a silent zero value.
func fileMutationBody(w http.ResponseWriter, r *http.Request, name opapi.NativeRouteName, target any) bool {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return false
	}
	ok, err := decodeNativeBody(w, r, name, target)
	if !ok {
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
		}
		return false
	}
	return true
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
