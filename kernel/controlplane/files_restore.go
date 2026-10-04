// SPDX-License-Identifier: MIT

package controlplane

import (
	"errors"
	"path/filepath"

	"github.com/agezt/agezt/kernel/app/files"
	"github.com/agezt/agezt/kernel/platform/rollbackstore"
)

func handleFileRestore(dc *DispatchCtx) {
	id, _ := dc.Req.Args["id"].(string)
	ctx := dc.K.WithActorCorrelation(dc.Ctx, "operator", dc.CorrelationID)
	path := filepath.Join(dc.S.baseDir, filepath.FromSlash(rollbackstore.RelativePath))
	output, err := files.ApplyRestore(ctx, dc.K, dc.CorrelationID, dc.Req.ID, path, id)
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
