// SPDX-License-Identifier: MIT
package controlplane

import (
	"context"
	"reflect"
	"sync"
	"testing"
	"time"

	appchannels "github.com/agezt/agezt/kernel/app/channels"
)

func TestChannelOAuthNativeStatePortsSnapshotPruneAndFreshStatus(t *testing.T) {
	now := time.Now()
	s := &Server{}
	_ = s.channelOAuth()
	p := s.channelOAuthState
	p.Put("old", appchannels.OAuthFlow{Created: now.Add(-appchannels.OAuthFlowTTL - time.Second)}, now.Add(-appchannels.OAuthFlowTTL-time.Second))
	p.Put("edge", appchannels.OAuthFlow{Created: now.Add(-appchannels.OAuthFlowTTL)}, now.Add(-appchannels.OAuthFlowTTL))
	p.Put("fresh", appchannels.OAuthFlow{Created: now, Status: "pending"}, now)
	flow := appchannels.OAuthFlow{Kind: " raw kind ", Label: " raw label ", ClientID: " raw id ", ClientSecret: " raw secret ", RedirectURI: " raw redirect ", TokenURL: " raw token URL ", TokenEnv: " raw env ", Status: "pending", Created: now}
	p.Put("owned", flow, now)
	if _, ok := p.Flow("old"); ok {
		t.Fatal("expired state retained on start")
	}
	if _, ok := p.Flow("edge"); !ok {
		t.Fatal("TTL equality pruned")
	}
	snapshot, ok := p.Flow("owned")
	if !ok || !reflect.DeepEqual(snapshot, flow) {
		t.Fatal(snapshot)
	}
	snapshot.Status = "caller changed"
	snapshot.ClientSecret = "caller changed"
	p.SetStatus("owned", "done", "")
	fresh, ok := p.Flow("owned")
	if !ok || fresh.Status != "done" || fresh.ClientSecret != flow.ClientSecret || snapshot.Status != "caller changed" {
		t.Fatal("borrowed/stale flow snapshot")
	}
	status, err := s.channelOAuth().Status(context.Background(), appchannels.OAuthStatusInput{State: " owned "})
	if err != nil || status.Status != "done" {
		t.Fatal(status, err)
	}
	if _, ok := p.Flow("missing"); ok {
		t.Fatal("missing flow found")
	}
	p.SetStatus("missing", "error", "ignored")
}

func TestChannelOAuthNativeStableOwnerConcurrentInitAndServerIsolation(t *testing.T) {
	s := &Server{baseDir: t.TempDir()}
	var wg sync.WaitGroup
	services := make(chan *appchannels.OAuth, 32)
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); services <- s.channelOAuth() }()
	}
	wg.Wait()
	close(services)
	for service := range services {
		if service != s.channelOAuthService {
			t.Fatal("multiple services")
		}
	}
	now := time.Now()
	s.channelOAuthState.Put("owned", appchannels.OAuthFlow{Kind: "slack", Status: "pending", Created: now}, now)
	other := &Server{baseDir: t.TempDir()}
	_ = other.channelOAuth()
	if other.channelOAuthState == s.channelOAuthState {
		t.Fatal("state owner shared")
	}
	if _, ok := other.channelOAuthState.Flow("owned"); ok {
		t.Fatal("cross-server flow visible")
	}
}
