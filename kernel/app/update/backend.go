// SPDX-License-Identifier: MIT

// Package update owns operator update use cases over the verified update engine.
package update

import (
	"context"
	"time"

	core "github.com/agezt/agezt/kernel/update"
)

// Backend retains the existing update engine's verification and drain contract.
type Backend interface {
	Check(context.Context) (*core.CheckResult, error)
	Apply(context.Context, *core.UpdateInfo, func(context.Context, time.Duration) core.DrainResult) error
}
