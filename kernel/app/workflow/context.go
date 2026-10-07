// SPDX-License-Identifier: MIT
package workflow

import (
	"context"
	"github.com/agezt/agezt/kernel/contract/opapi"
)

// A host-bound operation identity wins. Direct service users retain explicit
// lifecycle IDs or fresh runtime correlation when no operation host is present.
func operationCorrelation(ctx context.Context, explicit string, fresh func() string) string {
	if id := opapi.CorrelationFromContext(ctx); id != "" {
		return id
	}
	if explicit != "" {
		return explicit
	}
	if fresh != nil {
		return fresh()
	}
	return ""
}
