// SPDX-License-Identifier: MIT
package controlplane

import (
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
)

func TestNativeTerminalWriteFinishOrderReentryAndPanic(t *testing.T) {
	s := &nativeTerminalWrite{}
	var order []int
	if s.After(nil) {
		t.Fatal("nil callback accepted")
	}
	s.After(func() {
		order = append(order, 1)
		s.finishWrite()
		s.discard()
		if s.After(func() { t.Error("late callback") }) {
			t.Error("closed scope accepted")
		}
	})
	s.After(func() { order = append(order, 2); panic("owned post-write panic") })
	s.After(func() { order = append(order, 3) })
	if len(order) != 0 {
		t.Fatal("callback ran before writer return")
	}
	func() {
		defer func() {
			if recover() != "owned post-write panic" {
				t.Error("panic identity")
			}
		}()
		s.finishWrite()
	}()
	s.finishWrite()
	s.discard()
	if !reflect.DeepEqual(order, []int{3, 2, 1}) || len(s.callbacks) != 0 {
		t.Fatal(order)
	}
}
func TestNativeTerminalWriteDiscardNeverRestarts(t *testing.T) {
	s := &nativeTerminalWrite{}
	called := 0
	if !s.After(func() { called++ }) {
		t.Fatal("registration")
	}
	s.discard()
	s.finishWrite()
	s.discard()
	if called != 0 || s.After(func() { called++ }) || len(s.callbacks) != 0 {
		t.Fatal("panic/discard executed callback", called)
	}
}
func TestNativeTerminalWriteConcurrentOwnershipExactlyOnce(t *testing.T) {
	s := &nativeTerminalWrite{}
	var accepted, executed atomic.Int64
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < 64; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			if s.After(func() { executed.Add(1) }) {
				accepted.Add(1)
			}
		}()
	}
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); <-start; s.finishWrite() }()
	}
	close(start)
	wg.Wait()
	s.finishWrite()
	s.discard()
	if accepted.Load() != executed.Load() {
		t.Fatal("lost/double callback", accepted.Load(), executed.Load())
	}
}
