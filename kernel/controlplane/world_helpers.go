// SPDX-License-Identifier: MIT
//
// kernel/controlplane world-model shared helper (worldAliasesAttrs).
// Extracted from world.go during Day 211 god-file refactor (#79).
// Public API unchanged.
package controlplane

func worldAliasesAttrs(args map[string]any) ([]string, map[string]string, error) {
	aliases, _, err := argStringList(args, "aliases")
	if err != nil {
		return nil, nil, err
	}
	attrs, _, err := argStringMap(args, "attrs")
	if err != nil {
		return nil, nil, err
	}
	return aliases, attrs, nil
}
