// SPDX-License-Identifier: MIT
package configcenter

import (
	"context"
	"testing"

	core "github.com/agezt/agezt/kernel/configcenter"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// The config center application's ports are filled by the APPLICATION from the
// kernel handle — the host never binds them itself.
//
// The test pins what that wiring buys: the entries the kernel's center holds
// are the entries the application reads, writes and deletes. A port bound to
// another center would leave the operator editing entries the daemon does not
// hold.
func TestTheConfigCenterPortsReachTheKernelEntries(t *testing.T) {
	dir := t.TempDir()
	k, err := runtime.Open(runtime.Config{BaseDir: dir, Provider: mock.New()})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer k.Close()

	ports := PortsFromKernel(k)
	if ports.Reader == nil {
		t.Fatal("the reader port is nil — the application cannot read the config center")
	}
	if ports.Writer == nil {
		t.Fatal("the writer port is nil — the application cannot write the config center")
	}

	// The write goes through the port and lands in the kernel's center.
	if err := ports.Set(core.NewConfigEntry("db.host", "localhost")); err != nil {
		t.Fatalf("Set: %v", err)
	}
	entries, ok := ports.Entries()
	if !ok {
		t.Fatal("the entries port says the daemon holds no config center — the kernel holds one")
	}
	if !hasEntry(entries, "db.host") {
		t.Fatalf("Entries = %+v, want the entry the port wrote", entries)
	}

	// The read service reads the same center.
	read, err := NewReads(ports.Reader).Get(context.Background(), GetInput{Key: "db.host"})
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if read.Entry.Key != "db.host" {
		t.Fatalf("Get = %+v, want the entry the port wrote", read.Entry)
	}

	// And the delete goes through the port and lands too.
	if err := ports.Delete("db.host"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	entries, _ = ports.Entries()
	if hasEntry(entries, "db.host") {
		t.Fatalf("Entries = %+v, want the entry the port deleted", entries)
	}
}

// A daemon with NO config center holds no entries, writes nothing and deletes
// nothing — and it never calls into nothing.
//
// The kernel always opens a config center, so the empty center is not
// reachable through a handle. The DISCIPLINE still has to be there, and it
// still has to be provable: the adapter used to spell this nil-check itself,
// once per surface, and the surfaces disagreed about what "no config center"
// meant.
func TestTheConfigCenterPortsSurviveADaemonWithNoConfigCenter(t *testing.T) {
	ports := PortsFromCenter(nil)

	if ports.Reader != nil {
		t.Error("the reader port is not nil — the service would call into nothing")
	}
	if ports.Writer != nil {
		t.Error("the writer port is not nil — the service would call into nothing")
	}
	entries, ok := ports.Entries()
	if ok || entries != nil {
		t.Errorf("Entries = %+v, %v — want no entries and no verdict", entries, ok)
	}
	if err := ports.Delete("db.host"); err != nil {
		t.Errorf("Delete: %v — want nothing to delete", err)
	}
	if err := ports.Set(core.NewConfigEntry("db.host", "localhost")); err != nil {
		t.Errorf("Set: %v — want nothing to write", err)
	}

	// And the services built from the empty ports refuse rather than panic.
	if _, err := NewReads(ports.Reader).Get(context.Background(), GetInput{Key: "db.host"}); err == nil {
		t.Error("the read service answered for a config center that does not exist")
	}
	if _, err := NewWrites(ports.Writer).Delete(context.Background(), DeleteInput{Key: "db.host"}); err == nil {
		t.Error("the write service answered for a config center that does not exist")
	}
}

// hasEntry reports whether the entries hold the given key.
func hasEntry(entries []*core.ConfigEntry, key string) bool {
	for _, entry := range entries {
		if entry.Key == key {
			return true
		}
	}
	return false
}
