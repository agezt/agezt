// SPDX-License-Identifier: MIT

package schema_test

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/platform/schema"
)

func TestFromTypeAdvertisesActualJSONFields(t *testing.T) {
	type nested struct {
		Enabled bool `json:"enabled"`
	}
	type input struct {
		Name   string   `json:"name"`
		Count  int      `json:"count,omitempty"`
		Items  []nested `json:"items"`
		Hidden string   `json:"-"`
	}
	raw, err := schema.FromType(reflect.TypeFor[input](), false)
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{`{"name":"x","items":[{"enabled":true}]}`, `{"name":"x","count":2,"items":[]}`} {
		if err := schema.ValidateJSON(raw, json.RawMessage(value)); err != nil {
			t.Errorf("valid input=%s error=%v", value, err)
		}
	}
	for _, value := range []string{`{"items":[]}`, `{"name":"x","items":[{"enabled":"wrong"}]}`, `{"name":"x","items":[],"hidden":"not wire"}`, `{"name":"x","items":[],"count":1.5}`} {
		if err := schema.ValidateJSON(raw, json.RawMessage(value)); err == nil {
			t.Errorf("invalid input=%s accepted", value)
		}
	}
	for _, unsupported := range []reflect.Type{reflect.TypeFor[map[int]string](), reflect.TypeFor[chan int](), reflect.TypeFor[*input](), reflect.TypeFor[time.Time](), reflect.TypeFor[json.RawMessage]()} {
		if _, err := schema.FromType(unsupported, false); err == nil {
			t.Errorf("unsupported %s accepted", unsupported)
		}
	}
}

func TestFromTypeDistinguishesByteSlicesAndArrays(t *testing.T) {
	for _, tc := range []struct {
		typ reflect.Type
		raw json.RawMessage
	}{
		{reflect.TypeFor[[]byte](), json.RawMessage(`"AQI="`)},
		{reflect.TypeFor[[2]byte](), json.RawMessage(`[1,2]`)},
	} {
		raw, err := schema.FromType(tc.typ, false)
		if err != nil {
			t.Fatal(err)
		}
		if err := schema.ValidateJSON(raw, tc.raw); err != nil {
			t.Errorf("%s advertised incorrectly: %s %v", tc.typ, raw, err)
		}
	}
}
