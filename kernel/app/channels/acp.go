// SPDX-License-Identifier: MIT
package channels

import (
	"context"
	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/acpcatalog"
	"os"
	"strings"
)

type ACPDiscovery func(context.Context, string, bool) acpcatalog.Inventory
type ACPInventory struct {
	active   func() string
	discover ACPDiscovery
}

func NewACPInventory(active func() string, discover ACPDiscovery) *ACPInventory {
	if active == nil {
		active = func() string { return os.Getenv(brand.EnvPrefix + "ACP_AGENT_CMD") }
	}
	if discover == nil {
		discover = acpcatalog.Discover
	}
	return &ACPInventory{active: active, discover: discover}
}

type ACPInput struct{}
type ACPOutput = acpcatalog.Inventory

// List keeps the caller context and uses the selected discovery's ordinary
// cached refresh policy. Discovery failures remain fields in the inventory.
func (s *ACPInventory) List(ctx context.Context, _ ACPInput) (ACPOutput, error) {
	active := strings.TrimSpace(s.active())
	return s.discover(ctx, active, false), nil
}
