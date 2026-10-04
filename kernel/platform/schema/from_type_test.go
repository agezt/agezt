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
	for _, unsupported := range []reflect.Type{reflect.TypeFor[map[int]string](), reflect.TypeFor[chan int](), reflect.TypeFor[time.Time](), reflect.TypeFor[json.RawMessage]()} {
		if _, err := schema.FromType(unsupported, false); err == nil {
			t.Errorf("unsupported %s accepted", unsupported)
		}
	}
}

func TestFromTypeRetainsNullableAndTypedMapWireContract(t *testing.T) {
	type child struct {
		Name string `json:"name"`
	}
	type payload struct {
		Pointer  *child            `json:"pointer"`
		Optional *string           `json:"optional,omitempty"`
		Items    []*child          `json:"items"`
		Bytes    []byte            `json:"bytes"`
		Values   map[string]*child `json:"values"`
	}
	raw, err := schema.FromType(reflect.TypeFor[payload](), false)
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{
		`{"pointer":null,"items":null,"bytes":null,"values":null}`,
		`{"pointer":{"name":"p"},"optional":null,"items":[null,{"name":"i"}],"bytes":"AQI=","values":{"empty":null,"named":{"name":"v"}}}`,
	} {
		if err := schema.ValidateJSON(raw, json.RawMessage(value)); err != nil {
			t.Errorf("valid nullable wire rejected: %s %v", value, err)
		}
	}
	for _, value := range []string{
		`{"items":null,"bytes":null,"values":null}`,
		`{"pointer":null,"items":[{"name":2}],"bytes":null,"values":null}`,
		`{"pointer":null,"items":null,"bytes":null,"values":{"bad":{"name":2}}}`,
		`{"pointer":null,"items":null,"bytes":null,"values":{"bad":{"name":"v","extra":true}}}`,
	} {
		if err := schema.ValidateJSON(raw, json.RawMessage(value)); err == nil {
			t.Errorf("invalid nullable wire accepted: %s", value)
		}
	}
	for _, typ := range []reflect.Type{reflect.TypeFor[*payload](), reflect.TypeFor[**payload]()} {
		root, err := schema.FromType(typ, false)
		if err != nil {
			t.Fatal(err)
		}
		if err := schema.ValidateJSON(root, json.RawMessage(`null`)); err != nil {
			t.Errorf("root null rejected for %s: %v", typ, err)
		}
	}
	type recursive struct {
		Next *recursive `json:"next"`
	}
	if _, err := schema.FromType(reflect.TypeFor[recursive](), false); err == nil {
		t.Fatal("recursive unsupported shape accepted")
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
