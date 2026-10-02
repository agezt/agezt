// SPDX-License-Identifier: MIT

package configcenter

import (
	"context"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/approval"
)

// TestGetDoesNotHoldTheCenterLockWhileAwaitingApproval: a restricted key's Get
// waits for an operator (up to the approval timeout, minutes by default). It
// used to wait while holding the center's read lock — and a sync.RWMutex makes
// a waiting writer block every new reader — so one pending approval plus one
// Set froze every config read and write in the daemon.
func TestGetDoesNotHoldTheCenterLockWhileAwaitingApproval(t *testing.T) {
	c, err := New(DefaultConfig(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	reg := approval.New(approval.Config{Timeout: time.Minute})
	c.SetApprovalRegistry(reg)
	if err := c.Set(&ConfigEntry{Key: "db.replica", Value: "replica.internal:5432", Rating: RatingRestricted}); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _, _ = c.Get(ctx, ConfigAccessRequest{AgentID: "a1", Key: "db.replica", Reason: "test"}) }()
	deadline := time.Now().Add(3 * time.Second)
	for reg.PendingCount() == 0 {
		if time.Now().After(deadline) {
			t.Fatal("the restricted Get never reached the approval queue")
		}
		time.Sleep(5 * time.Millisecond)
	}

	done := make(chan error, 1)
	go func() { done <- c.Set(&ConfigEntry{Key: "feature.flag", Value: "on", Rating: RatingPublic}) }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Set blocked behind a Get that is waiting for an operator")
	}
}
