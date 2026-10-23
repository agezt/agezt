// SPDX-License-Identifier: MIT

package remoteapi

// Filter narrows an artifact query: the kind of artifact, where it came from
// and the run it belongs to. An empty field matches everything.
//
// It lives beside the artifact row because it is a query OVER that row, and
// the tools that list artifacts are PLUGINS: a plugin may reach the contract
// and the platform layers and nothing else, so a row a plugin has to name
// cannot live in a module's implementation package. The artifacts module
// keeps the name as an alias.
type Filter struct {
	Kind   string
	Source string
	Corr   string
}
