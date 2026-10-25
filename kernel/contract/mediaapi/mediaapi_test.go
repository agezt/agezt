// SPDX-License-Identifier: MIT

package mediaapi

import (
	"reflect"
	"strings"
	"testing"
)

// TestTheSeamsCarryExactlyTheirVerbs: a seam is a grant. Every method on it
// is a reach that every holder of the seam has — the daemon's adapters, the
// plugins that satisfy them, and the tools that drive them.
//
// The counts are the counts the three tools actually use: the voice tool
// transcribes and speaks (two verbs) and asks which half is configured (two
// more), the image tool generates and asks, the rerank tool reorders and
// asks. A new verb would have to be earned here.
func TestTheSeamsCarryExactlyTheirVerbs(t *testing.T) {
	cases := []struct {
		seam  interface{}
		names []string
	}{
		{(*Voice)(nil), []string{"Transcribe", "Speak", "HasSTT", "HasTTS"}},
		{(*ImageGen)(nil), []string{"GenerateImage", "HasImage"}},
		{(*Reranker)(nil), []string{"Rerank", "HasRerank"}},
	}
	for _, c := range cases {
		typ := reflect.TypeOf(c.seam).Elem()
		if typ.Kind() != reflect.Interface {
			t.Fatalf("%s must be an interface, got %v", typ.Name(), typ.Kind())
		}
		if typ.NumMethod() != len(c.names) {
			t.Errorf("%s has %d methods, want %d", typ.Name(), typ.NumMethod(), len(c.names))
		}
		for _, name := range c.names {
			if _, ok := typ.MethodByName(name); !ok {
				t.Errorf("%s must have a %s method", typ.Name(), name)
			}
		}
	}
}

// TestTheSeamsAreStructural: a seam is satisfied STRUCTURALLY — a provider
// plugin implements it without importing anything from the kernel.
//
// That only holds while every method speaks in standard library types. The
// moment a seam names a kernel type, every plugin that satisfies it has to
// import the kernel to say so, and a plugin may not.
func TestTheSeamsAreStructural(t *testing.T) {
	for _, seam := range []interface{}{(*Voice)(nil), (*ImageGen)(nil), (*Reranker)(nil)} {
		typ := reflect.TypeOf(seam).Elem()
		for i := 0; i < typ.NumMethod(); i++ {
			m := typ.Method(i)
			// An interface method's type carries no receiver: In(0) is the
			// first ARGUMENT.
			for j := 0; j < m.Type.NumIn(); j++ {
				assertStdlibOnly(t, typ.Name()+"."+m.Name, m.Type.In(j))
			}
			for j := 0; j < m.Type.NumOut(); j++ {
				assertStdlibOnly(t, typ.Name()+"."+m.Name, m.Type.Out(j))
			}
		}
	}
}

// assertStdlibOnly fails when a type in a seam's signature comes from the
// kernel. Standard library packages (and the builtin types, whose package
// path is empty) are fine; everything else is a leak waiting to happen.
func assertStdlibOnly(t *testing.T, where string, typ reflect.Type) {
	t.Helper()
	switch typ.Kind() {
	case reflect.Slice, reflect.Array, reflect.Pointer:
		assertStdlibOnly(t, where, typ.Elem())
		return
	case reflect.Map:
		assertStdlibOnly(t, where, typ.Key())
		assertStdlibOnly(t, where, typ.Elem())
		return
	case reflect.Struct:
		for i := 0; i < typ.NumField(); i++ {
			assertStdlibOnly(t, where, typ.Field(i).Type)
		}
		return
	}
	pkg := typ.PkgPath()
	if pkg == "" || pkg == "context" {
		return
	}
	if strings.Contains(pkg, "agezt") {
		t.Errorf("%s names a kernel type (%s): a plugin could not satisfy the seam without importing the kernel", where, pkg)
		return
	}
	if !isStdlib(pkg) {
		t.Errorf("%s names a third-party type (%s): the seam must speak only in standard library types", where, pkg)
	}
}

// isStdlib reports whether a package path belongs to the standard library.
// The standard library has no dot in its first path element.
func isStdlib(pkg string) bool {
	first := pkg
	if i := strings.Index(pkg, "/"); i >= 0 {
		first = pkg[:i]
	}
	return !strings.Contains(first, ".")
}
