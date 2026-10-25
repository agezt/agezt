// SPDX-License-Identifier: MIT

package standing

import "errors"

// errNoStandingLibrary is the answer the standing app layer gives when it
// holds no library.
//
// The standing library is the DAEMON's file (W3.50): the kernel holds the
// contract's store port, and a host that was given none holds a nil port.
//
// The refusal is the answer — not a panic, and not a silent "nothing to do".
// An operator who asks to arm a standing order on a daemon with no library
// must be told the library is missing, not that the order does not exist.
var errNoStandingLibrary = errors.New("standing: the standing library is not available on this daemon")
