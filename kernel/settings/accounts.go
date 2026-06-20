// SPDX-License-Identifier: MIT

package settings

import (
	"os"
	"regexp"
	"sort"
	"strings"
)

// Multi-account channel instances (M-channels): a channel kind can have several
// independent accounts running at once (e.g. 10 email accounts, several Telegram
// bots). An instance is identified by a short LABEL; its field values are the
// base AGEZT_* env names with a "#<label>" suffix — non-secret fields in the
// config store, secret fields in the vault — exactly the "<NAME>#<label>" slot
// convention the credentials keyring already uses. The UNLABELLED key (no suffix)
// is the immortal "default" instance, so every existing single-account
// deployment keeps working byte-for-byte.

// AccountSep separates a base env name from an instance label ("AGEZT_X#work").
const AccountSep = "#"

// accountLabelPattern constrains an instance label to a short slug (mirrors the
// keyring's key-label rule).
var accountLabelPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,31}$`)

// ValidAccountLabel reports whether s is a usable instance label.
func ValidAccountLabel(s string) bool { return accountLabelPattern.MatchString(s) }

// SuffixEnv returns the storage key for base under instance label. The default
// instance (empty label) uses the bare base name.
func SuffixEnv(base, label string) string {
	if label == "" {
		return base
	}
	return base + AccountSep + label
}

// AccountLabels returns the distinct NON-default instance labels configured for
// the given base env names — i.e. the labels appearing as "<base>#<label>" among
// keys (a union of the config store's and vault's key names). The default
// instance (empty label) is not included; callers always process it separately.
// Result is sorted.
func AccountLabels(keys []string, baseEnvs []string) []string {
	bases := make(map[string]struct{}, len(baseEnvs))
	for _, b := range baseEnvs {
		bases[b] = struct{}{}
	}
	seen := map[string]struct{}{}
	for _, k := range keys {
		base, label, ok := strings.Cut(k, AccountSep)
		if !ok || label == "" {
			continue
		}
		if _, isBase := bases[base]; !isBase {
			continue
		}
		if !ValidAccountLabel(label) {
			continue
		}
		seen[label] = struct{}{}
	}
	out := make([]string, 0, len(seen))
	for l := range seen {
		out = append(out, l)
	}
	sort.Strings(out)
	return out
}

// FieldGetter returns a getter that resolves a base env name to the value for
// instance label, reading the process environment (which the daemon populates
// from the store + vault at boot via injectConfig). The default instance reads
// the bare name; a labelled instance reads "<base>#<label>".
func FieldGetter(label string) func(baseEnv string) string {
	return func(base string) string {
		return os.Getenv(SuffixEnv(base, label))
	}
}

// SectionEnvs returns the base env names of the config-schema section with the
// given id (its Field.Env values), or nil if there is no such section. This is
// the single source of truth for "which env vars belong to channel kind X".
func SectionEnvs(sectionID string) []string {
	for _, sec := range Schema() {
		if sec.ID != sectionID {
			continue
		}
		envs := make([]string, 0, len(sec.Fields))
		for _, f := range sec.Fields {
			envs = append(envs, f.Env)
		}
		return envs
	}
	return nil
}
