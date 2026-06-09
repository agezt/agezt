// SPDX-License-Identifier: MIT

package creds

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// A keyring stores several API keys per provider env var and tracks which one is
// active — "store many, pick active" (M700). The ACTIVE key is mirrored to the
// bare env-var name (e.g. OPENAI_API_KEY), which is what a provider's CredLookup
// reads; every key (including the active one) is also stored under
// "<NAME>#<label>" so it can be switched to later. Switching is a manual copy of
// the chosen key into the bare name — no rotation, no failover (the owner's
// choice). A key set the old way (`agt provider creds set NAME value`, bare name
// only) appears as a synthetic "default" entry, so this layers cleanly on top of
// the existing single-credential model.

const keyringSep = "#"

// keyLabelPattern constrains a key label to a short slug.
var keyLabelPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,31}$`)

// DefaultKeyLabel is the synthetic label for a bare key that has no slot.
const DefaultKeyLabel = "default"

// KeyInfo describes one stored key WITHOUT its value: a label, whether it's the
// active key, and a short fingerprint (last 4 chars) so an operator can tell keys
// apart without ever seeing them.
type KeyInfo struct {
	Label  string `json:"label"`
	Active bool   `json:"active"`
	Last4  string `json:"last4"`
}

func slotName(name, label string) string { return name + keyringSep + label }

// last4 returns a non-reversible fingerprint: the last 4 characters, or bullets
// for a short value. Never the full secret.
func last4(v string) string {
	r := []rune(v)
	if len(r) <= 4 {
		return strings.Repeat("•", len(r))
	}
	return "…" + string(r[len(r)-4:])
}

// KeyringList returns the keys stored for env var name (labels + active flag +
// fingerprint), never the values. A bare key with no matching labelled slot shows
// up as a synthetic "default" entry.
func (s *Store) KeyringList(name string) []KeyInfo {
	active := s.Get(name)
	prefix := name + keyringSep
	out := make([]KeyInfo, 0, 4)
	matchedActive := false
	for _, k := range s.Names() {
		if !strings.HasPrefix(k, prefix) {
			continue
		}
		label := k[len(prefix):]
		if label == "" {
			continue
		}
		val := s.Get(k)
		isActive := active != "" && val == active
		if isActive {
			matchedActive = true
		}
		out = append(out, KeyInfo{Label: label, Active: isActive, Last4: last4(val)})
	}
	if active != "" && !matchedActive {
		out = append(out, KeyInfo{Label: DefaultKeyLabel, Active: true, Last4: last4(active)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Label < out[j].Label })
	return out
}

// KeyringAdd stores value under label for env var name (in memory; call Save). If
// makeActive — or if there's no active key yet — it also becomes the active key,
// mirrored to the bare name. Returns whether the active key changed.
func (s *Store) KeyringAdd(name, label, value string, makeActive bool) (activeChanged bool, err error) {
	if !keyLabelPattern.MatchString(label) {
		return false, fmt.Errorf("key label %q must be a slug (lowercase letters, digits, '-', '_'; max 32)", label)
	}
	if value == "" {
		return false, fmt.Errorf("key value required")
	}
	if err := s.Set(slotName(name, label), value); err != nil {
		return false, err
	}
	if makeActive || s.Get(name) == "" {
		if s.Get(name) == value {
			return false, nil
		}
		if err := s.Set(name, value); err != nil {
			return false, err
		}
		return true, nil
	}
	return false, nil
}

// KeyringActivate makes label's key the active one (mirrors it to the bare name).
// Activating "default" when only a bare key exists is a no-op.
func (s *Store) KeyringActivate(name, label string) error {
	v := s.Get(slotName(name, label))
	if v == "" {
		if label == DefaultKeyLabel && s.Get(name) != "" {
			return nil
		}
		return fmt.Errorf("no key labelled %q for %s", label, name)
	}
	return s.Set(name, v)
}

// KeyringRemove deletes label's key (in memory; call Save). Removing the active
// key clears the bare name too, leaving the provider uncredentialed until another
// key is activated. Returns whether anything was removed and whether the active
// key was the one removed.
func (s *Store) KeyringRemove(name, label string) (removed, wasActive bool) {
	slot := slotName(name, label)
	if s.Has(slot) {
		wasActive = s.Get(name) != "" && s.Get(slot) == s.Get(name)
		s.Remove(slot)
		if wasActive {
			s.Remove(name)
		}
		return true, wasActive
	}
	// Synthetic default = the bare key itself.
	if label == DefaultKeyLabel && s.Has(name) {
		s.Remove(name)
		return true, true
	}
	return false, false
}
