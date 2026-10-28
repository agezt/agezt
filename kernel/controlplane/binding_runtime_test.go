// SPDX-License-Identifier: MIT

package controlplane

import (
	"os"
	"sort"
	"strings"
	"testing"
)

// kernelRuntimeImportPath is the module path of the kernel runtime.
//
// The adapter may HOLD the kernel handle — the runtime is what actually
// executes the runs — but it does not have to NAME the runtime package to
// hand that handle over.
const kernelRuntimeImportPath = `"github.com/agezt/agezt/kernel/runtime"`

// bindingNamesRuntime reports whether a binding file's source names the kernel
// runtime.
func bindingNamesRuntime(src string) bool {
	return strings.Contains(src, kernelRuntimeImportPath)
}

// bindingViolations are the binding files that name the kernel runtime.
//
// The list is sorted: a map's order is random, and a report whose order
// changes between two runs of the same code is a report that cannot be
// compared between two runs.
func bindingViolations(sources map[string]string) []string {
	var violations []string
	for name, src := range sources {
		if bindingNamesRuntime(src) {
			violations = append(violations, name)
		}
	}
	sort.Strings(violations)
	return violations
}

// The adapter's BINDING files are the ones that hand a family's ports over.
//
// A binding that names the runtime has to invent a helper whose only purpose
// is to spell the type:
//
//	func toolsPortsFor(k *runtime.Kernel) apptools.Ports { return apptools.PortsFromKernel(k) }
//
// and that helper is the handle in disguise — the one place in the adapter
// where the kernel's type is spelled out, and the place a family boundary
// quietly stops holding. Go does not need the name: a value of the kernel's
// type can be passed to a function that expects it without the caller ever
// spelling the package.
//
//	func (s *Server) toolsPorts() apptools.Ports { return apptools.PortsFromKernel(s.k) }
//
// So the rule is that a binding file does not import the runtime at all.
func TestTheAdapterBindingsDoNotNameTheRuntime(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	sources := map[string]string{}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		if !strings.HasSuffix(name, "_ports.go") {
			continue
		}
		raw, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		sources[name] = string(raw)
	}
	if len(sources) == 0 {
		t.Fatal("no binding files found — the adapter's bindings moved?")
	}
	for _, name := range bindingViolations(sources) {
		t.Errorf("%s is a binding file and it names the kernel runtime — the binding hands the handle over without spelling the package", name)
	}
}

// The rule is held by the rule: a matcher that could not see the violation
// would let the binding walk back in through the very door this slice closed,
// and nothing else in the tree would notice.
func TestTheBindingRuleSeesTheViolationItExistsFor(t *testing.T) {
	sources := map[string]string{
		"violating.go": "import (\n\t\"github.com/agezt/agezt/kernel/runtime\"\n)\n",
		"clean.go":     "import apptools \"github.com/agezt/agezt/kernel/app/tools\"\n",
		"contract.go":  "import \"github.com/agezt/agezt/kernel/contract/toolapi\"\n",
	}
	violations := bindingViolations(sources)
	if len(violations) != 1 || violations[0] != "violating.go" {
		t.Errorf("violations = %v, want the binding that names the runtime and nothing else", violations)
	}
	if bindingNamesRuntime("import \"github.com/agezt/agezt/kernel/app/tools\"\n") {
		t.Error("the rule took an application's import for the runtime")
	}
}

// controlPlaneRuntimeNamingFiles is how many PRODUCTION files of the control
// plane name the kernel runtime.
//
// It is the current measurement and it may only go DOWN: every file that
// names the runtime holds the handle by name, and the programme's last step
// is the handle itself.
//
// The number was fifteen before the tools, objective and workboard families
// were wired, and it stayed fifteen only because those three slices' bindings
// were written WITHOUT naming the runtime. Written the other way — with a
// helper that spelled the type — the count went to eighteen, and every
// document that said "fifteen files" became false while nothing failed. This
// test is the thing that fails.
const controlPlaneRuntimeNamingFiles = 15

// runtimeNamingDrifted reports whether a measured count has drifted from the
// recorded one.
//
// It compares in BOTH directions on purpose: a check that only fails when the
// debt grows lets the debt grow, and a check that only fails when the debt
// shrinks leaves the record stale forever. The number is a measurement, and a
// measurement has to be updated in the slice that changes it.
func runtimeNamingDrifted(count int) bool {
	return count != controlPlaneRuntimeNamingFiles
}

// runtimeNamingFiles are the control plane's production files that name the
// kernel runtime.
func runtimeNamingFiles(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	var naming []string
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		raw, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		if bindingNamesRuntime(string(raw)) {
			naming = append(naming, name)
		}
	}
	return naming
}

func TestTheControlPlaneNamesTheRuntimeInFifteenFiles(t *testing.T) {
	naming := runtimeNamingFiles(t)
	if runtimeNamingDrifted(len(naming)) {
		t.Errorf("the control plane names the kernel runtime in %d production files (%s), not %d — the number may only go down: if it did, this constant and the documents that state it change in the same slice; if it grew, the handle walked back in",
			len(naming), strings.Join(naming, ", "), controlPlaneRuntimeNamingFiles)
	}
}

// The comparison is held by the comparison: a check that can only fail in one
// direction is a check the debt walks straight past.
func TestTheRuntimeNamingCheckFailsInBothDirections(t *testing.T) {
	if runtimeNamingDrifted(controlPlaneRuntimeNamingFiles) {
		t.Error("the check rejects the recorded number itself")
	}
	if !runtimeNamingDrifted(controlPlaneRuntimeNamingFiles + 1) {
		t.Error("the check accepts more files than recorded — a growing debt walks past it")
	}
	if !runtimeNamingDrifted(controlPlaneRuntimeNamingFiles - 1) {
		t.Error("the check accepts fewer files than recorded — a shrinking debt never updates the record")
	}
}

// The measurement is held by the measurement: a counter that could not count
// would report a number it never measured, and the ratchet would be a wall
// with no wall behind it.
func TestTheRuntimeNamingMeasurementCountsWhatItClaims(t *testing.T) {
	naming := runtimeNamingFiles(t)
	if len(naming) == 0 {
		t.Fatal("the measurement found no file naming the runtime — the adapter's runtime imports are gone?")
	}
	for _, name := range naming {
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			t.Errorf("the measurement counted %q, which is not a production file of the control plane", name)
		}
	}
}
