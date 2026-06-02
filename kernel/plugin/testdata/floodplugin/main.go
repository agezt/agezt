// SPDX-License-Identifier: MIT

// Command floodplugin is a hostile-plugin fixture for the M177 frame
// bound: on startup it writes a large UN-terminated blob to stdout (no
// '\n'), simulating a plugin that floods the host's stdout reader. A
// host with a bounded frame reader tears the plugin down (markDead);
// an unbounded one would allocate until OOM. It then blocks on stdin so
// it does not exit on its own — the host must be the one to end it.
package main

import "os"

func main() {
	// 2 MiB of 'x' with no newline — exceeds the small MaxFrameBytes the
	// test configures, so the host's readFrame must give up rather than
	// keep growing its buffer.
	blob := make([]byte, 2<<20)
	for i := range blob {
		blob[i] = 'x'
	}
	_, _ = os.Stdout.Write(blob)

	// Block forever (until the host closes our stdin / kills us).
	var b [1]byte
	for {
		if _, err := os.Stdin.Read(b[:]); err != nil {
			return
		}
	}
}
