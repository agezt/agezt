// SPDX-License-Identifier: MIT

package schedule

import "errors"

// errNoScheduleLibrary is the answer the schedule app layer gives when it
// holds no library.
//
// The schedule library is the DAEMON's file (W3.49): the kernel holds the
// contract's host port, and a host that was given none holds a nil port.
//
// The refusal is the answer — not a panic, and not a silent "nothing to do".
// An operator who asks to arm a schedule on a daemon with no library must be
// told the library is missing, not that the schedule does not exist.
var errNoScheduleLibrary = errors.New("schedule: the schedule library is not available on this daemon")
