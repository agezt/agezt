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
