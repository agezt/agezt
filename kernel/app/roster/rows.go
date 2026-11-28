// SPDX-License-Identifier: MIT

package roster

// The roster family's request rows live in the contract
// (kernel/contract/opapi): the agent's ref and the cursor are texts, the page
// size is a number that knows whether it was sent at all, and the graveyard's
// age is a number — or the string a query string carries for one.
//
// The family's own rules stay here, because they are the family's vocabulary:
// a ref is required and is not trimmed, a page size that is not a page size is
// the family's default, and a cursor is read leniently.

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/agezt/agezt/kernel/contract/opapi"
)

// refOf reads the agent's ref a request row carried: the family's ref is
// required, and a blank one is refused.
//
// The ref is not trimmed — the agent's name is the name it was given.
func refOf(ref string) (string, error) {
	if strings.TrimSpace(ref) == "" {
		return "", fmt.Errorf("args.ref required")
	}
	return ref, nil
}

// rawRefOf reads the agent's ref a raw request field carried: a ref is a text,
// and a blank one is refused.
//
// The write half still reads its refs raw; this is where that reading lives
// until those rows come home too.
func rawRefOf(raw json.RawMessage) (string, error) {
	value, present := rawValue(raw)
	ref, ok := value.(string)
	if present && !ok {
		return "", fmt.Errorf("args.ref must be a string")
	}
	return refOf(ref)
}

// pageLimit reads the page size a row asked for: the family's default when the
// row did not ask for one, and the family's cap when it asked for too many.
//
// A count above zero is the caller's page size — a fraction included, which
// truncates to no rows at all; a count at or below zero is no page size the
// caller asked for, so the default stands.
func pageLimit(limit opapi.Limit, fallback, max int) int {
	size := fallback
	if limit.Present() && limit.Number() > 0 {
		size = int(limit.Number())
	}
	if size > max {
		size = max
	}
	return size
}

// pageCursor reads the cursor a row carried: a sequence number, or none.
//
// A cursor that is not a sequence number is no cursor — the page starts at the
// beginning, and the caller is not told.
func pageCursor(cursor string) (int64, bool) {
	raw := strings.TrimSpace(cursor)
	if raw == "" {
		return 0, false
	}
	seq, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || seq <= 0 {
		return 0, false
	}
	return seq, true
}
