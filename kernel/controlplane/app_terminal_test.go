// SPDX-License-Identifier: MIT
package controlplane

import (
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
)

func TestNativeTerminalCleanupOrderReentryPanicAndRejectedOwnership(t *testing.T) {
	s := &nativeTerminalCleanup{}
	order := []int{}
	if s.Defer(nil) {
		t.Fatal("nil cleanup")
	}
	s.Defer(func() {
		order = append(order, 1)
		s.release()
		if s.Defer(func() { t.Error("late callback ran") }) {
			t.Error("closed scope accepted")
		}
	})
	s.Defer(func() { order = append(order, 2); panic("owned cleanup panic") })
	s.Defer(func() { order = append(order, 3) })
	func() {
		defer func() {
			if recover() != "owned cleanup panic" {
				t.Error("panic identity")
			}
		}()
		s.release()
	}()
	s.release()
	if !reflect.DeepEqual(order, []int{3, 2, 1}) || len(s.callbacks) != 0 {
		t.Fatal(order)
	}
}

func TestNativeTerminalCleanupConcurrentOwnershipExactlyOnce(t *testing.T) {
	s := &nativeTerminalCleanup{}
	var accepted, released atomic.Int64
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < 64; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			if s.Defer(func() { released.Add(1) }) {
				accepted.Add(1)
			}
		}()
	}
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); <-start; s.release() }()
	}
	close(start)
	wg.Wait()
	s.release()
	if released.Load() != accepted.Load() {
		t.Fatal("lost/double cleanup", accepted.Load(), released.Load())
	}
}
