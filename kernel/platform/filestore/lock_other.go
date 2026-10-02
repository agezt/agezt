// SPDX-License-Identifier: MIT

//go:build !unix && !windows

package filestore

import (
	"os"
	"sync"
)

// Platforms without an OS file lock (js/wasm, plan9) run one process at a time
// in practice; a process-wide mutex still serialises goroutines.
var fallback sync.Mutex

func lockFile(*os.File) error   { fallback.Lock(); return nil }
func unlockFile(*os.File) error { fallback.Unlock(); return nil }
