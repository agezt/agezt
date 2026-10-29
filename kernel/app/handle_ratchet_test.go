// SPDX-License-Identifier: MIT

package app_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// applicationHandleNamingFiles is the number of files in the APPLICATION layer
// that name the kernel's handle OUTSIDE a family's port file.
//
// The port file is where the handle is FILLED: every family that owns kernel
// verbs has one, and the fill is the application's own business — the
// application knows which capabilities its family needs and how each of them
// answers.
//
// A file outside a port file that names the handle is a service that reaches
// for the kernel itself. Those files are the remaining work of the port
// programme's last phase, and the count is the measurement.
//
// The count only ever goes DOWN. A family that moves its reads behind its
// ports takes its file out of this list; a file that walks back in is a
// handle that walked back in.
const applicationHandleNamingFiles = 4

// TestTheApplicationLayerNamesTheHandleOnlyInPortFiles holds the count: the
// application layer names the kernel's handle in exactly the recorded number
// of files, and every one of them is a service — not a port file.
func TestTheApplicationLayerNamesTheHandleOnlyInPortFiles(t *testing.T) {
	var offenders []string
	err := filepath.Walk(".", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		name := filepath.Base(path)
		if strings.HasSuffix(name, "_test.go") || name == "ports.go" {
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		// "kernelruntime.Kernel" ends with "runtime.Kernel", so one spelling
		// finds both the plain and the aliased import.
		if strings.Contains(string(raw), "runtime.Kernel") {
			offenders = append(offenders, filepath.ToSlash(path))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(offenders) != applicationHandleNamingFiles {
		t.Errorf("the application layer names the handle in %d files (%s) — the recorded count is %d",
			len(offenders), strings.Join(offenders, ", "), applicationHandleNamingFiles)
	}
}
