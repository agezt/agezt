// SPDX-License-Identifier: MIT

package auth

// Tier is the minimum authority required by an operation.
//
// Higher tiers include the permissions of lower tiers: an admin credential can
// authorize user operations, while a user credential cannot authorize admin
// operations. Public operations require no credential.
type Tier uint8

const (
	TierPublic Tier = iota
	TierUser
	TierAdmin
)

// Valid reports whether t is a defined authority tier.
func (t Tier) Valid() bool {
	return t >= TierPublic && t <= TierAdmin
}

func (t Tier) String() string {
	switch t {
	case TierPublic:
		return "public"
	case TierUser:
		return "user"
	case TierAdmin:
		return "admin"
	default:
		return "unknown"
	}
}
