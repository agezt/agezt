// SPDX-License-Identifier: MIT

package main

import (
	"reflect"
	"strings"
	"testing"
)

// allBuckets is every bucket the on-disk tree carries today, plus the m1000+
// bucket bucketFor produces for a chunk whose smallest milestone reference is
// >= 1000 — which the working set is one pure-M1000+ subsection away from.
var allBuckets = map[string][]unreleasedChunk{
	"current":   {{Header: "### Fixed"}},
	"m100-m199": {{Header: "### Fixed"}},
	"m200-m299": {{Header: "### Fixed"}},
	"m300-m399": {{Header: "### Fixed"}},
	"m400-m499": {{Header: "### Fixed"}},
	"m500-m599": {{Header: "### Fixed"}},
	"m600-m649": {{Header: "### Fixed"}},
	"m650-m699": {{Header: "### Fixed"}},
	"m700-m799": {{Header: "### Fixed"}},
	"m800-m899": {{Header: "### Fixed"}},
	"m900-m999": {{Header: "### Fixed"}},
	"m1000+":    {{Header: "### Fixed"}},
}

// readmeBucketOrder extracts the bucket keys in the order renderReadme lists
// them under "## Layout". The fixed `current.md` line is skipped so the
// assertion is purely about the sorted sequence.
func readmeBucketOrder(out string) []string {
	var order []string
	for _, line := range strings.Split(out, "\n") {
		const p = "- `unreleased/"
		if !strings.HasPrefix(line, p) {
			continue
		}
		rest := strings.TrimPrefix(line, p)
		key := rest[:strings.Index(rest, ".md")]
		if key != "current" {
			order = append(order, key)
		}
	}
	return order
}

// reorgBucketOrder extracts the keys in the order renderReorgLog lists them
// under "## Unreleased slices" (that listing includes "current").
func reorgBucketOrder(out string) []string {
	var order []string
	inSlices := false
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "## Unreleased slices") {
			inSlices = true
			continue
		}
		if !inSlices || !strings.HasPrefix(line, "- `") {
			continue
		}
		rest := strings.TrimPrefix(line, "- `")
		order = append(order, rest[:strings.Index(rest, "`")])
	}
	return order
}

// Buckets must list in MILESTONE order: "m1000+" names milestones >= 1000, so
// it belongs after m900-m999. sort.Strings puts it between m100-m199 and
// m200-m299 ('-' < '0' at index 4, then '1' < '2'), because the bucket name
// with a four-digit number is no longer fixed-width. Both generated indexes
// (README layout and the reorg log) would mislead an operator scanning for
// where recent work landed.
func TestRenderReadmeListsBucketsInNumericOrder(t *testing.T) {
	want := []string{
		"m100-m199", "m200-m299", "m300-m399", "m400-m499", "m500-m599",
		"m600-m649", "m650-m699", "m700-m799", "m800-m899", "m900-m999",
		"m1000+",
	}
	got := readmeBucketOrder(renderReadme(nil, allBuckets, nil))
	if !reflect.DeepEqual(got, want) {
		t.Errorf("README layout order = %v, want %v", got, want)
	}
}

func TestRenderReorgLogListsBucketsInNumericOrder(t *testing.T) {
	want := []string{
		"current",
		"m100-m199", "m200-m299", "m300-m399", "m400-m499", "m500-m599",
		"m600-m649", "m650-m699", "m700-m799", "m800-m899", "m900-m999",
		"m1000+",
	}
	got := reorgBucketOrder(renderReorgLog(nil, allBuckets, nil))
	if !reflect.DeepEqual(got, want) {
		t.Errorf("reorg log slice order = %v, want %v", got, want)
	}
}
