// SPDX-License-Identifier: MIT

// Package generic provides type-safe generic utilities for Agezt.
package generic

import (
	"maps"
	"slices"
)

// Contains reports whether v is in s.
func Contains[T comparable](s []T, v T) bool {
	return slices.Contains(s, v)
}

// First returns the first element of s, or zero value if empty.
func First[T any](s []T) T {
	if len(s) == 0 {
		return *new(T)
	}
	return s[0]
}

// Map applies f to each element of s, returning a new slice.
func Map[T any, U any](s []T, f func(T) U) []U {
	result := make([]U, len(s))
	for i, v := range s {
		result[i] = f(v)
	}
	return result
}

// Filter returns a new slice containing all elements of s that satisfy f.
func Filter[T any](s []T, f func(T) bool) []T {
	result := make([]T, 0)
	for _, v := range s {
		if f(v) {
			result = append(result, v)
		}
	}
	return result
}

// Reduce reduces s to a single value by applying f repeatedly.
func Reduce[T any, U any](s []T, init U, f func(U, T) U) U {
	acc := init
	for _, v := range s {
		acc = f(acc, v)
	}
	return acc
}

// Clone creates a shallow copy of s.
func Clone[T any](s []T) []T {
	return slices.Clone(s)
}

// Equal reports whether two slices are equal.
// Uses maps.Equal for constant-time comparison on comparable types.
func Equal[T comparable](a, b []T) bool {
	return slices.Equal(a, b)
}

// MapSet is a set backed by a map[T]struct{}.
type MapSet[T comparable] struct {
	m map[T]struct{}
}

// NewMapSet creates a new MapSet from a slice.
func NewMapSet[T comparable](elems ...T) MapSet[T] {
	s := MapSet[T]{m: make(map[T]struct{}, len(elems))}
	for _, e := range elems {
		s.m[e] = struct{}{}
	}
	return s
}

// Add adds e to the set.
func (s *MapSet[T]) Add(e T) {
	s.m[e] = struct{}{}
}

// Remove removes e from the set.
func (s *MapSet[T]) Remove(e T) {
	delete(s.m, e)
}

// Contains reports whether e is in the set.
func (s MapSet[T]) Contains(e T) bool {
	_, ok := s.m[e]
	return ok
}

// Len returns the number of elements in the set.
func (s MapSet[T]) Len() int {
	return len(s.m)
}

// Iterate yields each element of the set.
func (s MapSet[T]) Iterate(yield func(T) bool) {
	for k := range s.m {
		if !yield(k) {
			return
		}
	}
}

// Keys returns all elements of the set as a slice.
func (s MapSet[T]) Keys() []T {
	// maps.Keys yields an iter.Seq[T]; collect it into a slice. (Direct return
	// of the iterator is a compile error — its type is iter.Seq[T], not []T.)
	return slices.Collect(maps.Keys(s.m))
}

// Union returns the union of s and t.
func (s MapSet[T]) Union(t MapSet[T]) MapSet[T] {
	result := NewMapSet[T]()
	for k := range s.m {
		result.Add(k)
	}
	for k := range t.m {
		result.Add(k)
	}
	return result
}

// Intersection returns the intersection of s and t.
func (s MapSet[T]) Intersection(t MapSet[T]) MapSet[T] {
	smaller, larger := s, t
	if s.Len() > t.Len() {
		smaller, larger = t, s
	}
	result := NewMapSet[T]()
	for k := range smaller.m {
		if larger.Contains(k) {
			result.Add(k)
		}
	}
	return result
}

// Result is a container for operation results that may succeed or fail.
type Result[T any] struct {
	Value T
	Err   error
}

// OK returns a successful result containing v.
func OK[T any](v T) Result[T] {
	return Result[T]{Value: v}
}

// Err returns a failed result containing err.
func Err[T any](err error) Result[T] {
	return Result[T]{Err: err}
}

// Unwrap returns the value or panics if Err.
func (r Result[T]) Unwrap() T {
	if r.Err != nil {
		panic("Result.Err is not nil: " + r.Err.Error())
	}
	return r.Value
}

// UnwrapOr returns the value or defaultValue if Err.
func (r Result[T]) UnwrapOr(defaultValue T) T {
	if r.Err != nil {
		return defaultValue
	}
	return r.Value
}

// UnwrapOrElse returns the value or the result of f if Err.
func (r Result[T]) UnwrapOrElse(f func() T) T {
	if r.Err != nil {
		return f()
	}
	return r.Value
}

// Map applies f to the value if OK, otherwise propagates the error.
func (r Result[T]) Map(f func(T) T) Result[T] {
	if r.Err != nil {
		return r
	}
	return OK(f(r.Value))
}

// FlatMap applies f to the value if OK, flattening the result.
func (r Result[T]) FlatMap(f func(T) Result[T]) Result[T] {
	if r.Err != nil {
		return r
	}
	return f(r.Value)
}
