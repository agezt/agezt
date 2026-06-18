// SPDX-License-Identifier: MIT

package market

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func remotePack(name string) Pack {
	return Pack{
		Name:        name,
		Version:     "1.0.0",
		Description: "remote pack",
		Category:    "test",
		Skills:      []PackSkill{{SkillMD: sampleSkillMD}},
	}
}

// serveMarketplace stands up an httptest server publishing a marketplace.json
// plus one pack bundle per entry, mirroring the static-catalogue layout.
func serveMarketplace(t *testing.T, packs []Pack, sign ed25519.PrivateKey) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mp := Marketplace{Name: "remote", FormatVersion: FormatVersion, Packs: nil}
	for i := range packs {
		if sign != nil {
			sig, err := SignPack(packs[i], sign, 1)
			if err != nil {
				t.Fatalf("sign: %v", err)
			}
			packs[i].Signature = sig
		}
		e := packs[i].Entry("packs/" + packs[i].Name + ".json")
		mp.Packs = append(mp.Packs, e)
		p := packs[i]
		mux.HandleFunc("/packs/"+p.Name+".json", func(w http.ResponseWriter, _ *http.Request) {
			_ = json.NewEncoder(w).Encode(p)
		})
	}
	mux.HandleFunc("/marketplace.json", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(mp)
	})
	return httptest.NewServer(mux)
}

func TestSyncerCachesAndResolves(t *testing.T) {
	srv := serveMarketplace(t, []Pack{remotePack("remote-tool")}, nil)
	defer srv.Close()

	store := NewStore(t.TempDir())
	src := Source{Name: "acme", URL: srv.URL + "/marketplace.json"}
	if err := store.AddSource(src); err != nil {
		t.Fatalf("add source: %v", err)
	}
	syncer := &Syncer{HTTP: srv.Client(), Timeout: DefaultSyncTimeout}
	res, err := syncer.Sync(context.Background(), store, src, 100)
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if res.Packs != 1 {
		t.Fatalf("want 1 pack synced, got %d", res.Packs)
	}

	// Cached index carries the LOCAL source name, not the remote's self-report.
	mps, err := store.CachedMarketplaces()
	if err != nil || len(mps) != 1 || mps[0].Name != "acme" {
		t.Fatalf("cached marketplaces = %+v err %v", mps, err)
	}
	if mps[0].Packs[0].SkillCount != 1 {
		t.Fatalf("entry counts not refreshed from pack body: %+v", mps[0].Packs[0])
	}

	// Composite library resolves the remote pack offline from the cache.
	lib := NewCompositeLibrary(nil, store)
	p, err := lib.ResolvePack("acme", "remote-tool", "")
	if err != nil {
		t.Fatalf("resolve cached pack: %v", err)
	}
	if p.Name != "remote-tool" || len(p.Skills) != 1 {
		t.Fatalf("resolved pack wrong: %+v", p)
	}
	// Unqualified resolution also finds it.
	if _, err := lib.ResolvePack("", "remote-tool", ""); err != nil {
		t.Fatalf("unqualified resolve: %v", err)
	}
}

func TestSyncKeepLastGood(t *testing.T) {
	store := NewStore(t.TempDir())

	// First sync: good catalogue with one pack.
	good := serveMarketplace(t, []Pack{remotePack("remote-tool")}, nil)
	defer good.Close()
	src := Source{Name: "acme", URL: good.URL + "/marketplace.json"}
	syncer := &Syncer{HTTP: good.Client(), Timeout: DefaultSyncTimeout}
	if _, err := syncer.Sync(context.Background(), store, src, 1); err != nil {
		t.Fatalf("first sync: %v", err)
	}

	// Second sync points at a server whose pack body is INVALID (empty skill).
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/marketplace.json" {
			mp := Marketplace{Name: "remote", FormatVersion: FormatVersion,
				Packs: []MarketplaceEntry{{Name: "broken", Version: "1.0.0", Source: "packs/broken.json"}}}
			_ = json.NewEncoder(w).Encode(mp)
			return
		}
		_ = json.NewEncoder(w).Encode(Pack{Name: "broken", Version: "1.0.0"}) // empty → Validate fails
	}))
	defer bad.Close()
	badSrc := Source{Name: "acme", URL: bad.URL + "/marketplace.json"}
	syncer2 := &Syncer{HTTP: bad.Client(), Timeout: DefaultSyncTimeout}
	if _, err := syncer2.Sync(context.Background(), store, badSrc, 2); err == nil {
		t.Fatal("expected the bad sync to fail")
	}
	// The previous good catalogue must still be intact (keep-last-good).
	p, err := store.CachedPack("acme", "remote-tool")
	if err != nil || p.Name != "remote-tool" {
		t.Fatalf("keep-last-good broken: pack=%+v err=%v", p, err)
	}
}

func TestVerifyPack(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(nil)
	p := remotePack("signed")
	sig, err := SignPack(p, priv, 5)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	p.Signature = sig

	signed, err := VerifyPack(p, "")
	if err != nil || !signed {
		t.Fatalf("valid sig should verify: signed=%v err=%v", signed, err)
	}

	// Tampering invalidates the signature.
	bad := p
	bad.Description = "tampered after signing"
	if _, err := VerifyPack(bad, ""); err == nil {
		t.Fatal("tampered pack should fail verification")
	}

	// Unsigned is allowed (signed=false, no error) unless a key is required.
	unsigned := remotePack("plain")
	if s, err := VerifyPack(unsigned, ""); err != nil || s {
		t.Fatalf("unsigned should be (false,nil): s=%v err=%v", s, err)
	}
	if _, err := VerifyPack(unsigned, hexPub(pub)); err == nil {
		t.Fatal("unsigned should fail when a signer key is required")
	}

	// A pinned key that doesn't match the signer is rejected.
	otherPub, _, _ := ed25519.GenerateKey(nil)
	if _, err := VerifyPack(p, hexPub(otherPub)); err == nil {
		t.Fatal("mismatched pinned key should be rejected")
	}
}

func hexPub(p ed25519.PublicKey) string {
	const hexdig = "0123456789abcdef"
	out := make([]byte, len(p)*2)
	for i, b := range p {
		out[i*2] = hexdig[b>>4]
		out[i*2+1] = hexdig[b&0xf]
	}
	return string(out)
}

func TestAddSourceRejectsReservedAndBadURL(t *testing.T) {
	store := NewStore(t.TempDir())
	if err := store.AddSource(Source{Name: MarketplaceOfficial, URL: "https://x/m.json"}); err == nil {
		t.Fatal("must reject the reserved official name")
	}
	if err := store.AddSource(Source{Name: "acme", URL: "ftp://x/m.json"}); err == nil {
		t.Fatal("must reject a non-http url")
	}
	if err := store.AddSource(Source{Name: "acme", URL: "https://x/m.json"}); err != nil {
		t.Fatalf("valid source rejected: %v", err)
	}
}
