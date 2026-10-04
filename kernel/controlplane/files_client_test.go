// SPDX-License-Identifier: MIT

package controlplane_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/agezt/agezt/kernel/app/files"
	apptools "github.com/agezt/agezt/kernel/app/tools"
	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/kernel/edict"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

func TestFileErrorCodeSurvivesEveryClientMode(t *testing.T) {
	root := filepath.Join(t.TempDir(), "not-created")
	t.Setenv("AGEZT_FILE_ROOT", root)
	k, _, client, _ := startPairWithConfig(t, runtime.Config{Provider: mock.New(), NewToolInvoker: apptools.NewInvoker})
	k.Edict().SetLevel(edict.CapFileWrite, edict.LevelDeny)
	ctx := context.Background()
	args := map[string]any{"path": "nested", "parents": true}
	for name, call := range map[string]func() error{
		"Call":              func() error { _, err := client.Call(ctx, controlplane.CmdFileMkdir, args); return err },
		"CallRaw":           func() error { _, err := client.CallRaw(ctx, controlplane.CmdFileMkdir, args); return err },
		"Stream":            func() error { _, err := client.Stream(ctx, controlplane.CmdFileMkdir, args, nil); return err },
		"StreamUntilCancel": func() error { return client.StreamUntilCancel(ctx, controlplane.CmdFileMkdir, args, nil) },
	} {
		t.Run(name, func(t *testing.T) {
			err := call()
			var remote *controlplane.ErrServerError
			if !errors.As(err, &remote) || remote.Code != files.Denied || remote.Msg == "" {
				t.Fatalf("domain classification lost: %v", err)
			}
		})
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatalf("denied client mode changed disk: %v", err)
	}
	_, err := client.Call(ctx, "unknown_operation_for_error", nil)
	var remote *controlplane.ErrServerError
	if !errors.As(err, &remote) || remote.Code != "" || remote.Msg == "" {
		t.Fatalf("legacy error changed: %v", err)
	}
}
