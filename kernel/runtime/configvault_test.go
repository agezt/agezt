// SPDX-License-Identifier: MIT

package runtime_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/configcenter"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

type fakeVault map[string]string

func (v fakeVault) Get(n string) string   { return v[n] }
func (v fakeVault) Set(n, s string) error { v[n] = s; return nil }
func (v fakeVault) Remove(n string) bool  { _, ok := v[n]; delete(v, n); return ok }
func (v fakeVault) Save() error           { return nil }

// TestOpenMovesConfigCenterSecretsIntoTheVault: the daemon passes its vault as
// Config.ConfigVault, and Open migrates plaintext secrets left by older
// versions out of the config center's entry files.
func TestOpenMovesConfigCenterSecretsIntoTheVault(t *testing.T) {
	base := t.TempDir()
	const secret = "legacy-plaintext-secret"
	legacy, err := configcenter.New(configcenter.DefaultConfig(base))
	if err != nil {
		t.Fatal(err)
	}
	if err := legacy.Set(&configcenter.ConfigEntry{Key: "db.password", Value: secret, Rating: configcenter.RatingSecret}); err != nil {
		t.Fatal(err)
	}

	v := fakeVault{}
	k, err := runtime.Open(runtime.Config{BaseDir: base, Provider: mock.New(mock.FinalText("x")), ConfigVault: v})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = k.Close() })

	files, _ := filepath.Glob(filepath.Join(base, "configcenter", "entry_*.json"))
	for _, f := range files {
		b, _ := os.ReadFile(f)
		if strings.Contains(string(b), secret) {
			t.Fatalf("%s still holds the secret after Open", f)
		}
	}
	if v[configcenter.VaultPrefix+"db.password"] != secret {
		t.Fatalf("vault = %v, want the migrated secret", v)
	}
}
