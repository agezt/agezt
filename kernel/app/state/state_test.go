// SPDX-License-Identifier: MIT

package state

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/platform/schema"
)

type fakeStore struct {
	namespaces []string
	keys       map[string][]string
	values     map[string]json.RawMessage
	err        error
	calls      []string
}

func (f *fakeStore) Namespaces() []string {
	f.calls = append(f.calls, "namespaces")
	return f.namespaces
}

func (f *fakeStore) Keys(ns string) ([]string, error) {
	f.calls = append(f.calls, "keys:"+ns)
	return f.keys[ns], f.err
}

func (f *fakeStore) Get(ns, key string) (json.RawMessage, bool, error) {
	f.calls = append(f.calls, "get:"+ns+"/"+key)
	v, ok := f.values[ns+"/"+key]
	return v, ok, f.err
}

func decode[T any](t *testing.T, raw string) T {
	t.Helper()
	var in T
	if err := json.Unmarshal([]byte(raw), &in); err != nil {
		t.Fatal(err)
	}
	return in
}

func encode(t *testing.T, v any) string {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func TestList(t *testing.T) {
	ctx := context.Background()
	for _, raw := range []string{`{}`, `{"namespace":""}`, `{"namespace":3}`, `{"namespace":null}`} {
		f := &fakeStore{}
		out, err := New(f).List(ctx, decode[ListRequest](t, raw))
		if err != nil || encode(t, out) != `{"namespaces":[],"namespace":""}` || !reflect.DeepEqual(f.calls, []string{"namespaces"}) {
			t.Fatal("no string namespace lists the namespaces, always as an array", raw, encode(t, out), err, f.calls)
		}
	}
	f := &fakeStore{namespaces: []string{"a", "b"}, keys: map[string][]string{" a": {"k2", "k1"}}}
	if out, _ := New(f).List(ctx, decode[ListRequest](t, `{}`)); encode(t, out) != `{"namespaces":["a","b"],"namespace":""}` {
		t.Fatal(encode(t, out))
	}
	out, err := New(f).List(ctx, decode[ListRequest](t, `{"namespace":" a"}`))
	if err != nil || encode(t, out) != `{"keys":["k2","k1"],"namespace":" a"}` {
		t.Fatal("a named namespace lists its keys in store order; the name passes untrimmed", encode(t, out), err)
	}
	if out, _ := New(f).List(ctx, decode[ListRequest](t, `{"namespace":"missing"}`)); encode(t, out) != `{"keys":[],"namespace":"missing"}` {
		t.Fatal("an unknown namespace has no keys", encode(t, out))
	}
	boom := errors.New("state: invalid namespace")
	if _, err := New(&fakeStore{err: boom}).List(ctx, decode[ListRequest](t, `{"namespace":"../x"}`)); !errors.Is(err, boom) {
		t.Fatal(err)
	}
}

func TestGet(t *testing.T) {
	ctx := context.Background()
	for _, raw := range []string{`{}`, `{"namespace":"app"}`, `{"key":"k"}`, `{"namespace":"app","key":""}`, `{"namespace":3,"key":"k"}`, `{"namespace":"app","key":null}`} {
		f := &fakeStore{}
		if _, err := New(f).Get(ctx, decode[GetRequest](t, raw)); err == nil || err.Error() != "args.namespace and args.key required" || f.calls != nil {
			t.Fatal(raw, err, f.calls)
		}
	}
	f := &fakeStore{values: map[string]json.RawMessage{"app/n": json.RawMessage(`9007199254740993`), "app/o": json.RawMessage(`{"z":[1,"<b>"],"a":null}`), "app/nul": json.RawMessage(`null`), "app/bad": json.RawMessage(`{`)}}
	out, err := New(f).Get(ctx, decode[GetRequest](t, `{"namespace":"app","key":"o"}`))
	if err != nil || encode(t, out) != `{"namespace":"app","key":"o","found":true,"value":{"a":null,"z":[1,"\u003cb\u003e"]}}` {
		t.Fatal("the stored value keeps its JSON type", encode(t, out), err)
	}
	if out, _ := New(f).Get(ctx, decode[GetRequest](t, `{"namespace":"app","key":"n"}`)); encode(t, out) != `{"namespace":"app","key":"n","found":true,"value":9007199254740992}` {
		t.Fatal("numbers decode as the transport's float64", encode(t, out))
	}
	if out, _ := New(f).Get(ctx, decode[GetRequest](t, `{"namespace":"app","key":"nul"}`)); encode(t, out) != `{"namespace":"app","key":"nul","found":true,"value":null}` {
		t.Fatal("a stored null is found", encode(t, out))
	}
	if out, _ := New(f).Get(ctx, decode[GetRequest](t, `{"namespace":"app","key":" ghost"}`)); encode(t, out) != `{"namespace":"app","key":" ghost","found":false,"value":null}` {
		t.Fatal("a missing key is not found; the key passes untrimmed", encode(t, out))
	}
	if _, err := New(f).Get(ctx, decode[GetRequest](t, `{"namespace":"app","key":"bad"}`)); err == nil || err.Error() != "state value corrupt: unexpected end of JSON input" {
		t.Fatal(err)
	}
	boom := errors.New("state: invalid namespace")
	if _, err := New(&fakeStore{err: boom}).Get(ctx, decode[GetRequest](t, `{"namespace":"a/b","key":"k"}`)); !errors.Is(err, boom) {
		t.Fatal(err)
	}
}

func TestOperations(t *testing.T) {
	if _, err := Operations(nil); err == nil {
		t.Fatal("nil provider")
	}
	f := &fakeStore{namespaces: []string{"a"}, keys: map[string][]string{"a": {"k"}}, values: map[string]json.RawMessage{"a/k": json.RawMessage(`{"x":1}`)}}
	ops, err := Operations(func(context.Context) *Service { return New(f) })
	if err != nil || len(ops) != 2 {
		t.Fatal(ops, err)
	}
	for i, name := range []string{"state_list", "state_get"} {
		spec := ops[i].Spec()
		if spec.Name != name || !spec.ReadOnly || spec.Authz != opapi.PrimaryOnly || spec.Tenancy != opapi.Primary || !spec.AllowUnknownInput || spec.HTTP != (opapi.HTTP{}) {
			t.Fatal(spec)
		}
	}
	ctx := context.Background()
	for i, out := range []any{
		func() any { o, _ := New(f).List(ctx, ListRequest{}); return o }(),
		func() any { o, _ := New(f).List(ctx, decode[ListRequest](t, `{"namespace":"a"}`)); return o }(),
	} {
		if err := schema.ValidateJSON(ops[0].Spec().OutputSchema, []byte(encode(t, out))); err != nil {
			t.Fatal(i, err)
		}
	}
	got, _ := New(f).Get(ctx, decode[GetRequest](t, `{"namespace":"a","key":"k"}`))
	if err := schema.ValidateJSON(ops[1].Spec().OutputSchema, []byte(encode(t, got))); err != nil {
		t.Fatal(err)
	}
}
