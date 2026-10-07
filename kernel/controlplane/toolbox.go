// SPDX-License-Identifier: MIT

package controlplane

import (
	"context"

	"github.com/agezt/agezt/kernel/toolbox"
)

// nativeToolboxReader binds existing bounded host discovery functions.
type nativeToolboxReader struct{}

func (nativeToolboxReader) Detect(ctx context.Context) toolbox.Inventory { return toolbox.Detect(ctx) }
func (nativeToolboxReader) Outdated(ctx context.Context) map[string]bool {
	return toolbox.Outdated(ctx)
}

// nativeToolboxInstaller keeps package-manager execution at the host boundary.
type nativeToolboxInstaller struct{}

func (nativeToolboxInstaller) Install(ctx context.Context, name string) toolbox.InstallResult {
	return toolbox.Install(ctx, name)
}
