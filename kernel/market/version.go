// SPDX-License-Identifier: MIT

package market

import "github.com/agezt/agezt/kernel/contract/marketapi"

// CompareVersions orders two version strings by semver precedence and returns
// -1, 0 or +1. The market validates versions with the semver pattern
// (major.minor.patch with an optional -prerelease), and multi-digit
// components are legal, so string comparison is wrong twice: "1.10.0" sorts
// BELOW "1.9.0" ('1' < '9') and "2.0.0-rc.1" does not sort below "2.0.0" at
// all.
//
// The rule lives in the contract layer (kernel/contract/marketapi) — it is
// pure, and it is shared across packages: the built-in catalogue (and any
// other market.Library implementation) must compare versions through it
// rather than fork a second comparator or fall back to string order. This is
// a delegation, so there is one comparator.
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
func CompareVersions(a, b string) int { return marketapi.CompareVersions(a, b) }
