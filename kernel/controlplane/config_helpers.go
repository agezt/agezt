// SPDX-License-Identifier: MIT

// Config helpers: stringSliceMapToAny.
// Code extracted from config.go during the Day-74 god-file split. Public API unchanged.
package controlplane






func stringSliceMapToAny(in map[string][]string) map[string]any {
	out := make(map[string]any, len(in))
	for k, v := range in {
		arr := make([]any, len(v))
		for i, s := range v {
			arr[i] = s
		}
		out[k] = arr
	}
	return out
}
