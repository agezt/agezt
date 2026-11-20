// SPDX-License-Identifier: MIT

package webui

// The console's own view of the operation catalogue: the routes of one kind.
// The catalogue itself does not need this helper — it is a reading convenience
// for the console's tests, so it lives here rather than in the contract
// package.

import "github.com/agezt/agezt/kernel/contract/opapi"

// consoleRoutesOfKind returns the catalogue's routes of one kind.
func consoleRoutesOfKind(kind opapi.HTTPRouteKind) []opapi.HTTPRoute {
	routes := []opapi.HTTPRoute{}
	for _, route := range opapi.ConsoleRoutes() {
		if route.Kind == kind {
			routes = append(routes, route)
		}
	}
	return routes
}

// consoleRoutePaths returns every path the catalogue binds.
func consoleRoutePaths() []string {
	paths := []string{}
	for _, route := range opapi.ConsoleRoutes() {
		paths = append(paths, route.Path)
	}
	return paths
}
