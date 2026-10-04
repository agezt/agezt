// SPDX-License-Identifier: MIT

package schema_test

import (
	"encoding/json"
	"net/netip"
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

type EmbeddedFields struct {
	Name  string `json:"name"`
	Count int    `json:"count,omitempty"`
}
type embeddedPrivate struct {
	Visible bool `json:"visible"`
}
type EmbeddedLeft struct{ Value string }
type EmbeddedRight struct {
	Value int `json:"Value"`
}
type EmbeddedTagged struct {
	Value bool `json:"Value"`
}
type EmbeddedDiamondLeft struct{ EmbeddedFields }
type EmbeddedDiamondRight struct{ EmbeddedFields }
type EmbeddedUnsupported struct{ Value chan int }

func TestFromTypeEmbeddedFieldsMatchEncodingJSON(t *testing.T) {
	for _, value := range []any{
		struct {
			EmbeddedFields
			Enabled bool `json:"enabled"`
		}{EmbeddedFields: EmbeddedFields{Name: "x"}},
		struct {
			*EmbeddedFields
			Enabled bool `json:"enabled"`
		}{},
		struct {
			*EmbeddedFields
			Enabled bool `json:"enabled"`
		}{EmbeddedFields: &EmbeddedFields{Name: "x"}},
		struct {
			EmbeddedFields `json:"nested"`
		}{EmbeddedFields: EmbeddedFields{Name: "x"}},
		struct {
			*EmbeddedFields `json:"nested,omitempty"`
		}{},
		struct{ embeddedPrivate }{embeddedPrivate: embeddedPrivate{Visible: true}},
		struct {
			EmbeddedLeft
			EmbeddedRight
		}{},
		reflect.Zero(conflictingEmbeddedType(reflect.TypeFor[EmbeddedRight](), reflect.TypeFor[EmbeddedTagged]())).Interface(),
		struct {
			EmbeddedLeft
			Value int
		}{},
		struct {
			EmbeddedUnsupported
			Value string
		}{},
		struct {
			EmbeddedFields `json:"-"`
			Enabled        bool `json:"enabled"`
		}{},
		reflect.Zero(conflictingEmbeddedType(reflect.TypeFor[EmbeddedDiamondLeft](), reflect.TypeFor[EmbeddedDiamondRight]())).Interface(),
	} {
		t.Run(reflect.TypeOf(value).String(), func(t *testing.T) {
			raw, err := schema.FromType(reflect.TypeOf(value), false)
			if err != nil {
				t.Fatal(err)
			}
			encoded, err := json.Marshal(value)
			if err != nil {
				t.Fatal(err)
			}
			if err := schema.ValidateJSON(raw, encoded); err != nil {
				t.Fatalf("actual wire %s rejected: %v schema=%s", encoded, err, raw)
			}
			var wire map[string]any
			if err := json.Unmarshal(encoded, &wire); err != nil {
				t.Fatal(err)
			}
			var declaration struct {
				Properties map[string]json.RawMessage `json:"properties"`
			}
			if err := json.Unmarshal(raw, &declaration); err != nil {
				t.Fatal(err)
			}
			for key, child := range declaration.Properties {
				if _, ok := wire[key]; !ok {
					continue
				} // optional nil embedding/omitempty
				var field struct {
					Type any `json:"type"`
				}
				if err := json.Unmarshal(child, &field); err != nil {
					t.Fatal(err)
				}
				if field.Type == "integer" || field.Type == "boolean" {
					wire[key] = "wrong"
				} else {
					wire[key] = 42
				}
				invalid, _ := json.Marshal(wire)
				if schema.ValidateJSON(raw, invalid) == nil {
					t.Fatalf("wrong promoted field type accepted: %s", invalid)
				}
				break
			}
		})
	}
}

func TestFromTypeEmbeddedFieldDominance(t *testing.T) {
	for _, tc := range []struct {
		typ            reflect.Type
		valid, invalid string
	}{
		{reflect.TypeFor[struct {
			EmbeddedLeft
			EmbeddedRight
		}](), `{"Value":1}`, `{"Value":"wrong"}`},
		{conflictingEmbeddedType(reflect.TypeFor[EmbeddedRight](), reflect.TypeFor[EmbeddedTagged]()), `{}`, `{"Value":1}`},
		{reflect.TypeFor[struct {
			EmbeddedLeft
			Value int
		}](), `{"Value":1}`, `{"Value":"wrong"}`},
		{conflictingEmbeddedType(reflect.TypeFor[EmbeddedDiamondLeft](), reflect.TypeFor[EmbeddedDiamondRight]()), `{}`, `{"name":"x"}`},
		{reflect.TypeFor[struct{ EmbeddedFields }](), `{"name":"x"}`, `{}`},
		{reflect.TypeFor[struct{ *EmbeddedFields }](), `{}`, `{"name":42}`},
	} {
		raw, err := schema.FromType(tc.typ, false)
		if err != nil {
			t.Fatal(err)
		}
		if err := schema.ValidateJSON(raw, json.RawMessage(tc.valid)); err != nil {
			t.Errorf("%s valid rejected: %v", tc.typ, err)
		}
		if schema.ValidateJSON(raw, json.RawMessage(tc.invalid)) == nil {
			t.Errorf("%s invalid accepted: %s", tc.typ, tc.invalid)
		}
	}
}

func TestFromTypeRejectsUndecodableEmbeddedPointer(t *testing.T) {
	typ := reflect.TypeFor[struct{ *embeddedPrivate }]()
	if _, err := schema.FromType(typ, false); err == nil {
		t.Fatal("unexported embedded pointer accepted without explicit schema")
	}
}

// Build intentionally ambiguous wire types dynamically: the standard vet tag
// checker rejects their source declarations, while encoding/json omits them.
func conflictingEmbeddedType(left, right reflect.Type) reflect.Type {
	return reflect.StructOf([]reflect.StructField{
		{Name: left.Name(), Type: left, Anonymous: true},
		{Name: right.Name(), Type: right, Anonymous: true},
	})
}

func TestFromTypeRequiresExplicitTextWireSchemas(t *testing.T) {
	address := netip.MustParseAddr("192.0.2.1")
	wire, err := json.Marshal(address)
	if err != nil {
		t.Fatal(err)
	}
	if string(wire) != `"192.0.2.1"` {
		t.Fatalf("stock text representation changed: %s", wire)
	}
	for _, typ := range []reflect.Type{
		reflect.TypeFor[netip.Addr](), reflect.TypeFor[*netip.Addr](),
		reflect.TypeFor[struct {
			Address netip.Addr `json:"address"`
		}](),
		reflect.TypeFor[[]netip.Addr](), reflect.TypeFor[map[string]netip.Addr](),
		reflect.TypeFor[textEncodeOnly](), reflect.TypeFor[*textEncodeOnly](),
		reflect.TypeFor[textDecodeOnly](), reflect.TypeFor[*textDecodeOnly](),
	} {
		if derived, err := schema.FromType(typ, false); err == nil {
			t.Errorf("custom text type %s registered with guessed schema %s", typ, derived)
		}
	}
}

type textEncodeOnly int

func (*textEncodeOnly) MarshalText() ([]byte, error) { return []byte("ready"), nil }

type textDecodeOnly int

func (n *textDecodeOnly) UnmarshalText([]byte) error { *n = 1; return nil }
