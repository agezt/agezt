// SPDX-License-Identifier: MIT

package marketapi

// Library is the port through which a catalogue of packs is read: the
// marketplaces it serves and the packs it carries.
//
// The built-in "Official" catalogue is a PLUGIN, so it implements this port
// rather than naming the market module — and the market's own composite
// library composes catalogues through it.
type Library interface {
	// Marketplaces returns the catalogues this library serves.
	Marketplaces() []Marketplace
	// ResolvePack returns the full pack. marketplace may be "" to search all;
	// version may be "" for the catalogued version.
	ResolvePack(marketplace, name, version string) (Pack, error)
}
