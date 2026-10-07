// SPDX-License-Identifier: MIT
package configcenter

import (
	"context"
	"reflect"
	"strings"

	core "github.com/agezt/agezt/kernel/configcenter"
)

// Keep the foundation's exact shape/type assertions when the output becomes a DTO.
// Reflection retains integer types, unlike a JSON roundtrip through float64.
func centerTestValue(v reflect.Value) any {
	if v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return nil
		}
		return centerTestValue(v.Elem())
	}
	if v.Kind() == reflect.Struct {
		out := map[string]any{}
		for i := 0; i < v.NumField(); i++ {
			field := v.Type().Field(i)
			tag := strings.Split(field.Tag.Get("json"), ",")
			value := v.Field(i)
			if tag[0] == "-" {
				continue
			}
			if len(tag) > 1 && tag[1] == "omitempty" && ((value.Kind() == reflect.Slice || value.Kind() == reflect.Map || value.Kind() == reflect.String) && value.Len() == 0 || value.IsZero()) {
				continue
			}
			out[tag[0]] = centerTestValue(value)
		}
		return out
	}
	if v.Kind() == reflect.Slice && v.Type().Elem().Kind() == reflect.Struct {
		out := make([]map[string]any, v.Len())
		for i := range out {
			out[i] = centerTestValue(v.Index(i)).(map[string]any)
		}
		return out
	}
	return v.Interface()
}
func centerTestMap(out any, err error) (map[string]any, error) {
	if err != nil {
		return nil, err
	}
	return centerTestValue(reflect.ValueOf(out)).(map[string]any), nil
}
func testCenterEntryMap(e *core.ConfigEntry) map[string]any {
	return centerTestValue(reflect.ValueOf(entryRow(e))).(map[string]any)
}

type centerTestReads struct{ *Reads }

func testCenterReads(p Reader) *centerTestReads { return &centerTestReads{NewReads(p)} }
func (s *centerTestReads) Get(ctx context.Context, in GetInput) (map[string]any, error) {
	return centerTestMap(s.Reads.Get(ctx, in))
}
func (s *centerTestReads) List(ctx context.Context, in ListInput) (map[string]any, error) {
	return centerTestMap(s.Reads.List(ctx, in))
}
func (s *centerTestReads) AccessLog(ctx context.Context, in AccessLogInput) (map[string]any, error) {
	return centerTestMap(s.Reads.AccessLog(ctx, in))
}
func (s *centerTestReads) Audit(ctx context.Context, in AuditInput) (map[string]any, error) {
	return centerTestMap(s.Reads.Audit(ctx, in))
}
func (s *centerTestReads) Health(ctx context.Context, in HealthInput) (map[string]any, error) {
	return centerTestMap(s.Reads.Health(ctx, in))
}

type centerTestWrites struct{ *Writes }

func testCenterWrites(p Writer) *centerTestWrites { return &centerTestWrites{NewWrites(p)} }
func (s *centerTestWrites) Set(ctx context.Context, in SetInput) (map[string]any, error) {
	return centerTestMap(s.Writes.Set(ctx, in))
}
func (s *centerTestWrites) Delete(ctx context.Context, in DeleteInput) (map[string]any, error) {
	return centerTestMap(s.Writes.Delete(ctx, in))
}
func (s *centerTestWrites) SetRating(ctx context.Context, in SetRatingInput) (map[string]any, error) {
	return centerTestMap(s.Writes.SetRating(ctx, in))
}
func (s *centerTestWrites) SetAccess(ctx context.Context, in SetAccessInput) (map[string]any, error) {
	return centerTestMap(s.Writes.SetAccess(ctx, in))
}
