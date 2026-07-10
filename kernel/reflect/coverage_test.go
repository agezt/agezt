// SPDX-License-Identifier: MIT

package reflect

import (
	"context"
	"testing"

	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/journal"
	"github.com/agezt/agezt/kernel/worldmodel"
)

// TestReflect_NilBusSkipsPublish covers the e.bus == nil early return in
// publish: an engine built without a bus still completes a Reflect pass, it just
// doesn't journal the report through the bus.
func TestReflect_NilBusSkipsPublish(t *testing.T) {
	j, err := journal.Open(t.TempDir(), journal.Options{})
	if err != nil {
		t.Fatalf("journal.Open: %v", err)
	}
	t.Cleanup(func() { _ = j.Close() })
	ws, err := worldmodel.Open(t.TempDir())
	if err != nil {
		t.Fatalf("worldmodel.Open: %v", err)
	}
	w := worldmodel.NewGraph(ws, nil)

	e := New(j, w, nil, Config{}) // nil bus
	if _, err := e.Reflect(context.Background(), "corr-nil-bus"); err != nil {
		t.Fatalf("Reflect with nil bus: %v", err)
	}
}

// TestLatest_SkipsMalformedPayload covers the json.Unmarshal failure branch in
// Latest: a reflection.completed event with an unparseable payload is ignored,
// so found stays false when it's the only such event.
func TestLatest_SkipsMalformedPayload(t *testing.T) {
	e, b, _, _ := newTestEngine(t, Config{})
	// A non-reflection event exercises the ev.Kind != KindReflectionCompleted skip.
	if _, err := b.Publish(event.Spec{Subject: "task.received", Kind: event.KindTaskReceived, Actor: "test"}); err != nil {
		t.Fatalf("publish unrelated event: %v", err)
	}
	if _, err := b.Publish(event.Spec{
		Subject: "reflection.completed",
		Kind:    event.KindReflectionCompleted,
		Actor:   "test",
		Payload: []byte("{not valid json"),
	}); err != nil {
		t.Fatalf("publish malformed report: %v", err)
	}
	if _, ok := e.Latest(); ok {
		t.Fatalf("Latest() should report not-found when the only report is malformed")
	}
}
