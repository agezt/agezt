// SPDX-License-Identifier: MIT

// Package state exposes the kernel's key/value state store to operators for
// inspection: which namespaces and keys exist and what one key holds.
package state

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"

	"github.com/agezt/agezt/kernel/app"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/platform/schema"
)

// Store is the read side of the primary kernel's state store.
type Store interface {
	Namespaces() []string
	Keys(ns string) ([]string, error)
	Get(ns, key string) (json.RawMessage, bool, error)
}

type Service struct{ store Store }

func New(store Store) *Service { return &Service{store: store} }

type ListRequest struct {
	Namespace json.RawMessage `json:"namespace,omitempty"`
}

type GetRequest struct {
	Namespace json.RawMessage `json:"namespace,omitempty"`
	Key       json.RawMessage `json:"key,omitempty"`
}

// ListOutput carries either the namespaces (no namespace named) or one
// namespace's keys; the list present is always an array.
type ListOutput struct {
	Namespaces *[]string `json:"namespaces,omitempty"`
	Keys       *[]string `json:"keys,omitempty"`
	Namespace  string    `json:"namespace"`
}

// GetOutput reports one key. A missing key is found=false with a null value,
// so a stored null stays distinguishable.
type GetOutput struct {
	Namespace string `json:"namespace"`
	Key       string `json:"key"`
	Found     bool   `json:"found"`
	Value     any    `json:"value"`
}

// lenientString reads an optional string; any other JSON value reads as empty.
func lenientString(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var v any
	_ = json.Unmarshal(raw, &v)
	s, _ := v.(string)
	return s
}

// List enumerates every namespace (sorted by the store) or, for a named
// namespace, its keys.
func (s *Service) List(_ context.Context, in ListRequest) (ListOutput, error) {
	ns := lenientString(in.Namespace)
	if ns == "" {
		namespaces := append(make([]string, 0), s.store.Namespaces()...)
		return ListOutput{Namespaces: &namespaces}, nil
	}
	keys, err := s.store.Keys(ns)
	if err != nil {
		return ListOutput{}, err
	}
	keys = append(make([]string, 0, len(keys)), keys...)
	return ListOutput{Keys: &keys, Namespace: ns}, nil
}

// Get reads one key, decoding the stored value so it keeps its JSON type.
func (s *Service) Get(_ context.Context, in GetRequest) (GetOutput, error) {
	ns, key := lenientString(in.Namespace), lenientString(in.Key)
	if ns == "" || key == "" {
		return GetOutput{}, errors.New("args.namespace and args.key required")
	}
	raw, found, err := s.store.Get(ns, key)
	if err != nil {
		return GetOutput{}, err
	}
	var value any
	if found {
		if err := json.Unmarshal(raw, &value); err != nil {
			return GetOutput{}, errors.New("state value corrupt: " + err.Error())
		}
	}
	return GetOutput{Namespace: ns, Key: key, Found: found, Value: value}, nil
}

// Operations declares the two unaudited, operator-only state reads.
func Operations(provider func(context.Context) *Service) ([]app.Operation, error) {
	if provider == nil {
		return nil, errors.New("state provider required")
	}
	listOut, err := schema.FromType(reflect.TypeFor[ListOutput](), false)
	if err != nil {
		return nil, err
	}
	getOut, err := schema.FromType(reflect.TypeFor[GetOutput](), false)
	if err != nil {
		return nil, err
	}
	list, err := app.NewOperation(opapi.Spec{Name: "state_list", ReadOnly: true, OutputSchema: listOut, Authz: opapi.PrimaryOnly, Tenancy: opapi.Primary, AllowUnknownInput: true, InputSchema: json.RawMessage(`{"type":"object","additionalProperties":true,"properties":{"namespace":{}}}`)}, func(ctx context.Context, in ListRequest) (ListOutput, error) {
		return provider(ctx).List(ctx, in)
	})
	if err != nil {
		return nil, err
	}
	get, err := app.NewOperation(opapi.Spec{Name: "state_get", ReadOnly: true, OutputSchema: getOut, Authz: opapi.PrimaryOnly, Tenancy: opapi.Primary, AllowUnknownInput: true, InputSchema: json.RawMessage(`{"type":"object","additionalProperties":true,"properties":{"namespace":{},"key":{}}}`)}, func(ctx context.Context, in GetRequest) (GetOutput, error) {
		return provider(ctx).Get(ctx, in)
	})
	if err != nil {
		return nil, err
	}
	return []app.Operation{list, get}, nil
}
