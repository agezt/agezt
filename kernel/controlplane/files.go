// SPDX-License-Identifier: MIT

package controlplane

import (
	"errors"

	"github.com/agezt/agezt/kernel/app/files"
)

func registerFileCommands() {
	// The console workspace is daemon-global; tenant tokens cannot mutate it.
	register(
		commandSpec{Cmd: CmdFileMkdir, Handler: handleFileMutation},
		commandSpec{Cmd: CmdFileRename, Handler: handleFileMutation},
		commandSpec{Cmd: CmdFileDelete, Handler: handleFileMutation},
	)
}

func handleFileMutation(dc *DispatchCtx) {
	ctx := dc.K.WithActorCorrelation(dc.Ctx, "operator", dc.CorrelationID)
	output, err := files.Apply(ctx, dc.K, dc.CorrelationID, dc.Req.ID, dc.Req.Cmd, dc.Req.Args)
	if err != nil {
		code := files.Unavailable
		var classified *files.Error
		if errors.As(err, &classified) {
			code = classified.Code
		}
		dc.S.writeResp(dc.Conn, Response{ID: dc.Req.ID, Type: RespError, Error: err.Error(), ErrorCode: code})
		return
	}
	dc.S.writeResp(dc.Conn, Response{ID: dc.Req.ID, Type: RespResult, Result: output})
}
