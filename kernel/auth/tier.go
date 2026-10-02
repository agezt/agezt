// SPDX-License-Identifier: MIT

package auth

// Tier is the minimum authority required by an operation.
//
// Higher tiers include the permissions of lower tiers: an admin credential can
// authorize user operations, while a user credential cannot authorize admin
// operations. Public operations require no credential.
type Tier uint8

const (
	// TierUnset is the zero value, and it is deliberately NOT a usable tier.
	//
	// Tier is embedded as a required field in httpserver.RouteOpts, and
	// httpserver.Handle panics when the tier is not Valid() — a guard whose whole
	// job is to catch a route registered with no declared authority. That guard
	// could not do its job while TierPublic was 0: a RouteOpts literal that
	// omitted `Tier:` received TierPublic, the MOST PERMISSIVE tier, Valid()
	// returned true, the panic never fired, and the route was registered with no
	// authentication and no diagnostic at all. The dangerous value was a valid
	// one, so no check on validity could ever see it.
	//
	// Giving the zero value its own name makes forgetting the field fail the
	// validity check, which is what the guard always claimed to do. Today every
	// route either declares its tier or copies a variable that does — this makes
	// that a property of the type rather than a convention someone has to
	// remember.
	//
	// Nothing depended on the old numbering: Tier is compared symbolically
	// throughout the tree and is never persisted, parsed or sent over the wire.
	TierUnset Tier = iota
	TierPublic
	TierUser
	TierAdmin
)

// Valid reports whether t is a defined authority tier. TierUnset is not one:
// it is the zero value a caller gets by forgetting to set the field, and
// treating that as "public" is the failure this package exists to prevent.
func (t Tier) Valid() bool {
	return t >= TierPublic && t <= TierAdmin
}

func (t Tier) String() string {
	switch t {
	case TierUnset:
		return "unset"
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
