// SPDX-License-Identifier: MIT

package market

import (
	"cmp"
	"strconv"
	"strings"
)

// CompareVersions orders two version strings by semver precedence and returns
// -1, 0 or +1. The market validates versions with semverRe (major.minor.patch
// with an optional -prerelease), and multi-digit components are legal, so
// string comparison is wrong twice: "1.10.0" sorts BELOW "1.9.0" ('1' < '9')
// and "2.0.0-rc.1" does not sort below "2.0.0" at all.
//
// Exported because this is the ONE ordering rule for market versions and it is
// shared across packages: plugins/builtinmarket (and any other market.Library
// implementation) must compare versions through it rather than fork a second
// comparator or fall back to string order.
//
// Precedence follows the semver spec: the numeric core first, then prerelease
// where absence > presence, identifiers split on "." compare numerically when
// both are numeric, numerically-lower when mixed (numeric < alphanumeric), and
// lexically otherwise; a shorter identifier list is lower when all preceding
// identifiers match.
//
// A version that does not parse falls back to plain string comparison rather
// than an error: versions reach here from installed.json, which an operator can
// hand-edit, and a browsing listing must never fail (or panic) on malformed
// provenance — "different strings" also preserves the old behaviour of flagging
// something worth looking at.
func CompareVersions(a, b string) int {
	aCore, aPre, aOK := splitVersion(a)
	bCore, bPre, bOK := splitVersion(b)
	if !aOK || !bOK {
		return strings.Compare(a, b)
	}
	if c := compareCore(aCore, bCore); c != 0 {
		return c
	}
	return comparePrerelease(aPre, bPre)
}

// splitVersion cuts a version into its numeric core and prerelease, parsing the
// core into three ints. ok is false when the core is not exactly three numeric
// components.
func splitVersion(v string) (core [3]int, pre string, ok bool) {
	// Build metadata is ignored by semver precedence ("1.0.0+b" == "1.0.0").
	v, _, _ = strings.Cut(v, "+")
	c, p, _ := strings.Cut(v, "-")
	for i, part := range strings.Split(c, ".") {
		if i > 2 {
			return core, pre, false
		}
		n, err := strconv.Atoi(part)
		if err != nil {
			return core, pre, false
		}
		core[i] = n
	}
	return core, p, strings.Count(c, ".") == 2
}

func compareCore(a, b [3]int) int {
	for i := range 3 {
		if c := cmp.Compare(a[i], b[i]); c != 0 {
			return c
		}
	}
	return 0
}

func comparePrerelease(a, b string) int {
	switch {
	case a == b:
		return 0
	case a == "":
		return 1 // a release outranks any of its prereleases
	case b == "":
		return -1
	}
	ai := strings.Split(a, ".")
	bi := strings.Split(b, ".")
	for i := 0; i < len(ai) && i < len(bi); i++ {
		if c := compareIdent(ai[i], bi[i]); c != 0 {
			return c
		}
	}
	// All shared identifiers equal: more identifiers is higher precedence.
	return cmp.Compare(len(ai), len(bi))
}

// compareIdent orders one prerelease identifier pair per semver: numeric
// identifiers compare numerically and rank below alphanumeric ones.
func compareIdent(a, b string) int {
	an, aErr := strconv.Atoi(a)
	bn, bErr := strconv.Atoi(b)
	switch {
	case aErr == nil && bErr == nil:
		return cmp.Compare(an, bn)
	case aErr == nil:
		return -1
	case bErr == nil:
		return 1
	default:
		return strings.Compare(a, b)
	}
}
