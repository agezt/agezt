// SPDX-License-Identifier: MIT

package main

import (
	apptools "github.com/agezt/agezt/kernel/app/tools"
	kernelruntime "github.com/agezt/agezt/kernel/runtime"
)

// openAppKernel is shared by primary and tenant composition. Every Open binds
// the application entry to that kernel's own policy, audit and lookup ports.
func openAppKernel(cfg kernelruntime.Config) (*kernelruntime.Kernel, error) {
	cfg.NewToolInvoker = apptools.NewInvoker
	return kernelruntime.Open(cfg)
}
