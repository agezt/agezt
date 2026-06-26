// SPDX-License-Identifier: MIT

package settings

import "sort"

// ReloadBoundary is the inspectable config boundary: which env vars apply live
// and which require daemon/plugin restart. DeerFlow keeps this explicit to avoid
// guessing during hot-reload; AGEZT derives it from the same schema the Config
// Center renders.
type ReloadBoundary struct {
	Apply Apply    `json:"apply"`
	Envs  []string `json:"envs"`
}

// ReloadBoundaries groups fields by Apply mode with deterministic ordering.
func ReloadBoundaries(sections []Section) []ReloadBoundary {
	grouped := map[Apply][]string{}
	for _, sec := range sections {
		for _, f := range sec.Fields {
			if f.Env == "" {
				continue
			}
			apply := f.Apply
			if apply == "" {
				apply = ApplyRestart
			}
			grouped[apply] = append(grouped[apply], f.Env)
		}
	}
	order := []Apply{ApplyLive, ApplyRestart}
	out := make([]ReloadBoundary, 0, len(grouped))
	for _, apply := range order {
		envs := grouped[apply]
		if len(envs) == 0 {
			continue
		}
		sort.Strings(envs)
		out = append(out, ReloadBoundary{Apply: apply, Envs: envs})
		delete(grouped, apply)
	}
	var rest []string
	for apply := range grouped {
		rest = append(rest, string(apply))
	}
	sort.Strings(rest)
	for _, raw := range rest {
		apply := Apply(raw)
		envs := grouped[apply]
		sort.Strings(envs)
		out = append(out, ReloadBoundary{Apply: apply, Envs: envs})
	}
	return out
}
