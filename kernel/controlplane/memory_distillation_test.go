// SPDX-License-Identifier: MIT

package controlplane_test

import (
	"context"
	"testing"

	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

func TestMemoryDistillationNativeNoOpAndHalt(t *testing.T) {
	k, _, client, _ := startPair(t, mock.New(mock.FinalText("unused")))
	seen := map[string]bool{}
	for _, command := range []string{controlplane.CmdMemoryConsolidate, controlplane.CmdProfileRebuild} {
		out, err := client.Call(context.Background(), command, nil)
		if err != nil {
			t.Fatal(err)
		}
		corr, ok := out["correlation_id"].(string)
		if !ok || corr == "" || seen[corr] {
			t.Fatalf("fresh correlation lost: %v", out)
		}
		seen[corr] = true
		nullField := "consolidated_ids"
		zeroFields := []string{"clusters_found", "clusters_merged", "records_superseded", "skipped_non_json", "active_before", "active_after"}
		if command == controlplane.CmdProfileRebuild {
			nullField = "facets"
			zeroFields = []string{"input_records", "facets_written"}
		}
		if value, present := out[nullField]; !present || value != nil {
			t.Errorf("%s null %s=%v present=%v", command, nullField, value, present)
		}
		for _, field := range zeroFields {
			if value, present := out[field]; !present || value != float64(0) {
				t.Errorf("%s zero %s=%v present=%v", command, field, value, present)
			}
		}
	}
	k.Halt()
	for _, command := range []string{controlplane.CmdMemoryConsolidate, controlplane.CmdProfileRebuild} {
		if _, err := client.Call(context.Background(), command, nil); err == nil || err.Error() != "controlplane: "+runtime.ErrHalted.Error() {
			t.Errorf("%s halted error=%v", command, err)
		}
	}
}
