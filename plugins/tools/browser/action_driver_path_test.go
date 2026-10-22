// SPDX-License-Identifier: MIT

package browser

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestResolveActionDriverPath_FindsTheBundledDriver: the browser-use bundle is
// embedded in the binary AND shipped in the source tree — the skills module's
// built-in bundles (kernel/modules/skills/builtins). The driver lookup walks
// the working directory's ancestors to find it, so when the bundle moved into
// the module the lookup has to follow it: a stale path would silently resolve
// to nothing and the browser action would lose its driver in every source
// checkout (the e2e test skips when the driver is missing, so nothing else
// would notice).
func TestResolveActionDriverPath_FindsTheBundledDriver(t *testing.T) {
	driver := ResolveActionDriverPath()
	if driver == "" {
		t.Fatal("the bundled browser-use driver was not found (the bundle path is stale)")
	}
	want := filepath.Join("kernel", "modules", "skills", "builtins", "browseruse", "scripts", "browse.mjs")
	if !strings.HasSuffix(filepath.ToSlash(driver), filepath.ToSlash(want)) {
		t.Errorf("the driver resolved to %q, want a path ending in %q", driver, want)
	}
	st, err := os.Stat(driver)
	if err != nil {
		t.Fatalf("stat the resolved driver: %v", err)
	}
	if st.IsDir() || st.Size() == 0 {
		t.Errorf("the resolved driver is not a readable file: %q", driver)
	}
}
