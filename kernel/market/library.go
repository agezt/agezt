// SPDX-License-Identifier: MIT

package market

import (
	"fmt"
	"strings"
)

// compositeLibrary serves packs from the built-in Official catalogue AND every
// synced remote cached in the Store. The built-in seed always wins a name clash
// (a remote can't shadow Official), and remotes are resolved from the on-disk
// cache the Syncer populated — so install works fully offline after a sync.
// When several synced marketplaces carry the same pack, the NEWEST version
// wins (CompareVersions) unless a specific version was requested — an explicit
// version is an exact request, honored or refused with a list of what is
// actually cached, never silently substituted. That holds even when a Library
// implementation ignores the version argument: the composite verifies what it
// was handed. Ties resolve to the alphabetically-first marketplace, so the
// choice is deterministic rather than an accident of cache order.
type compositeLibrary struct {
	builtin Library
	store   *Store
}

// NewCompositeLibrary composes the built-in Library with the Store's synced
// marketplaces. Pass the builtin seed (plugins/builtinmarket) and the same Store
// the Manager uses.
func NewCompositeLibrary(builtin Library, store *Store) Library {
	return &compositeLibrary{builtin: builtin, store: store}
}

func (c *compositeLibrary) Marketplaces() []Marketplace {
	var out []Marketplace
	if c.builtin != nil {
		out = append(out, c.builtin.Marketplaces()...)
	}
	if c.store != nil {
		cached, err := c.store.CachedMarketplaces()
		if err == nil {
			out = append(out, cached...)
		}
	}
	return out
}

func (c *compositeLibrary) ResolvePack(marketplace, name, version string) (Pack, error) {
	// Built-in first: it owns the "official" name and never needs the cache.
	if marketplace == "" || marketplace == MarketplaceOfficial {
		if c.builtin != nil {
			if p, err := c.builtin.ResolvePack(marketplace, name, version); err == nil {
				// Defensive: the Library contract makes an explicit version an
				// exact request, but the interface cannot force an
				// implementation to honor it — one that ignores the argument
				// returns its own version with err == nil. Verify what came
				// back and treat a mismatch as the miss it should have been,
				// so a contract-violating source can never substitute a
				// version nobody asked for.
				if version == "" || CompareVersions(p.Version, version) == 0 {
					return p, nil
				}
				if marketplace == MarketplaceOfficial {
					return Pack{}, fmt.Errorf("market: builtin library returned %q at %s for requested %s", p.Name, p.Version, version)
				}
				// Unqualified: fall through to the synced marketplaces, which
				// may genuinely carry the requested version.
			} else if marketplace == MarketplaceOfficial {
				return Pack{}, err
			}
		}
	}
	if c.store != nil {
		if marketplace != "" {
			p, err := c.store.CachedPack(marketplace, name)
			if err != nil {
				return Pack{}, err
			}
			if version != "" && CompareVersions(p.Version, version) != 0 {
				return Pack{}, fmt.Errorf("market: %s caches %q at %s, not %s", marketplace, name, p.Version, version)
			}
			return p, nil
		}
		// Unqualified: search every synced marketplace for the name. With no
		// version requested, pick the NEWEST — not whichever marketplace name
		// happens to sort first, which is how CachedMarketplaces orders them;
		// ties keep the first (name-sorted) marketplace, so the choice stays
		// deterministic. With a version requested, it is an EXACT request:
		// honor it or name what is actually cached, never silently substitute
		// the newest. The builtin seed above still wins any clash by policy: a
		// remote cannot shadow Official.
		mps, err := c.store.CachedMarketplaces()
		if err != nil {
			return Pack{}, err
		}
		var best Pack
		found := false
		var cached []string
		for _, mp := range mps {
			p, perr := c.store.CachedPack(mp.Name, name)
			if perr != nil {
				continue
			}
			cached = append(cached, mp.Name+"@"+p.Version)
			if version != "" {
				if CompareVersions(p.Version, version) == 0 {
					return p, nil
				}
				continue
			}
			if !found || CompareVersions(p.Version, best.Version) > 0 {
				best, found = p, true
			}
		}
		if version != "" {
			if len(cached) == 0 {
				return Pack{}, fmt.Errorf("market: pack %q not found", name)
			}
			return Pack{}, fmt.Errorf("market: pack %q not found at version %s (cached: %s)", name, version, strings.Join(cached, ", "))
		}
		if found {
			return best, nil
		}
	}
	return Pack{}, fmt.Errorf("market: pack %q not found", name)
}
