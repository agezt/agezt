// SPDX-License-Identifier: MIT

package market

import "fmt"

// compositeLibrary serves packs from the built-in Official catalogue AND every
// synced remote cached in the Store. The built-in seed always wins a name clash
// (a remote can't shadow Official), and remotes are resolved from the on-disk
// cache the Syncer populated — so install works fully offline after a sync.
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
				return p, nil
			} else if marketplace == MarketplaceOfficial {
				return Pack{}, err
			}
		}
	}
	if c.store != nil {
		if marketplace != "" {
			return c.store.CachedPack(marketplace, name)
		}
		// Unqualified: search every synced marketplace for the name.
		mps, err := c.store.CachedMarketplaces()
		if err != nil {
			return Pack{}, err
		}
		for _, mp := range mps {
			if p, perr := c.store.CachedPack(mp.Name, name); perr == nil {
				return p, nil
			}
		}
	}
	return Pack{}, fmt.Errorf("market: pack %q not found", name)
}
