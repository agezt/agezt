// SPDX-License-Identifier: MIT

package skill_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	appskill "github.com/agezt/agezt/kernel/app/skill"
	"github.com/agezt/agezt/kernel/event"
	"reflect"
	"testing"
)

func TestSkillHistoryReturnsRangeCauseWithoutPartialSuccess(t *testing.T) {
	cause := errors.New("owned range failure")
	for i, rows := range [][]event.Event{nil, {{Kind: event.KindSkillCreated, Payload: json.RawMessage(`{"id":"owned"}`)}}} {
		t.Run(fmt.Sprintf("rows-%d", i), func(t *testing.T) {
			service := appskill.NewObservations(nil, observationReader{events: rows, cause: cause})
			out, err := service.History(context.Background(), appskill.GetInput{ID: "owned"})
			if err != cause || !reflect.DeepEqual(out, appskill.HistoryOutput{}) {
				t.Fatalf("EXPECTED: original Range cause and zero failed output; ACTUAL: err=%v count=%d events=%v", err, out.Count, out.Events)
			}
		})
	}
}
