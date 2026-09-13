// SPDX-License-Identifier: MIT
//
// ChatGPT provider-boot: SeedChatGPTCatalog + SyncChatGPTCatalog (the
// public seeding entry points) + writeChatGPTEntry + sameModelIDs (the
// writer + equality check).
// Extracted from chatgpt.go during the Day-207 god-file split.
// Public API unchanged.
package providerboot

import (
	"time"

	"github.com/agezt/agezt/kernel/catalog"
	"github.com/agezt/agezt/kernel/chatgptauth"
)

// SeedChatGPTCatalog keeps the chatgpt entry in custom.json in step with what
// the backend actually serves, so it survives reloads and appears in Models.
//
// It seeds when absent, and REFRESHES an existing entry whenever the model set
// is authoritative — the seed-once-and-never-update behaviour it replaces is
// what left signed-in installs pinned to a model list frozen at first boot. A
// builtin (offline) set never overwrites an entry already on disk.
func SeedChatGPTCatalog(store *catalog.Store, baseDir string) {
	writeChatGPTEntry(store, resolveChatGPTModels(chatgptauth.NewManager(baseDir)))
}

// SyncChatGPTCatalog refreshes the catalog entry after a sign-in and reports the
// served model surface (ids most-preferred first + the default to pin). The
// control plane calls this through a Deps hook — the kernel never imports the
// provider layer.
//
// It is called on every sign-in status poll, so it must stay cheap: discovery is
// memoized and the catalog write is skipped when the entry already matches.
func SyncChatGPTCatalog(store *catalog.Store, baseDir string) (models []string, defaultModel string) {
	// Signing in unlocks the backend source, so drop a memo that came from the
	// weaker CLI-cache one. A backend result is already the best available and
	// stays memoized — otherwise polling would mean one request per poll.
	chatgptModelCache.mu.Lock()
	if chatgptModelCache.set.Source != chatgptSourceBackend {
		chatgptModelCache.set, chatgptModelCache.at = chatgptModelSet{}, time.Time{}
	}
	chatgptModelCache.mu.Unlock()

	set := resolveChatGPTModels(chatgptauth.NewManager(baseDir))
	writeChatGPTEntry(store, set)
	return set.IDs, set.Default
}

// writeChatGPTEntry persists the entry when it would actually change anything: a
// builtin (offline) set never overwrites what is already on disk, and a set that
// matches the stored model ids is a no-op.
func writeChatGPTEntry(store *catalog.Store, set chatgptModelSet) {
	if store == nil {
		return
	}
	cur, _ := store.Load()
	var existing *catalog.Provider
	if cur != nil {
		existing = cur.Providers["chatgpt"]
	}
	if existing != nil {
		if !set.Authoritative() {
			return // nothing better to say than what's already stored
		}
		if sameModelIDs(existing.Models, set.IDs) {
			return
		}
	}
	_, _ = store.UpsertCustomProvider(chatgptCatalogEntry(set))
}

// sameModelIDs reports whether a stored model map holds exactly ids.
func sameModelIDs(stored map[string]*catalog.Model, ids []string) bool {
	if len(stored) != len(ids) {
		return false
	}
	for _, id := range ids {
		if stored[id] == nil {
			return false
		}
	}
	return true
}

