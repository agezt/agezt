// SPDX-License-Identifier: MIT
package storage_test

import (
	"context"
	"encoding/json"
	"errors"
	appstorage "github.com/agezt/agezt/kernel/app/storage"
	"io/fs"
	"path/filepath"
	"reflect"
	"testing"
	"testing/fstest"

	"github.com/agezt/agezt/kernel/platform/schema"
)

type files struct {
	root      fs.FS
	readCause error
	usage     map[string][2]int64
	paths     []string
}

func (f *files) ReadDir(string) ([]fs.DirEntry, error) {
	rows, err := fs.ReadDir(f.root, ".")
	if f.readCause != nil {
		return rows, f.readCause
	}
	return rows, err
}
func (f *files) Usage(path string) (int64, int64) {
	f.paths = append(f.paths, path)
	values := f.usage[filepath.Base(path)]
	return values[0], values[1]
}
func fixture() *files {
	return &files{root: fstest.MapFS{"journal": {Mode: fs.ModeDir}, "memory": {Mode: fs.ModeDir}, "plugin": {Mode: fs.ModeDir}, "zero": {Mode: fs.ModeDir}, "root.txt": {Data: []byte("12345")}, "empty.txt": {Data: []byte{}}}, usage: map[string][2]int64{"journal": {10, 2}, "memory": {10, 1}, "plugin": {3, 1}, "zero": {0, 0}}}
}
func TestStorageStatsRetainsAggregationOrderLabelsAndSelectedPaths(t *testing.T) {
	f := fixture()
	probePath := ""
	service := appstorage.New("owned-base", f, func(path string) (uint64, uint64, error) { probePath = path; return 0, 100, nil })
	out, err := service.Stats(context.Background(), appstorage.StatsInput{})
	if err != nil || out.BaseDir != "owned-base" || out.TotalBytes != 28 || out.TotalFiles != 6 || probePath != "owned-base" {
		t.Fatal(out, err, probePath)
	}
	names := []string{}
	for _, dir := range out.Directories {
		names = append(names, dir.Name)
	}
	if !reflect.DeepEqual(names, []string{"journal", "memory", "(home root)", "plugin", "zero"}) {
		t.Fatal(names)
	}
	if out.Directories[0].Label == "" || out.Directories[1].Label == "" || out.Directories[2].Label != "Loose files at the home root" || out.Directories[3].Label != "" || out.Directories[4].Files != 0 {
		t.Fatal(out.Directories)
	}
	wantPaths := []string{filepath.Join("owned-base", "journal"), filepath.Join("owned-base", "memory"), filepath.Join("owned-base", "plugin"), filepath.Join("owned-base", "zero")}
	if !reflect.DeepEqual(f.paths, wantPaths) {
		t.Fatal(f.paths, wantPaths)
	}
	if !out.DiskAvailable || out.DiskFreeBytes == nil || *out.DiskFreeBytes != 0 || out.DiskTotalBytes == nil || *out.DiskTotalBytes != 100 || out.DiskFreePercent == nil || *out.DiskFreePercent != 0 {
		t.Fatal(out)
	}
	raw, _ := json.Marshal(out)
	var wire map[string]any
	_ = json.Unmarshal(raw, &wire)
	for _, key := range []string{"disk_free_bytes", "disk_total_bytes", "disk_free_pct"} {
		if _, present := wire[key]; !present {
			t.Fatal("present zero probe field omitted", key, string(raw))
		}
	}
}
func TestStorageStatsRetainsProbeAvailabilityAndPercentage(t *testing.T) {
	cause := errors.New("owned probe cause")
	for _, mode := range []string{"none", "error", "zero-total", "success"} {
		t.Run(mode, func(t *testing.T) {
			var probe func(string) (uint64, uint64, error)
			switch mode {
			case "error":
				probe = func(string) (uint64, uint64, error) { return 25, 100, cause }
			case "zero-total":
				probe = func(string) (uint64, uint64, error) { return 25, 0, nil }
			case "success":
				probe = func(string) (uint64, uint64, error) { return 25, 100, nil }
			}
			out, err := appstorage.New("owned", fixture(), probe).Stats(context.Background(), appstorage.StatsInput{})
			if err != nil || out.DiskAvailable != (mode == "success") {
				t.Fatal(out, err)
			}
			if mode == "success" {
				if out.DiskFreePercent == nil || *out.DiskFreePercent != 25 {
					t.Fatal(out)
				}
			} else {
				raw, _ := json.Marshal(out)
				var wire map[string]any
				_ = json.Unmarshal(raw, &wire)
				for _, key := range []string{"disk_free_bytes", "disk_total_bytes", "disk_free_pct"} {
					if _, exists := wire[key]; exists {
						t.Fatal(mode, key, string(raw))
					}
				}
			}
		})
	}
}
func TestStorageStatsRetainsEmptyAndPartialBestEffortDiagnostic(t *testing.T) {
	empty := &files{root: fstest.MapFS{}, readCause: errors.New("owned read cause")}
	out, err := appstorage.New("owned", empty, nil).Stats(context.Background(), appstorage.StatsInput{})
	raw, _ := json.Marshal(out)
	var wire map[string]any
	_ = json.Unmarshal(raw, &wire)
	if err != nil || out.TotalBytes != 0 || out.TotalFiles != 0 || out.Directories != nil || wire["dirs"] != nil || wire["disk_available"] != false {
		t.Fatal(out, err, string(raw))
	}
	partial := fixture()
	partial.readCause = errors.New("partial read cause")
	out, err = appstorage.New("owned", partial, nil).Stats(context.Background(), appstorage.StatsInput{})
	if err != nil || out.TotalBytes != 28 || out.TotalFiles != 6 {
		t.Fatal(out, err)
	}
	if _, err := json.Marshal(out); err != nil {
		t.Fatal(err)
	}
}
func TestDiskStatsReportsJournalUsageAndProbe(t *testing.T) {
	cause := errors.New("owned probe cause")
	for _, mode := range []string{"none", "error", "zero-total", "success", "zero-free"} {
		t.Run(mode, func(t *testing.T) {
			var probe func(string) (uint64, uint64, error)
			probePath := ""
			switch mode {
			case "error":
				probe = func(string) (uint64, uint64, error) { return 25, 100, cause }
			case "zero-total":
				probe = func(string) (uint64, uint64, error) { return 25, 0, nil }
			case "success":
				probe = func(path string) (uint64, uint64, error) { probePath = path; return 1, 3, nil }
			case "zero-free":
				probe = func(string) (uint64, uint64, error) { return 0, 9007199254740993, nil }
			}
			f := fixture()
			out, err := appstorage.New("owned-base", f, probe).Disk(context.Background(), appstorage.DiskInput{})
			if err != nil || out.BaseDir != "owned-base" || out.JournalBytes != 10 || !reflect.DeepEqual(f.paths, []string{filepath.Join("owned-base", "journal")}) {
				t.Fatal("the journal's usage is the journal directory's", out, err, f.paths)
			}
			raw, _ := json.Marshal(out)
			switch mode {
			case "success":
				if string(raw) != `{"base_dir":"owned-base","journal_bytes":10,"disk_available":true,"disk_free_bytes":1,"disk_total_bytes":3,"disk_free_pct":33.33333333333333}` || probePath != "owned-base" {
					t.Fatal(string(raw), probePath)
				}
			case "zero-free":
				if string(raw) != `{"base_dir":"owned-base","journal_bytes":10,"disk_available":true,"disk_free_bytes":0,"disk_total_bytes":9007199254740993,"disk_free_pct":0}` {
					t.Fatal("present zero fields stay, and byte counts keep full precision", string(raw))
				}
			default:
				if string(raw) != `{"base_dir":"owned-base","journal_bytes":10,"disk_available":false}` {
					t.Fatal("an unknown or zero-sized filesystem leaves the disk fields out", mode, string(raw))
				}
			}
		})
	}
	empty := &files{root: fstest.MapFS{}}
	if out, err := appstorage.New("owned", empty, nil).Disk(context.Background(), appstorage.DiskInput{}); err != nil || out.JournalBytes != 0 {
		t.Fatal("a missing journal counts as empty", out, err)
	}
}
func TestStorageOperationsDeclareTwoUnauditedReads(t *testing.T) {
	if _, err := appstorage.Operations(nil); err == nil {
		t.Fatal("nil provider")
	}
	ops, err := appstorage.Operations(func(context.Context) *appstorage.Service { return appstorage.New("owned", fixture(), nil) })
	if err != nil || len(ops) != 2 {
		t.Fatal(ops, err)
	}
	for i, name := range []string{"storage_stats", "disk_stats"} {
		spec := ops[i].Spec()
		if spec.Name != name || !spec.ReadOnly || !spec.AllowUnknownInput || len(spec.OutputSchema) == 0 {
			t.Fatal(spec)
		}
	}
	out, _ := appstorage.New("owned", fixture(), func(string) (uint64, uint64, error) { return 1, 2, nil }).Disk(context.Background(), appstorage.DiskInput{})
	raw, _ := json.Marshal(out)
	if err := schema.ValidateJSON(ops[1].Spec().OutputSchema, raw); err != nil {
		t.Fatal(err, string(raw))
	}
}
